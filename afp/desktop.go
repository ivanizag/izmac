package afp

import (
	"encoding/binary"
	"os"
)

/*
The desktop database: what the Finder keeps in the Desktop file of a disk of
its own, it asks a file server for. The icons of the files of each
application, which application opens the documents of each creator, and the
comments of Get Info.

It is kept in memory for as long as the server runs. The first time it is
opened, the volume is gone through for the applications on it, and their
bundles give the icons and the applications, which is what the Finder does to
rebuild the Desktop file of a disk; from then on the Finder adds what it
copies.
*/

const (
	desktopRef = 1

	// The most files gone through looking for applications, and how deep
	scanMostFiles = 20000
	scanDeepest   = 12
)

type icon struct {
	creator  [4]uint8
	fileType [4]uint8
	kind     uint8
	tag      uint32
	data     []uint8
}

type application struct {
	creator [4]uint8
	rel     string
	tag     uint32
}

type desktop struct {
	icons        []icon
	applications []application
	comments     map[string][]uint8
	scanned      bool
}

func newDesktop() *desktop {
	return &desktop{comments: make(map[string][]uint8)}
}

// readDesktop reads the reference number of the desktop database, which has
// to be the one there is
func (r *reader) desktop() bool {
	r.byte()
	return r.uint16() == desktopRef
}

func (r *reader) fourCC() [4]uint8 {
	var code [4]uint8
	copy(code[:], r.take(4))
	return code
}

// open opens the database, which is looked for the first time
func (d *desktop) open(v *volume) ([]uint8, int32) {
	if !d.scanned {
		d.scanned = true
		d.scan(v)
	}
	return binary.BigEndian.AppendUint16(nil, desktopRef), errNoErr
}

// moved and forget follow what moved and went
func (d *desktop) moved(from string, to string) {
	for rel, comment := range d.comments {
		if rest, ok := within(rel, from); ok {
			delete(d.comments, rel)
			d.comments[to+rest] = comment
		}
	}
	for i := range d.applications {
		if rest, ok := within(d.applications[i].rel, from); ok {
			d.applications[i].rel = to + rest
		}
	}
}

func (d *desktop) forget(rel string) {
	delete(d.comments, rel)
	kept := d.applications[:0]
	for _, a := range d.applications {
		if a.rel != rel {
			kept = append(kept, a)
		}
	}
	d.applications = kept
}

// addIcon keeps an icon, which comes as the data of a write
func (d *desktop) addIcon(r *reader, data []uint8) ([]uint8, int32) {
	if !r.desktop() {
		return nil, errParamErr
	}
	i := icon{creator: r.fourCC(), fileType: r.fourCC(), kind: r.byte()}
	r.byte()
	i.tag = r.uint32()
	size := int(r.uint16())
	if r.failed {
		return nil, errParamErr
	}
	i.data = append([]uint8{}, data[:min(size, len(data))]...)
	d.putIcon(i)
	return nil, errNoErr
}

// putIcon keeps an icon in place of the one there was for the same thing
func (d *desktop) putIcon(i icon) {
	for n := range d.icons {
		old := &d.icons[n]
		if old.creator == i.creator && old.fileType == i.fileType && old.kind == i.kind {
			*old = i
			return
		}
	}
	d.icons = append(d.icons, i)
}

// getIcon answers with an icon, as much of it as was asked for
func (d *desktop) getIcon(r *reader) ([]uint8, int32) {
	if !r.desktop() {
		return nil, errParamErr
	}
	creator, fileType, kind := r.fourCC(), r.fourCC(), r.byte()
	r.byte()
	size := int(r.uint16())
	if r.failed {
		return nil, errParamErr
	}
	for _, i := range d.icons {
		if i.creator == creator && i.fileType == fileType && i.kind == kind {
			return i.data[:min(size, len(i.data))], errNoErr
		}
	}
	return nil, errItemNotFound
}

// getIconInfo answers with what an icon of a creator is, by its index
func (d *desktop) getIconInfo(r *reader) ([]uint8, int32) {
	if !r.desktop() {
		return nil, errParamErr
	}
	creator := r.fourCC()
	index := int(r.uint16())
	if r.failed {
		return nil, errParamErr
	}
	for _, i := range d.icons {
		if i.creator != creator {
			continue
		}
		index--
		if index == 0 {
			reply := binary.BigEndian.AppendUint32(nil, i.tag)
			reply = append(reply, i.fileType[:]...)
			reply = append(reply, i.kind, 0)
			return binary.BigEndian.AppendUint16(reply, uint16(len(i.data))), errNoErr
		}
	}
	return nil, errItemNotFound
}

