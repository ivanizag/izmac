// Package hfs builds Macintosh HFS volumes out of files, knowing nothing of the
// emulator they are made for.
package hfs

import (
	"encoding/binary"
	"fmt"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/unicode/norm"
)

/*
A new HFS volume, laid out the way the Macintosh lays out one it has just
initialized and then had files copied onto: the boot blocks, the master
directory block, the volume bitmap, the extents and catalog B-trees, and the
forks of the files one after the other, each in a single extent.

	block 0-1     boot blocks, zeros: the volume does not start a machine
	block 2       the master directory block
	block 3-      the volume bitmap, a bit for each allocation block
	              the allocation blocks
	last but one  a copy of the master directory block
	last          nothing

The details Inside Macintosh leaves open were read off System Tools of System
6.0.8, as Apple's own software wrote it: the keys of index nodes padded to the
longest a key can be, records that start on an even byte, the flags of a file
record left at zero, the extents B-tree a header node and nothing else.

The volume is complete and the Macintosh can write to it: the catalog is given
room to grow, and the rest of the disk is free space.
*/

const (
	// BlockSize is the size of a logical block, which everything on the
	// volume is counted in
	BlockSize = 512

	nodeSize = 512

	// The longest keys of the two B-trees, not counting the length byte
	catalogKeyLength = 37
	extentsKeyLength = 7

	// The catalog node ids that are not files or folders
	rootParentID  = 1
	rootFolderID  = 2
	extentsFileID = 3
	catalogFileID = 4
	firstUserID   = 16

	// The longest names, in Mac OS Roman bytes
	maxNameLength       = 31
	maxVolumeNameLength = 27

	// The catalog records, by type
	recordFolder       = 1
	recordFile         = 2
	recordFolderThread = 3

	// The node types
	nodeIndex  = 0x00
	nodeHeader = 0x01
	nodeLeaf   = 0xff

	// mapNodes is how many nodes the map record of the header node covers,
	// which is as big as a catalog built here is allowed to be
	mapNodes = 256 * 8

	// volumeUnmounted is the attribute that says the volume was put away
	// properly and needs no checking
	volumeUnmounted = 0x0100

	// macEpoch is 1904 on the Unix clock
	macEpoch = 2082844800

	// hasBeenInited, onDesk and the rest are Finder flags. A file that
	// arrives with hasBeenInited clear is placed by the Finder in its window
	// and has its bundle read, which is what a file new to a volume wants.
	hasBeenInited = 0x0100
	isOnDesk      = 0x0001
)

// File is a Macintosh file, both forks and the Finder information
type File struct {
	Name     string
	Data     []uint8
	Resource []uint8
	Type     [4]uint8
	Creator  [4]uint8

	// Flags are the Finder flags
	Flags uint16

	Modified time.Time
}

// Folder holds files and other folders
type Folder struct {
	Name     string
	Files    []*File
	Folders  []*Folder
	Modified time.Time
}

/*
Add puts a file in the folder, inside the folders named by path, which are
made as they are needed
*/
func (f *Folder) Add(path []string, file *File) {
	folder := f
	for _, name := range path {
		folder = folder.child(name)
	}
	folder.Files = append(folder.Files, file)
}

// child finds a folder by name, making it if it is not there
func (f *Folder) child(name string) *Folder {
	for _, folder := range f.Folders {
		if folder.Name == name {
			return folder
		}
	}
	folder := &Folder{Name: name, Modified: f.Modified}
	f.Folders = append(f.Folders, folder)
	return folder
}

// Count is how many files there are in the folder and every folder in it
func (f *Folder) Count() int {
	count := len(f.Files)
	for _, folder := range f.Folders {
		count += folder.Count()
	}
	return count
}

/*
NoRoomError says the files do not fit on a volume of the size asked for, and
roughly what size would do
*/
type NoRoomError struct {
	Size   int64
	Needed int64
}

func (e *NoRoomError) Error() string {
	return fmt.Sprintf("the files need about %vKb and the volume is %vKb",
		e.Needed/1024, e.Size/1024)
}

