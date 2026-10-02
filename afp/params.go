package afp

import (
	"encoding/binary"
	"os"
	"time"
)

/*
The parameters of files and folders, what FPGetFileDirParms and FPEnumerate
answer with and the FPSet calls change, each chosen by a bit of a bitmap and
in the order of the bits.
*/

// The parameters of both
const (
	paramAttributes = 1 << 0
	paramParentID   = 1 << 1
	paramCreated    = 1 << 2
	paramModified   = 1 << 3
	paramBackedUp   = 1 << 4
	paramFinderInfo = 1 << 5
	paramLongName   = 1 << 6
	paramShortName  = 1 << 7
	paramProDOSInfo = 1 << 13
)

// The parameters of a file
const (
	fileNumber         = 1 << 8
	fileDataLength     = 1 << 9
	fileResourceLength = 1 << 10

	fileAllParameters = 1<<11 - 1 | paramProDOSInfo
)

// The parameters of a folder
const (
	dirID           = 1 << 8
	dirOffspring    = 1 << 9
	dirOwnerID      = 1 << 10
	dirGroupID      = 1 << 11
	dirAccessRights = 1 << 12

	dirAllParameters = 1<<14 - 1
)

// The attributes
const (
	attributeInvisible      = 1 << 0
	attributeDataOpen       = 1 << 3
	attributeResourceOpen   = 1 << 4
	attributeWriteInhibit   = 1 << 5
	attributeSetClear       = 1 << 15
	finderFlagInvisible     = 0x4000
	finderFlagsOffset       = 8
	isDirectoryFlag         = 0x80
	shortNameLength         = 12
	mostForkLength          = 0x7fffffff
	everyoneMayDoEverything = 0x87070707
)

// finderInfo is the Finder information of a file or a folder
func (v *volume) finderInfo(rel string, e entry) [32]uint8 {
	finder, ok := v.meta.finderInfo(v.host(rel))
	if !ok && !e.info.IsDir() {
		finder = guessFinderInfo(e.host)
	}
	return finder
}

// invisible tells whether Finder information hides what it is of
func invisible(finder [32]uint8) bool {
	return binary.BigEndian.Uint16(finder[finderFlagsOffset:])&finderFlagInvisible != 0
}

// shortName is the name for MS-DOS and ProDOS, which nobody asks for here
func shortName(name []uint8) []uint8 {
	if len(name) > shortNameLength {
		return name[:shortNameLength]
	}
	return name
}

// common are the parameters files and folders have alike, up to their names
func (v *volume) common(p *parameters, rel string, e entry, bitmap uint16, attributes uint16) {
	finder := v.finderInfo(rel, e)
	if invisible(finder) {
		attributes |= attributeInvisible
	}
	if bitmap&paramAttributes != 0 {
		p.uint16(attributes)
	}
	if bitmap&paramParentID != 0 {
		p.uint32(v.parentID(rel))
	}
	if bitmap&paramCreated != 0 {
		p.uint32(afpTime(createdTime(e.info)))
	}
	if bitmap&paramModified != 0 {
		p.uint32(afpTime(e.info.ModTime()))
	}
	if bitmap&paramBackedUp != 0 {
		p.uint32(neverBackedUp)
	}
	if bitmap&paramFinderInfo != 0 {
		p.raw(finder[:])
	}
	if bitmap&paramLongName != 0 {
		p.name(e.name)
	}
	if bitmap&paramShortName != 0 {
		p.name(shortName(e.name))
	}
}

// fileParms are the parameters of a file
func (v *volume) fileParms(rel string, e entry, bitmap uint16) ([]uint8, int32) {
	if bitmap&^fileAllParameters != 0 {
		return nil, errBitmapErr
	}

	var attributes uint16
	dataOpen, resourceOpen := v.openForks(rel)
	if dataOpen {
		attributes |= attributeDataOpen
	}
	if resourceOpen {
		attributes |= attributeResourceOpen
	}
	if e.info.Mode().Perm()&0o200 == 0 {
		attributes |= attributeWriteInhibit
	}

	p := &parameters{}
	v.common(p, rel, e, bitmap, attributes)
	if bitmap&fileNumber != 0 {
		p.uint32(v.id(rel))
	}
	if bitmap&fileDataLength != 0 {
		p.uint32(uint32(min(e.info.Size(), mostForkLength)))
	}
	if bitmap&fileResourceLength != 0 {
		p.uint32(uint32(min(v.resourceLength(rel), mostForkLength)))
	}
	if bitmap&paramProDOSInfo != 0 {
		p.raw(make([]uint8, 6))
	}
	return p.bytes(), errNoErr
}