// addAPPL says which application opens the documents of a creator
func (d *desktop) addAPPL(v *volume, r *reader) ([]uint8, int32) {
	if !r.desktop() {
		return nil, errParamErr
	}
	dirID := r.uint32()
	creator := r.fourCC()
	tag := r.uint32()
	name, ok := r.path()
	if !ok {
		return nil, errParamErr
	}
	at, result := v.resolve(dirID, name)
	if result != errNoErr {
		return nil, result
	}
	if !at.exists || at.found.info.IsDir() {
		return nil, errObjectNotFound
	}
	d.putApplication(application{creator: creator, rel: at.rel(), tag: tag})
	return nil, errNoErr
}

func (d *desktop) putApplication(a application) {
	for n, old := range d.applications {
		if old.creator == a.creator && old.rel == a.rel {
			d.applications[n] = a
			return
		}
	}
	d.applications = append(d.applications, a)
}

func (d *desktop) removeAPPL(v *volume, r *reader) ([]uint8, int32) {
	if !r.desktop() {
		return nil, errParamErr
	}
	dirID := r.uint32()
	creator := r.fourCC()
	name, ok := r.path()
	if !ok {
		return nil, errParamErr
	}
	at, result := v.resolve(dirID, name)
	if result != errNoErr {
		return nil, result
	}
	for n, a := range d.applications {
		if a.creator == creator && at.exists && a.rel == at.rel() {
			d.applications = append(d.applications[:n], d.applications[n+1:]...)
			return nil, errNoErr
		}
	}
	return nil, errItemNotFound
}

/*
getAPPL answers with an application for the documents of a creator, by its
index, with the parameters of its file. An application that is not there any
more is passed over.
*/
func (d *desktop) getAPPL(v *volume, r *reader) ([]uint8, int32) {
	if !r.desktop() {
		return nil, errParamErr
	}
	creator := r.fourCC()
	index := int(r.uint16())
	bitmap := r.uint16()
	if r.failed {
		return nil, errParamErr
	}
	index = max(index, 1)
	for _, a := range d.applications {
		if a.creator != creator {
			continue
		}
		e, result := v.object(a.rel)
		if result != errNoErr || e.info.IsDir() {
			continue
		}
		index--
		if index > 0 {
			continue
		}
		parms, result := v.fileParms(a.rel, e, bitmap)
		if result != errNoErr {
			return nil, result
		}
		reply := binary.BigEndian.AppendUint16(nil, bitmap)
		reply = binary.BigEndian.AppendUint32(reply, a.tag)
		return append(reply, parms...), errNoErr
	}
	return nil, errItemNotFound
}

// The comments, by the path of what they are of
func (d *desktop) addComment(v *volume, r *reader) ([]uint8, int32) {
	if !r.desktop() {
		return nil, errParamErr
	}
	dirID := r.uint32()
	name, ok := r.path()
	r.pad()
	comment := r.pascal()
	if !ok || r.failed {
		return nil, errParamErr
	}
	at, result := v.resolve(dirID, name)
	if result != errNoErr {
		return nil, result
	}
	if !at.exists {
		return nil, errObjectNotFound
	}
	d.comments[at.rel()] = append([]uint8{}, comment...)
	return nil, errNoErr
}