/*
Build makes a volume of the given size, a multiple of BlockSize, holding the
folder. The name of the folder is not used: the volume has a name of its own,
and the folder's date for its own. A file or folder with no date is dated now.
*/
func Build(volumeName string, root *Folder, size int64) ([]uint8, error) {
	if size%BlockSize != 0 || size < 64*BlockSize {
		return nil, fmt.Errorf("a volume of %v bytes can not be made", size)
	}

	v := &volume{order: newNameOrder(), nextID: firstUserID}
	v.name = macName(volumeName, maxVolumeNameLength)
	if len(v.name) == 0 {
		v.name = []uint8("Untitled")
	}
	v.root = v.enter(root, rootFolderID, rootParentID, v.name)

	if err := v.layOut(size); err != nil {
		return nil, err
	}
	return v.write(), nil
}

// volume is a volume being built
type volume struct {
	order *nameOrder
	name  []uint8
	root  *folderEntry

	nextID    uint32
	fileCount uint32

	// folderCount does not count the root, the way the master directory
	// block does not
	folderCount uint32

	// What layOut works out
	blocks          int64 // logical blocks in the volume
	allocationSize  uint32
	allocationCount uint32
	allocationStart uint32 // in logical blocks
	bitmapBlocks    uint32
	extentsTree     extent
	catalogTree     extent
	catalogNodes    [][]uint8
	used            uint32 // allocation blocks taken
}

type extent struct {
	start uint32
	count uint32
}

type folderEntry struct {
	id       uint32
	parentID uint32
	name     []uint8
	modified time.Time
	files    []*fileEntry
	folders  []*folderEntry
}

type fileEntry struct {
	id       uint32
	parentID uint32
	name     []uint8
	file     *File
	data     extent
	resource extent
}

/*
enter gives a folder and everything in it their catalog node ids and their
names on the machine. Two names that the catalog takes for the same one, which
can happen once the case is ignored or a character is lost on the way to Mac
OS Roman, are told apart by a number.
*/
func (v *volume) enter(folder *Folder, id uint32, parentID uint32, name []uint8) *folderEntry {
	entry := &folderEntry{id: id, parentID: parentID, name: name, modified: folder.Modified}
	taken := make([][]uint8, 0, len(folder.Files)+len(folder.Folders))

	for _, child := range folder.Folders {
		childName := v.unique(macName(child.Name, maxNameLength), &taken)
		childID := v.nextID
		v.nextID++
		v.folderCount++
		entry.folders = append(entry.folders, v.enter(child, childID, id, childName))
	}

	for _, file := range folder.Files {
		fileName := v.unique(macName(file.Name, maxNameLength), &taken)
		entry.files = append(entry.files, &fileEntry{
			id:       v.nextID,
			parentID: id,
			name:     fileName,
			file:     file,
		})
		v.nextID++
		v.fileCount++
	}

	return entry
}

// unique numbers a name that is already taken in its folder
func (v *volume) unique(name []uint8, taken *[][]uint8) []uint8 {
	if len(name) == 0 {
		name = []uint8("Untitled")
	}

	candidate := name
	for n := 2; v.isTaken(candidate, *taken); n++ {
		suffix := fmt.Sprintf(" %v", n)
		base := name
		if len(base)+len(suffix) > maxNameLength {
			base = base[:maxNameLength-len(suffix)]
		}
		candidate = append(append([]uint8(nil), base...), suffix...)
	}

	*taken = append(*taken, candidate)
	return candidate
}

func (v *volume) isTaken(name []uint8, taken [][]uint8) bool {
	for _, other := range taken {
		if v.order.compare(name, other) == 0 {
			return true
		}
	}
	return false
}

/*
macName turns a name from the host into one the machine can have: Mac OS Roman,
with no colon, which is what separates the folders of a path there, and no
longer than the catalog takes. Characters the machine has no way to show come
out as a question mark. A name from macOS arrives with its accents as separate
characters and is composed first, or every é would be an e followed by one.
*/
func macName(name string, longest int) []uint8 {
	name = norm.NFC.String(name)

	out := make([]uint8, 0, len(name))
	for _, r := range name {
		if r == ':' {
			r = '-'
		}
		if r == utf8.RuneError || r < 0x20 && r != '\r' {
			r = '?'
		}
		b, ok := charmap.Macintosh.EncodeRune(r)
		if !ok {
			b = '?'
		}
		out = append(out, b)
	}

	if len(out) > longest {
		out = out[:longest]
	}
	return out
}

