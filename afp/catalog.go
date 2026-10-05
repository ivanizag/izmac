package afp

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

/*
The catalog: the folder of the host as the machine sees it, folders and files
with numbers and Mac OS Roman names.

AFP names a file or a folder by a directory ID and a path from it. The IDs are
handed out here as the machine comes across things, kept for as long as the
server runs, and followed through renames and moves; the root is 2 and its
parent 1, as AFP has them. A path is Pascal string of names separated by a
null each, where every null past the first goes up one folder.

What the host has hidden is left out: names starting with a dot, which are the
AppleDouble files of the files themselves, .DS_Store, and what tools leave
around.
*/

const (
	parentOfRootID = 1
	rootID         = 2
	firstFreeID    = 17

	// The kinds of path: short names are for MS-DOS and ProDOS, and taken
	// as long ones here
	pathShort = 1
	pathLong  = 2
)

// volume is the shared folder
type volume struct {
	name    []uint8
	root    string
	created time.Time
	changed time.Time

	// now is the time of the server, the host's unless it was given one
	now func() time.Time

	byID   map[uint32]string
	byPath map[string]uint32
	nextID uint32

	meta    metadataStore
	desktop *desktop

	// The forks open, by reference number, in every session, and the
	// resource forks among them, held in memory while they are open
	forks     map[uint16]*openFork
	resources map[string]*resourceFork
	nextFork  uint16
}

func newVolume(name []uint8, folder string, now func() time.Time) *volume {
	if absolute, err := filepath.Abs(folder); err == nil {
		folder = absolute
	}
	created := now()
	if info, err := os.Stat(folder); err == nil {
		created = info.ModTime()
	}
	return &volume{
		name:      name,
		root:      folder,
		created:   created,
		now:       now,
		byID:      map[uint32]string{rootID: ""},
		byPath:    map[string]uint32{"": rootID},
		nextID:    firstFreeID,
		meta:      newMetadataStore(folder),
		desktop:   newDesktop(),
		forks:     make(map[uint16]*openFork),
		resources: make(map[string]*resourceFork),
	}
}

// host is where something of the volume is on the host; rel is its path from
// the root, with slashes
func (v *volume) host(rel string) string {
	return filepath.Join(v.root, filepath.FromSlash(rel))
}

// id is the number of something of the volume, given one the first time
func (v *volume) id(rel string) uint32 {
	if id, ok := v.byPath[rel]; ok {
		return id
	}
	id := v.nextID
	v.nextID++
	v.byID[id] = rel
	v.byPath[rel] = id
	return id
}

// moved follows something and everything in it to where it went
func (v *volume) moved(from string, to string) {
	for rel, id := range v.byPath {
		if rest, ok := within(rel, from); ok {
			delete(v.byPath, rel)
			now := to + rest
			v.byPath[now] = id
			v.byID[id] = now
		}
	}
	v.movedForks(from, to)
}

// removed forgets something and everything in it
func (v *volume) removed(rel string) {
	for p, id := range v.byPath {
		if _, ok := within(p, rel); ok {
			delete(v.byPath, p)
			delete(v.byID, id)
		}
	}
}

// within tells whether a path is a folder or in it, and gives the rest of it
func within(rel string, folder string) (string, bool) {
	if rel == folder {
		return "", true
	}
	if folder == "" {
		return "/" + rel, true
	}
	if strings.HasPrefix(rel, folder+"/") {
		return rel[len(folder):], true
	}
	return "", false
}

// entry is a file or a folder in a folder
type entry struct {
	host string
	name []uint8
	info fs.FileInfo
}

// hidden tells whether a name of the host is left out of the volume
func hidden(name string) bool {
	return strings.HasPrefix(name, ".")
}

/*
list is what is in a folder, sorted by name. A name that the machine cannot
have as it is, or that is the same as another one but for case, which a host
can have and the machine cannot, is made unique.
*/
func (v *volume) list(rel string) ([]entry, int32) {
	dir, err := os.ReadDir(v.host(rel))
	if err != nil {
		return nil, hostError(err)
	}

	entries := make([]entry, 0, len(dir))
	var inexact []int
	for _, d := range dir {
		if hidden(d.Name()) {
			continue
		}
		info, err := os.Stat(filepath.Join(v.host(rel), d.Name()))
		if err != nil || !info.IsDir() && !info.Mode().IsRegular() {
			continue
		}
		name, exact := catalogName(d.Name())
		if !exact {
			inexact = append(inexact, len(entries))
		}
		entries = append(entries, entry{host: d.Name(), name: name, info: info})
	}

	for _, i := range inexact {
		entries[i].name = mangledName(entries[i].name, entries[i].host)
	}
	taken := make(map[string]bool, len(entries))
	for i := range entries {
		order := nameOrder(entries[i].name)
		if taken[order] {
			entries[i].name = mangledName(entries[i].name, entries[i].host)
			order = nameOrder(entries[i].name)
		}
		taken[order] = true
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return nameOrder(entries[i].name) < nameOrder(entries[j].name)
	})
	return entries, errNoErr
}

