package hfs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

/*
The volumes are read back here with a reader written for the tests, which walks
the catalog the way the File Manager does: down from the root through the index
nodes, and along the leaves. A volume built wrong in a way both the builder and
the reader agree on would pass that, which is why hfsutils reads them too when
it is installed. The volumes were also checked on the machine, where System 6
mounted one, opened its folders and ran an application off it.
*/

// reader reads a volume back
type reader struct {
	t     *testing.T
	image []uint8
	mdb   []uint8
}

func newReader(t *testing.T, image []uint8) *reader {
	t.Helper()
	r := &reader{t: t, image: image, mdb: image[2*BlockSize : 3*BlockSize]}
	if string(r.mdb[:2]) != "BD" {
		t.Fatalf("the master directory block starts with %q", r.mdb[:2])
	}
	if !bytes.Equal(r.mdb[:162], image[len(image)-2*BlockSize:][:162]) {
		t.Errorf("the copy of the master directory block at the end differs")
	}
	return r
}

func (r *reader) u16(at int) int { return int(binary.BigEndian.Uint16(r.mdb[at:])) }
func (r *reader) u32(at int) int { return int(binary.BigEndian.Uint32(r.mdb[at:])) }

// allocation is where an allocation block starts
func (r *reader) allocation(block int) int {
	return r.u16(28)*BlockSize + block*r.u32(20)
}

// node reads a node of the catalog
func (r *reader) node(number int) []uint8 {
	start := r.allocation(r.u16(150))
	return r.image[start+number*nodeSize : start+(number+1)*nodeSize]
}

// records splits a node into its records
func records(node []uint8) [][]uint8 {
	count := int(binary.BigEndian.Uint16(node[10:]))
	out := make([][]uint8, count)
	for i := 0; i < count; i++ {
		from := binary.BigEndian.Uint16(node[nodeSize-2*(i+1):])
		to := binary.BigEndian.Uint16(node[nodeSize-2*(i+2):])
		out[i] = node[from:to]
	}
	return out
}

// leafRecord is a catalog record read back
type leafRecord struct {
	parentID uint32
	name     []uint8
	body     []uint8
}

func splitRecord(record []uint8) leafRecord {
	keyLength := int(record[0])
	nameLength := int(record[6])
	bodyAt := 1 + keyLength
	if bodyAt%2 != 0 {
		bodyAt++
	}
	return leafRecord{
		parentID: binary.BigEndian.Uint32(record[2:]),
		name:     record[7 : 7+nameLength],
		body:     record[bodyAt:],
	}
}

/*
leaves walks the catalog along its leaves, from the first one the header names,
checking the links both ways and that the keys are in order
*/
func (r *reader) leaves() []leafRecord {
	header := r.node(0)
	firstLeaf := int(binary.BigEndian.Uint32(header[14+10:]))
	count := int(binary.BigEndian.Uint32(header[14+6:]))

	order := newNameOrder()
	var out []leafRecord
	previous := 0
	for number := firstLeaf; number != 0; {
		node := r.node(number)
		if node[8] != nodeLeaf || node[9] != 1 {
			r.t.Fatalf("node %v is of type $%02x at level %v, wanted a leaf", number, node[8], node[9])
		}
		if back := int(binary.BigEndian.Uint32(node[4:])); back != previous {
			r.t.Errorf("node %v links back to %v, wanted %v", number, back, previous)
		}
		for _, record := range records(node) {
			l := splitRecord(record)
			if len(out) > 0 {
				last := out[len(out)-1]
				if last.parentID > l.parentID ||
					last.parentID == l.parentID && order.compare(last.name, l.name) >= 0 {
					r.t.Errorf("%v:%q comes after %v:%q", l.parentID, l.name, last.parentID, last.name)
				}
			}
			out = append(out, l)
		}
		previous = number
		number = int(binary.BigEndian.Uint32(node[0:]))
	}

	if len(out) != count {
		r.t.Errorf("the leaves hold %v records and the header says %v", len(out), count)
	}
	return out
}