/*
layOut decides where everything goes: the size of the allocation blocks, the
two B-trees, and an extent for each fork
*/
func (v *volume) layOut(size int64) error {
	v.blocks = size / BlockSize

	// No more than 65535 allocation blocks, so they grow with the volume
	perAllocation := (v.blocks + 0xfffe) / 0xffff
	v.allocationSize = uint32(perAllocation) * BlockSize

	v.bitmapBlocks = uint32((v.blocks/perAllocation + 4095) / 4096)
	v.allocationStart = 3 + v.bitmapBlocks
	v.allocationCount = uint32((v.blocks - int64(v.allocationStart) - 2) / perAllocation)

	// The extents B-tree is never used by a volume made here, every fork
	// being one extent, and is given the few nodes the Macintosh gives one
	extentsNodes := max(4, v.allocationSize/nodeSize)
	v.extentsTree = v.allocate(int64(extentsNodes) * nodeSize)

	/*
		The catalog is built once to see how many nodes it takes, given
		half as many again to grow into, and built again once the forks
		have their places: the records are the same size either way.
	*/
	nodes := len(v.buildCatalog(0))
	spare := uint32(nodes)*3/2 + 4
	catalogBytes := (int64(spare)*nodeSize + int64(v.allocationSize) - 1) /
		int64(v.allocationSize) * int64(v.allocationSize)
	if catalogBytes/nodeSize > mapNodes {
		return fmt.Errorf("there are too many files for one volume: the catalog "+
			"would need %v nodes", catalogBytes/nodeSize)
	}
	v.catalogTree = v.allocate(catalogBytes)

	v.allocateForks(v.root)

	if v.used > v.allocationCount {
		overhead := int64(v.allocationStart+2) * BlockSize
		return &NoRoomError{
			Size:   size,
			Needed: overhead + int64(v.used)*int64(v.allocationSize),
		}
	}

	v.catalogNodes = v.buildCatalog(uint32(catalogBytes / nodeSize))
	return nil
}

// allocate takes the next allocation blocks for so many bytes
func (v *volume) allocate(bytes int64) extent {
	count := uint32((bytes + int64(v.allocationSize) - 1) / int64(v.allocationSize))
	e := extent{start: v.used, count: count}
	v.used += count
	return e
}

func (v *volume) allocateForks(folder *folderEntry) {
	for _, child := range folder.folders {
		v.allocateForks(child)
	}
	for _, f := range folder.files {
		if len(f.file.Data) > 0 {
			f.data = v.allocate(int64(len(f.file.Data)))
		}
		if len(f.file.Resource) > 0 {
			f.resource = v.allocate(int64(len(f.file.Resource)))
		}
	}
}

// write puts the volume together
func (v *volume) write() []uint8 {
	image := make([]uint8, v.blocks*BlockSize)

	mdb := v.masterDirectoryBlock()
	copy(image[2*BlockSize:], mdb)
	copy(image[(v.blocks-2)*BlockSize:], mdb)

	// The bitmap, a bit for each allocation block in use, the first one in
	// the high bit
	bitmap := image[3*BlockSize:]
	for i := uint32(0); i < v.used; i++ {
		bitmap[i/8] |= 0x80 >> (i % 8)
	}

	copy(image[v.offset(v.extentsTree.start):], v.emptyTree(v.extentsTree, extentsKeyLength))
	for i, node := range v.catalogNodes {
		copy(image[v.offset(v.catalogTree.start)+int64(i)*nodeSize:], node)
	}

	v.writeForks(image, v.root)
	return image
}

func (v *volume) writeForks(image []uint8, folder *folderEntry) {
	for _, child := range folder.folders {
		v.writeForks(image, child)
	}
	for _, f := range folder.files {
		copy(image[v.offset(f.data.start):], f.file.Data)
		copy(image[v.offset(f.resource.start):], f.file.Resource)
	}
}

// offset is where an allocation block starts in the image
func (v *volume) offset(allocationBlock uint32) int64 {
	return int64(v.allocationStart)*BlockSize + int64(allocationBlock)*int64(v.allocationSize)
}