// lookup finds a name in a folder
func (v *volume) lookup(rel string, name []uint8) (entry, bool, int32) {
	entries, result := v.list(rel)
	if result != errNoErr {
		return entry{}, false, result
	}
	for _, e := range entries {
		if sameName(e.name, name) {
			return e, true, errNoErr
		}
	}
	return entry{}, false, errNoErr
}

/*
object is something of the volume by its path, with the name the machine has
for it: the volume's own for the root
*/
func (v *volume) object(rel string) (entry, int32) {
	info, err := os.Stat(v.host(rel))
	if err != nil {
		return entry{}, hostError(err)
	}
	if rel == "" {
		return entry{name: v.name, info: info}, errNoErr
	}

	parent, base := parentOf(rel)
	entries, result := v.list(parent)
	if result != errNoErr {
		return entry{}, result
	}
	for _, e := range entries {
		if e.host == base {
			return e, errNoErr
		}
	}
	return entry{}, errObjectNotFound
}

// parentOf splits a path into its folder and its name
func parentOf(rel string) (string, string) {
	parent, base := path.Split(rel)
	return strings.TrimSuffix(parent, "/"), base
}

// parentID is the number of the folder something is in
func (v *volume) parentID(rel string) uint32 {
	if rel == "" {
		return parentOfRootID
	}
	parent, _ := parentOf(rel)
	return v.id(parent)
}

/*
place is where a path leads: the folder it ends in and the last name in it,
which may not be there yet, as for something to be made. A path that ends in
a folder itself, with no name after it, has no name, and the folder is the
place.
*/
type place struct {
	folder string
	name   []uint8
	found  entry
	exists bool
}

// rel is the path of what the place names, which is only one if it exists
func (p place) rel() string {
	if p.name == nil {
		return p.folder
	}
	return path.Join(p.folder, p.found.host)
}

// readPath reads the kind of path and the path
func (r *reader) path() ([]uint8, bool) {
	kind := r.byte()
	name := r.pascal()
	return name, !r.failed && (kind == pathShort || kind == pathLong)
}

/*
resolve follows a path from a folder given by number. What it leads to is
given a number if it has none: the machine knows of it now, and its changes on
the host are watched, see modified.
*/
func (v *volume) resolve(dirID uint32, p []uint8) (place, int32) {
	at, result := v.follow(dirID, p)
	if result == errNoErr && at.exists {
		v.id(at.rel())
	}
	return at, result
}

// follow follows a path from a folder given by number
func (v *volume) follow(dirID uint32, p []uint8) (place, int32) {
	var folder string
	atParentOfRoot := false
	switch dirID {
	case parentOfRootID:
		atParentOfRoot = true
	default:
		rel, ok := v.byID[dirID]
		if !ok {
			return place{}, errObjectNotFound
		}
		folder = rel
	}

	parts := splitPath(p)
	for i, part := range parts {
		last := i == len(parts)-1

		if part == nil {
			if atParentOfRoot {
				return place{}, errObjectNotFound
			}
			if folder == "" {
				atParentOfRoot = true
			} else {
				folder, _ = parentOf(folder)
			}
			continue
		}

		if atParentOfRoot {
			// Above the root there is the root, by the volume's name
			if !sameName(part, v.name) {
				return place{}, errObjectNotFound
			}
			atParentOfRoot = false
			folder = ""
			continue
		}

		e, ok, result := v.lookup(folder, part)
		if result != errNoErr {
			return place{}, result
		}
		if last {
			return place{folder: folder, name: part, found: e, exists: ok}, errNoErr
		}
		if !ok || !e.info.IsDir() {
			return place{}, errObjectNotFound
		}
		folder = path.Join(folder, e.host)
	}

	if atParentOfRoot {
		return place{}, errObjectNotFound
	}
	e, result := v.object(folder)
	if result != errNoErr {
		return place{}, result
	}
	return place{folder: folder, found: e, exists: true}, errNoErr
}

/*
splitPath cuts a path into its names, with a nil for each step up: one null
separates two names, and each one after it goes up a folder
*/
func splitPath(p []uint8) [][]uint8 {
	var parts [][]uint8
	i := 0
	for i < len(p) {
		if p[i] == 0 {
			i++
			for i < len(p) && p[i] == 0 {
				parts = append(parts, nil)
				i++
			}
			continue
		}
		start := i
		for i < len(p) && p[i] != 0 {
			i++
		}
		parts = append(parts, p[start:i])
	}
	return parts
}

// hostError is the result code for what the host said went wrong
func hostError(err error) int32 {
	switch {
	case err == nil:
		return errNoErr
	case os.IsNotExist(err):
		return errObjectNotFound
	case os.IsPermission(err):
		return errAccessDenied
	case os.IsExist(err):
		return errObjectExists
	}
	return errMiscErr
}