// dirParms are the parameters of a folder
func (v *volume) dirParms(rel string, e entry, bitmap uint16) ([]uint8, int32) {
	if bitmap&^dirAllParameters != 0 {
		return nil, errBitmapErr
	}

	p := &parameters{}
	v.common(p, rel, e, bitmap, 0)
	if bitmap&dirID != 0 {
		p.uint32(v.id(rel))
	}
	if bitmap&dirOffspring != 0 {
		entries, _ := v.list(rel)
		p.uint16(uint16(min(len(entries), 0xffff)))
	}
	if bitmap&dirOwnerID != 0 {
		p.uint32(0)
	}
	if bitmap&dirGroupID != 0 {
		p.uint32(0)
	}
	if bitmap&dirAccessRights != 0 {
		p.uint32(everyoneMayDoEverything)
	}
	if bitmap&paramProDOSInfo != 0 {
		p.raw(make([]uint8, 6))
	}
	return p.bytes(), errNoErr
}

// parms are the parameters of a file or a folder, with the bitmap for it
func (v *volume) parms(rel string, e entry, fileBitmap uint16, dirBitmap uint16) ([]uint8, int32) {
	if e.info.IsDir() {
		return v.dirParms(rel, e, dirBitmap)
	}
	return v.fileParms(rel, e, fileBitmap)
}

/*
getFileDirParms answers with the parameters of a file or a folder, after the
two bitmaps and whether it is a folder
*/
func (v *volume) getFileDirParms(r *reader) ([]uint8, int32) {
	r.byte()
	if !r.volume() {
		return nil, errParamErr
	}
	dirID := r.uint32()
	fileBitmap := r.uint16()
	dirBitmap := r.uint16()
	name, ok := r.path()
	if !ok {
		return nil, errParamErr
	}

	at, result := v.resolve(dirID, name)
	if result != errNoErr {
		return nil, result
	}
	if !at.exists {
		return nil, errObjectNotFound
	}

	reply := binary.BigEndian.AppendUint16(nil, fileBitmap)
	reply = binary.BigEndian.AppendUint16(reply, dirBitmap)
	if at.found.info.IsDir() {
		reply = append(reply, isDirectoryFlag, 0)
	} else {
		reply = append(reply, 0, 0)
	}
	parms, result := v.parms(at.rel(), at.found, fileBitmap, dirBitmap)
	if result != errNoErr {
		return nil, result
	}
	return append(reply, parms...), errNoErr
}

/*
enumerate lists a folder: from the index asked for, one based, as many as were
asked for and fit in the reply, each with its length, whether it is a folder,
and its parameters. Asking past the end is ObjectNotFound, which is how the
client knows it has them all.
*/
func (v *volume) enumerate(r *reader) ([]uint8, int32) {
	r.byte()
	if !r.volume() {
		return nil, errParamErr
	}
	dirID := r.uint32()
	fileBitmap := r.uint16()
	dirBitmap := r.uint16()
	count := int(r.uint16())
	start := int(r.uint16())
	most := int(r.uint16())
	name, ok := r.path()
	if !ok || start < 1 {
		return nil, errParamErr
	}
	if fileBitmap == 0 && dirBitmap == 0 {
		return nil, errBitmapErr
	}

	at, result := v.resolve(dirID, name)
	if result != errNoErr {
		return nil, result
	}
	if !at.exists {
		return nil, errDirNotFound
	}
	if !at.found.info.IsDir() {
		return nil, errObjectTypeErr
	}
	folder := at.rel()
	entries, result := v.list(folder)
	if result != errNoErr {
		return nil, result
	}

	// Only the files, or the folders, when the other bitmap is empty
	shown := entries[:0:0]
	for _, e := range entries {
		if e.info.IsDir() && dirBitmap != 0 || !e.info.IsDir() && fileBitmap != 0 {
			shown = append(shown, e)
		}
	}

	reply := binary.BigEndian.AppendUint16(nil, fileBitmap)
	reply = binary.BigEndian.AppendUint16(reply, dirBitmap)
	reply = append(reply, 0, 0)
	listed := 0
	for i := start - 1; i < len(shown) && listed < count; i++ {
		e := shown[i]
		parms, result := v.parms(folderJoin(folder, e.host), e, fileBitmap, dirBitmap)
		if result != errNoErr {
			return nil, result
		}

		item := []uint8{0, 0}
		if e.info.IsDir() {
			item[1] = isDirectoryFlag
		}
		item = alignEven(append(item, parms...))
		if len(item) > 0xff || len(reply)+len(item) > most {
			break
		}
		item[0] = uint8(len(item))
		reply = append(reply, item...)
		listed++
	}
	if listed == 0 {
		return nil, errObjectNotFound
	}
	binary.BigEndian.PutUint16(reply[4:], uint16(listed))
	return reply, errNoErr
}