/*
masterDirectoryBlock is the volume information, in the block after the boot
blocks and again at the end:

	  0  2  'BD'
	  2  4  created
	  6  4  modified
	 10  2  attributes
	 12  2  files in the root folder
	 14  2  the first block of the bitmap
	 16  2  where to start looking for free blocks
	 18  2  allocation blocks
	 20  4  the size of an allocation block
	 24  4  the default clump size
	 28  2  the first allocation block
	 30  4  the next catalog node id
	 34  2  free allocation blocks
	 36 28  the name
	 64  4  backed up
	 68  2  backup sequence number
	 70  4  write count
	 74  4  clump size of the extents file
	 78  4  clump size of the catalog file
	 82  2  folders in the root folder
	 84  4  files on the volume
	 88  4  folders on the volume
	 92 32  Finder information
	124  6  the sizes of caches, unused
	130  4  the size of the extents file
	134 12  its extents
	146  4  the size of the catalog file
	150 12  its extents
*/
func (v *volume) masterDirectoryBlock() []uint8 {
	mdb := make([]uint8, BlockSize)
	// The volume is dated as its root folder is, now when that has no date
	now := macTime(v.root.modified)

	binary.BigEndian.PutUint16(mdb[0:], 0x4244)
	binary.BigEndian.PutUint32(mdb[2:], now)
	binary.BigEndian.PutUint32(mdb[6:], now)
	binary.BigEndian.PutUint16(mdb[10:], volumeUnmounted)
	binary.BigEndian.PutUint16(mdb[12:], uint16(len(v.root.files)))
	binary.BigEndian.PutUint16(mdb[14:], 3)
	binary.BigEndian.PutUint16(mdb[18:], uint16(v.allocationCount))
	binary.BigEndian.PutUint32(mdb[20:], v.allocationSize)
	binary.BigEndian.PutUint32(mdb[24:], 4*v.allocationSize)
	binary.BigEndian.PutUint16(mdb[28:], uint16(v.allocationStart))
	binary.BigEndian.PutUint32(mdb[30:], v.nextID)
	binary.BigEndian.PutUint16(mdb[34:], uint16(v.allocationCount-v.used))
	mdb[36] = uint8(len(v.name))
	copy(mdb[37:], v.name)

	extentsBytes := v.extentsTree.count * v.allocationSize
	catalogBytes := v.catalogTree.count * v.allocationSize
	binary.BigEndian.PutUint32(mdb[74:], extentsBytes)
	binary.BigEndian.PutUint32(mdb[78:], catalogBytes)
	binary.BigEndian.PutUint16(mdb[82:], uint16(len(v.root.folders)))
	binary.BigEndian.PutUint32(mdb[84:], v.fileCount)
	binary.BigEndian.PutUint32(mdb[88:], v.folderCount)
	binary.BigEndian.PutUint32(mdb[92:], v.systemFolder())

	binary.BigEndian.PutUint32(mdb[130:], extentsBytes)
	putExtents(mdb[134:], v.extentsTree)
	binary.BigEndian.PutUint32(mdb[146:], catalogBytes)
	putExtents(mdb[150:], v.catalogTree)

	return mdb
}

/*
systemFolder finds a folder holding a System and a Finder, which the volume
names as its system folder. That does not make it a startup disk, which takes
boot blocks a volume made here does not have, but it does make the Finder show
the folder as the one it is.
*/
func (v *volume) systemFolder() uint32 {
	var found uint32
	var look func(folder *folderEntry)
	look = func(folder *folderEntry) {
		system, finder := false, false
		for _, f := range folder.files {
			switch string(f.file.Type[:]) {
			case "ZSYS":
				system = true
			case "FNDR":
				finder = true
			}
		}
		if system && finder && found == 0 {
			found = folder.id
		}
		for _, child := range folder.folders {
			look(child)
		}
	}
	look(v.root)
	return found
}

// putExtents writes an extent record, three extents of which only the first
// is used
func putExtents(out []uint8, e extent) {
	if e.count == 0 {
		return
	}
	binary.BigEndian.PutUint16(out[0:], uint16(e.start))
	binary.BigEndian.PutUint16(out[2:], uint16(e.count))
}

// macTime is a time on the clock of the machine, the seconds since 1904 in the
// local time
func macTime(t time.Time) uint32 {
	if t.IsZero() {
		t = time.Now()
	}
	_, zone := t.Zone()
	return uint32(t.Unix() + int64(zone) + macEpoch)
}