/*
find looks a key up from the root, through the index nodes, the way the File
Manager does, which is what the index nodes are for
*/
func (r *reader) find(parentID uint32, name []uint8) (leafRecord, bool) {
	header := r.node(0)
	number := int(binary.BigEndian.Uint32(header[14+2:]))
	order := newNameOrder()

	compare := func(l leafRecord) int {
		if l.parentID != parentID {
			if l.parentID < parentID {
				return -1
			}
			return 1
		}
		return order.compare(l.name, name)
	}

	for {
		node := r.node(number)
		recs := records(node)
		if node[8] == nodeLeaf {
			for _, record := range recs {
				if l := splitRecord(record); compare(l) == 0 {
					return l, true
				}
			}
			return leafRecord{}, false
		}

		// The last index record whose key is not past the one wanted
		next := -1
		for _, record := range recs {
			if record[0] != catalogKeyLength {
				r.t.Fatalf("an index key is %v long, wanted %v", record[0], catalogKeyLength)
			}
			if compare(splitRecord(record)) > 0 {
				break
			}
			next = int(binary.BigEndian.Uint32(record[1+catalogKeyLength:]))
		}
		if next < 0 {
			return leafRecord{}, false
		}
		number = next
	}
}

// fork reads a fork back out of a file record, at the offsets of its length
// and its first extent
func (r *reader) fork(body []uint8, lengthAt int, extentAt int) []uint8 {
	length := int(binary.BigEndian.Uint32(body[lengthAt:]))
	start := int(binary.BigEndian.Uint16(body[extentAt:]))
	if length == 0 {
		return nil
	}
	from := r.allocation(start)
	return r.image[from : from+length]
}

// someFiles is a folder with a bit of everything in it
func someFiles() *Folder {
	root := &Folder{Modified: time.Date(1991, 5, 1, 12, 0, 0, 0, time.Local)}
	root.Add(nil, &File{
		Name:    "Read Me",
		Data:    []uint8("Read me first.\r"),
		Type:    [4]uint8{'T', 'E', 'X', 'T'},
		Creator: [4]uint8{'t', 't', 'x', 't'},
		Flags:   hasBeenInited | 0x2000,
	})
	root.Add([]string{"Game ƒ"}, &File{
		Name:     "Game",
		Resource: bytes.Repeat([]uint8("CODE"), 3000),
		Type:     [4]uint8{'A', 'P', 'P', 'L'},
		Creator:  [4]uint8{'G', 'A', 'M', 'E'},
	})
	root.Add([]string{"Game ƒ", "Levels"}, &File{
		Name:     "Level 1",
		Data:     bytes.Repeat([]uint8{1, 2, 3}, 1000),
		Resource: []uint8("PICT"),
	})
	return root
}