// folderJoin is the path of something in a folder
func folderJoin(folder string, host string) string {
	if folder == "" {
		return host
	}
	return folder + "/" + host
}

// openDir gives the number of a folder; they have them already, and keep them
func (v *volume) openDir(r *reader) ([]uint8, int32) {
	r.byte()
	if !r.volume() {
		return nil, errParamErr
	}
	dirID := r.uint32()
	name, ok := r.path()
	if !ok {
		return nil, errParamErr
	}
	at, result := v.resolve(dirID, name)
	if result != errNoErr {
		return nil, result
	}
	if !at.exists {
		return nil, errObjectNotFound
	}
	if !at.found.info.IsDir() {
		return nil, errObjectTypeErr
	}
	return binary.BigEndian.AppendUint32(nil, v.id(at.rel())), errNoErr
}

/*
setParms changes the parameters of a file or a folder, those that can be: the
attributes, of which the invisible one is kept in the Finder information, the
modification date, and the Finder information. The rest of what can be set is
taken and left as it is: dates of creation and backup the host does not keep,
and owners and access rights there are none of. The parameters start on an
even byte after the path.
*/
func (v *volume) setParms(r *reader, files bool, folders bool) ([]uint8, int32) {
	r.byte()
	if !r.volume() {
		return nil, errParamErr
	}
	dirID := r.uint32()
	bitmap := r.uint16()
	name, ok := r.path()
	r.pad()
	if !ok {
		return nil, errParamErr
	}

	at, result := v.resolve(dirID, name)
	if result != errNoErr {
		return nil, result
	}
	if !at.exists {
		return nil, errObjectNotFound
	}
	isDir := at.found.info.IsDir()
	if isDir && !folders || !isDir && !files {
		return nil, errObjectTypeErr
	}

	settable := uint16(paramAttributes | paramCreated | paramModified | paramBackedUp |
		paramFinderInfo | paramProDOSInfo)
	if isDir {
		settable |= dirOwnerID | dirGroupID | dirAccessRights
	}
	if bitmap&^settable != 0 {
		return nil, errBitmapErr
	}

	rel := at.rel()
	host := v.host(rel)
	finder := v.finderInfo(rel, at.found)
	original := finder

	var attributes uint16
	var modified time.Time
	if bitmap&paramAttributes != 0 {
		attributes = r.uint16()
	}
	if bitmap&paramCreated != 0 {
		r.uint32()
	}
	if bitmap&paramModified != 0 {
		modified = fromAFPTime(r.uint32())
	}
	if bitmap&paramBackedUp != 0 {
		r.uint32()
	}
	if bitmap&paramFinderInfo != 0 {
		copy(finder[:], r.take(32))
	}
	if r.failed {
		return nil, errParamErr
	}

	if attributes&attributeInvisible != 0 {
		flags := binary.BigEndian.Uint16(finder[finderFlagsOffset:])
		if attributes&attributeSetClear != 0 {
			flags |= finderFlagInvisible
		} else {
			flags &^= finderFlagInvisible
		}
		binary.BigEndian.PutUint16(finder[finderFlagsOffset:], flags)
	}

	if finder != original || bitmap&paramFinderInfo != 0 {
		if err := v.meta.setFinderInfo(host, finder); err != nil {
			return nil, hostError(err)
		}
	}
	if !modified.IsZero() {
		if err := os.Chtimes(host, time.Now(), modified); err != nil {
			return nil, hostError(err)
		}
	}
	v.touch()
	return nil, errNoErr
}