func (d *desktop) removeComment(v *volume, r *reader) ([]uint8, int32) {
	if !r.desktop() {
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
	delete(d.comments, at.rel())
	return nil, errNoErr
}

func (d *desktop) getComment(v *volume, r *reader) ([]uint8, int32) {
	if !r.desktop() {
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
	comment, ok := d.comments[at.rel()]
	if !ok {
		return nil, errItemNotFound
	}
	return appendPascal(nil, comment), errNoErr
}

/*
scan goes through the volume for applications, and takes from the bundle of
each the icons of its files and that it opens the documents of its creator
*/
func (d *desktop) scan(v *volume) {
	seen := 0
	var walk func(rel string, depth int)
	walk = func(rel string, depth int) {
		dir, err := os.ReadDir(v.host(rel))
		if err != nil {
			return
		}
		for _, entry := range dir {
			if seen >= scanMostFiles || hidden(entry.Name()) {
				return
			}
			seen++
			child := folderJoin(rel, entry.Name())
			if entry.IsDir() {
				if depth < scanDeepest {
					walk(child, depth+1)
				}
				continue
			}
			finder, ok := v.meta.finderInfo(v.host(child))
			if !ok || string(finder[0:4]) != "APPL" {
				continue
			}
			var creator [4]uint8
			copy(creator[:], finder[4:8])
			d.putApplication(application{creator: creator, rel: child})
			for _, i := range bundleIcons(parseResources(v.meta.resource(v.host(child)))) {
				d.putIcon(i)
			}
		}
	}
	walk("", 0)
}

// iconKind is a kind of icon, by the resource type it is kept in
type iconKind struct {
	resourceType string
	kind         uint8
}

func iconKinds() []iconKind {
	return []iconKind{
		{"ICN#", 1}, {"icl4", 2}, {"icl8", 3}, {"ics#", 4}, {"ics4", 5}, {"ics8", 6},
	}
}

/*
bundleIcons are the icons an application's bundle gives its files. The BNDL
names its creator and maps the local IDs of its FREFs and its icons to
resource IDs; each FREF is a file type and the local ID of its icon, whose
resource ID is the same for every kind of icon.

	BNDL  creator(4) id(2) types-1(2), and for each type:
	      type(4) count-1(2) and count pairs of local id(2) resource id(2)
	FREF  file type(4) local icon id(2) name
*/
func bundleIcons(res resources) []icon {
	var icons []icon
	for _, bundle := range res["BNDL"] {
		if len(bundle) < 8 {
			continue
		}
		var creator [4]uint8
		copy(creator[:], bundle[0:4])

		maps := make(map[string]map[int16]int16)
		r := reader{data: bundle, at: 6}
		types := int(int16(r.uint16())) + 1
		for t := 0; t < types && !r.failed; t++ {
			resourceType := string(r.take(4))
			count := int(int16(r.uint16())) + 1
			ids := make(map[int16]int16)
			for i := 0; i < count && !r.failed; i++ {
				local := int16(r.uint16())
				ids[local] = int16(r.uint16())
			}
			maps[resourceType] = ids
		}

		for _, id := range maps["FREF"] {
			fref, ok := res["FREF"][id]
			if !ok || len(fref) < 6 {
				continue
			}
			var fileType [4]uint8
			copy(fileType[:], fref[0:4])
			iconID, ok := maps["ICN#"][int16(binary.BigEndian.Uint16(fref[4:]))]
			if !ok {
				continue
			}
			for _, k := range iconKinds() {
				if data, ok := res[k.resourceType][iconID]; ok {
					icons = append(icons, icon{creator: creator, fileType: fileType, kind: k.kind, data: data})
				}
			}
		}
	}
	return icons
}

// resources are the resources of a fork, by type and ID
type resources map[string]map[int16][]uint8

/*
parseResources reads a resource fork: a header with where the data and the map
are, and in the map the types, each with its list of references, each an ID,
and where the resource is after the start of the data, past its length.
*/
func parseResources(fork []uint8) resources {
	res := make(resources)
	if len(fork) < 16 {
		return res
	}
	dataStart := int(binary.BigEndian.Uint32(fork[0:]))
	mapStart := int(binary.BigEndian.Uint32(fork[4:]))
	if mapStart+30 > len(fork) {
		return res
	}
	typeList := mapStart + int(binary.BigEndian.Uint16(fork[mapStart+24:]))
	if typeList+2 > len(fork) {
		return res
	}
	types := int(int16(binary.BigEndian.Uint16(fork[typeList:]))) + 1
	for t := 0; t < types; t++ {
		at := typeList + 2 + 8*t
		if at+8 > len(fork) {
			break
		}
		resourceType := string(fork[at : at+4])
		count := int(binary.BigEndian.Uint16(fork[at+4:])) + 1
		refs := typeList + int(binary.BigEndian.Uint16(fork[at+6:]))
		byID := make(map[int16][]uint8)
		for i := 0; i < count; i++ {
			ref := refs + 12*i
			if ref+12 > len(fork) {
				break
			}
			id := int16(binary.BigEndian.Uint16(fork[ref:]))
			offset := dataStart + int(binary.BigEndian.Uint32(fork[ref+4:])&0xffffff)
			if offset+4 > len(fork) {
				continue
			}
			length := int(binary.BigEndian.Uint32(fork[offset:]))
			if offset+4+length > len(fork) {
				continue
			}
			byID[id] = fork[offset+4 : offset+4+length]
		}
		res[resourceType] = byID
	}
	return res
}