func TestAVolumeReadsBack(t *testing.T) {
	image, err := Build("Games", someFiles(), 800*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(image) != 800*1024 {
		t.Fatalf("the volume is %v bytes", len(image))
	}

	r := newReader(t, image)
	if name := r.mdb[37 : 37+r.mdb[36]]; string(name) != "Games" {
		t.Errorf("the volume is called %q", name)
	}
	if files, folders := r.u32(84), r.u32(88); files != 3 || folders != 2 {
		t.Errorf("the volume says %v files and %v folders, wanted 3 and 2", files, folders)
	}
	if root, rootFolders := r.u16(12), r.u16(82); root != 1 || rootFolders != 1 {
		t.Errorf("the root says %v files and %v folders, wanted 1 and 1", root, rootFolders)
	}

	// Every folder has its record and its thread, every file its record
	if leaves := r.leaves(); len(leaves) != 3*2+3 {
		t.Errorf("the catalog holds %v records, wanted 9", len(leaves))
	}

	readMe, ok := r.find(rootFolderID, []uint8("read me"))
	if !ok {
		t.Fatalf("Read Me is not found, with the case of its name changed")
	}
	if got := r.fork(readMe.body, 26, 74); string(got) != "Read me first.\r" {
		t.Errorf("the data fork of Read Me is %q", got)
	}
	if typ := string(readMe.body[4:12]); typ != "TEXTttxt" {
		t.Errorf("Read Me is of type and creator %q", typ)
	}
	if flags := binary.BigEndian.Uint16(readMe.body[12:]); flags != 0x2000 {
		t.Errorf("the Finder flags of Read Me are $%04x, wanted the bundle bit and not "+
			"the one that says the Finder has seen it", flags)
	}

	// The folder is found by its name in Mac OS Roman, and the files in it
	// by the folder's id
	folder, ok := r.find(rootFolderID, []uint8("Game \xc4"))
	if !ok {
		t.Fatalf("the folder Game ƒ is not found")
	}
	game, ok := r.find(binary.BigEndian.Uint32(folder.body[6:]), []uint8("Game"))
	if !ok {
		t.Fatalf("Game is not found in its folder")
	}
	if got := r.fork(game.body, 36, 86); !bytes.Equal(got, bytes.Repeat([]uint8("CODE"), 3000)) {
		t.Errorf("the resource fork of Game is %v bytes and not what went in", len(got))
	}
}

func TestTheBitmapCoversWhatIsTaken(t *testing.T) {
	image, err := Build("Games", someFiles(), 800*1024)
	if err != nil {
		t.Fatal(err)
	}

	r := newReader(t, image)
	blocks, free := r.u16(18), r.u16(34)
	bitmap := image[3*BlockSize:]

	set := 0
	for i := 0; i < blocks; i++ {
		if bitmap[i/8]&(0x80>>(i%8)) != 0 {
			set++
		}
	}
	if set != blocks-free {
		t.Errorf("the bitmap has %v blocks in use and the volume says %v", set, blocks-free)
	}
}

func TestABigCatalogHasIndexNodes(t *testing.T) {
	root := &Folder{}
	for i := 0; i < 600; i++ {
		root.Add([]string{fmt.Sprintf("Folder %v", i%7)},
			&File{Name: fmt.Sprintf("File %03v", i), Data: []uint8{uint8(i)}})
	}

	image, err := Build("Many", root, 2<<20)
	if err != nil {
		t.Fatal(err)
	}

	r := newReader(t, image)
	if depth := binary.BigEndian.Uint16(r.node(0)[14:]); depth < 3 {
		t.Errorf("a catalog of 600 files is %v levels deep, wanted the index nodes to "+
			"have index nodes of their own", depth)
	}
	if leaves := r.leaves(); len(leaves) != 600+2*8 {
		t.Errorf("the catalog holds %v records", len(leaves))
	}

	// Every file is found from the root
	for i := 0; i < 600; i++ {
		folder, ok := r.find(rootFolderID, []uint8(fmt.Sprintf("Folder %v", i%7)))
		if !ok {
			t.Fatalf("Folder %v is not found", i%7)
		}
		id := binary.BigEndian.Uint32(folder.body[6:])
		file, ok := r.find(id, []uint8(fmt.Sprintf("File %03v", i)))
		if !ok {
			t.Fatalf("File %03v is not found", i)
		}
		if got := r.fork(file.body, 26, 74); len(got) != 1 || got[0] != uint8(i) {
			t.Fatalf("File %03v holds %v", i, got)
		}
	}
}

func TestNamesAreMadeFitForTheMachine(t *testing.T) {
	root := &Folder{}
	for _, name := range []string{
		"Report.txt", "REPORT.TXT", // the same to the catalog
		"a:b",        // a colon separates folders on the machine
		"Cafe\u0301", // an accent the way macOS spells it
		"A name much too long for the catalog to take",
	} {
		root.Add(nil, &File{Name: name})
	}

	image, err := Build("Names", root, 800*1024)
	if err != nil {
		t.Fatal(err)
	}
	r := newReader(t, image)

	for _, name := range []string{
		"Report.txt", "REPORT.TXT 2", "a-b", "Caf\x8e",
		"A name much too long for the ca",
	} {
		if _, ok := r.find(rootFolderID, []uint8(name)); !ok {
			t.Errorf("%q is not on the volume", name)
		}
	}
}

func TestTheOrderIsTheCatalogs(t *testing.T) {
	o := newNameOrder()
	cases := []struct {
		a, b string
		want int
	}{
		{"apple", "APPLE", 0},        // case does not count
		{"apple", "banana", -1},      // the alphabet does
		{"Apple", "apples", -1},      // the shorter first
		{"e", "\x8e", -1},            // é is not e, and comes after it
		{"\x8e", "f", -1},            // but before f
		{"A", "`", -1},               // the backtick sits just after A, in the ROM as in Linux
		{"File 10", "File 9", -1},    // digits are characters, not numbers
		{"Zebra", "\xc4 florin", -1}, // the symbols come after the letters
	}

	for _, c := range cases {
		if got := o.compare([]uint8(c.a), []uint8(c.b)); got != c.want {
			t.Errorf("%q against %q is %v, wanted %v", c.a, c.b, got, c.want)
		}
	}
}

func TestTooMuchForTheVolumeSaysHowMuchIsNeeded(t *testing.T) {
	root := &Folder{}
	root.Add(nil, &File{Name: "Big", Data: make([]uint8, 900*1024)})

	_, err := Build("Big", root, 800*1024)
	var noRoom *NoRoomError
	if !errors.As(err, &noRoom) {
		t.Fatalf("900Kb on an 800Kb volume gave %v", err)
	}
	if noRoom.Needed <= 900*1024 {
		t.Errorf("the volume is said to need %v bytes", noRoom.Needed)
	}

	if _, err := Build("Big", root, noRoom.Needed*2); err != nil {
		t.Errorf("twice the room it needs is not enough: %v", err)
	}
}

func TestABigVolumeHasBigAllocationBlocks(t *testing.T) {
	// 65536 logical blocks, one more than the allocation blocks can count
	image, err := Build("Big", someFiles(), 32<<20)
	if err != nil {
		t.Fatal(err)
	}
	r := newReader(t, image)

	if size := r.u32(20); size != 1024 {
		t.Errorf("the allocation blocks of a 32Mb volume are %v bytes, wanted 1024", size)
	}
	if blocks := r.u16(18); blocks < 32000 {
		t.Errorf("a 32Mb volume has %v allocation blocks", blocks)
	}
	if _, ok := r.find(rootFolderID, []uint8("Read Me")); !ok {
		t.Errorf("Read Me is not found")
	}
}

// hfsutils, when it is installed, reads the volume as a reader written by
// somebody else
func TestHfsutilsReadsTheVolume(t *testing.T) {
	hls, err := exec.LookPath("hls")
	if err != nil {
		t.Skip("hfsutils is not installed")
	}
	hmount, _ := exec.LookPath("hmount")

	image, err := Build("Games", someFiles(), 800*1024)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	filename := filepath.Join(dir, "games.dsk")
	if err := os.WriteFile(filename, image, 0o600); err != nil {
		t.Fatal(err)
	}

	// hfsutils keeps the volume it has mounted in a file in the home folder
	env := append(os.Environ(), "HOME="+dir)
	mount := exec.Command(hmount, filename)
	mount.Env = env
	if output, err := mount.CombinedOutput(); err != nil {
		t.Fatalf("hmount failed: %v: %s", err, output)
	}

	list := exec.Command(hls, "-lR")
	list.Env = env
	output, err := list.CombinedOutput()
	if err != nil {
		t.Fatalf("hls failed: %v: %s", err, output)
	}

	for _, want := range []string{"TEXT/ttxt", "Read Me", "APPL/GAME", "Level 1"} {
		if !strings.Contains(string(output), want) {
			t.Errorf("hls does not list %q:\n%s", want, output)
		}
	}
}
