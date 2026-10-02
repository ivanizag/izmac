package afp

import (
	"encoding/binary"
	"os"
	"strings"
)

/*
What changes the catalog: making files and folders, deleting them, renaming
and moving them. A name the machine gives is a name on the host as well, see
hostFileName, and is refused when the folder already has one like it but for
case, which the machine would not tell apart.
*/

// validName tells whether a name can be given to a file or a folder
func validName(name []uint8) bool {
	return len(name) > 0 && len(name) <= longestName
}

/*
createFile makes an empty file. A soft create fails if the file is there; a
hard one empties it, unless it is open.
*/
func (v *volume) createFile(r *reader) ([]uint8, int32) {
	flag := r.byte()
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
	if at.name == nil {
		return nil, errObjectExists
	}

	if at.exists {
		if flag&0x80 == 0 || at.found.info.IsDir() {
			return nil, errObjectExists
		}
		rel := at.rel()
		if v.busy(rel) {
			return nil, errFileBusy
		}
		host := v.host(rel)
		if err := os.Truncate(host, 0); err != nil {
			return nil, hostError(err)
		}
		v.meta.setResource(host, nil)
		v.meta.setFinderInfo(host, [32]uint8{})
		v.touch()
		return nil, errNoErr
	}

	if !validName(at.name) {
		return nil, errParamErr
	}
	host := v.host(folderJoin(at.folder, hostFileName(at.name)))
	file, err := os.OpenFile(host, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, hostError(err)
	}
	file.Close()
	v.touch()
	return nil, errNoErr
}

// createDir makes a folder, and answers with its number
func (v *volume) createDir(r *reader) ([]uint8, int32) {
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
	if at.exists {
		return nil, errObjectExists
	}
	if !validName(at.name) {
		return nil, errParamErr
	}

	rel := folderJoin(at.folder, hostFileName(at.name))
	if err := os.Mkdir(v.host(rel), 0o755); err != nil {
		return nil, hostError(err)
	}
	v.touch()
	return binary.BigEndian.AppendUint32(nil, v.id(rel)), errNoErr
}

/*
delete deletes a file, which nobody may have open, or a folder, which has to
be empty. A folder the machine sees as empty may still have what the host
hides, which goes with it if it is what the host leaves in folders on its own:
AppleDouble files and .DS_Store.
*/
func (v *volume) delete(r *reader) ([]uint8, int32) {
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
	rel := at.rel()
	if rel == "" {
		return nil, errAccessDenied
	}
	host := v.host(rel)

	if at.found.info.IsDir() {
		entries, result := v.list(rel)
		if result != errNoErr {
			return nil, result
		}
		if len(entries) != 0 {
			return nil, errDirNotEmpty
		}
		clearClutter(host)
		if err := os.Remove(host); err != nil {
			if os.IsExist(err) || isNotEmpty(host) {
				return nil, errDirNotEmpty
			}
			return nil, hostError(err)
		}
	} else {
		if v.busy(rel) {
			return nil, errFileBusy
		}
		if err := os.Remove(host); err != nil {
			return nil, hostError(err)
		}
	}
	v.meta.removed(host)
	v.removed(rel)
	v.desktop.forget(rel)
	v.touch()
	return nil, errNoErr
}

// clearClutter removes what the host leaves in a folder on its own
func clearClutter(host string) {
	dir, err := os.ReadDir(host)
	if err != nil {
		return
	}
	for _, d := range dir {
		if d.Name() == ".DS_Store" || strings.HasPrefix(d.Name(), appleDoublePrefix) {
			os.Remove(host + string(os.PathSeparator) + d.Name())
		}
	}
}

func isNotEmpty(host string) bool {
	dir, err := os.ReadDir(host)
	return err == nil && len(dir) > 0
}

// newName reads a name to rename to, which comes as a path of one name
func (r *reader) newName() ([]uint8, bool) {
	name, ok := r.path()
	return name, ok && !strings.ContainsRune(string(name), 0)
}

// rename renames a file or a folder where it is
func (v *volume) rename(r *reader) ([]uint8, int32) {
	r.byte()
	if !r.volume() {
		return nil, errParamErr
	}
	dirID := r.uint32()
	name, ok := r.path()
	newName, newOK := r.newName()
	if !ok || !newOK {
		return nil, errParamErr
	}
	at, result := v.resolve(dirID, name)
	if result != errNoErr {
		return nil, result
	}
	if !at.exists {
		return nil, errObjectNotFound
	}
	rel := at.rel()
	if rel == "" {
		return nil, errCantRename
	}
	folder, _ := parentOf(rel)
	return nil, v.move(rel, at.found, folder, newName)
}

/*
moveAndRename moves a file or a folder to another folder, and renames it on
the way if a name is given. A folder cannot go into itself, or into a folder
in it.
*/
func (v *volume) moveAndRename(r *reader) ([]uint8, int32) {
	r.byte()
	if !r.volume() {
		return nil, errParamErr
	}
	fromID := r.uint32()
	toID := r.uint32()
	name, ok := r.path()
	toName, toOK := r.path()
	newName, newOK := r.newName()
	if !ok || !toOK || !newOK {
		return nil, errParamErr
	}

	from, result := v.resolve(fromID, name)
	if result != errNoErr {
		return nil, result
	}
	if !from.exists {
		return nil, errObjectNotFound
	}
	to, result := v.resolve(toID, toName)
	if result != errNoErr {
		return nil, result
	}
	if !to.exists || !to.found.info.IsDir() {
		return nil, errObjectNotFound
	}

	rel := from.rel()
	if rel == "" {
		return nil, errCantMove
	}
	destination := to.rel()
	if _, inside := within(destination, rel); inside && from.found.info.IsDir() {
		return nil, errCantMove
	}
	if len(newName) == 0 {
		newName = from.found.name
	}
	return nil, v.move(rel, from.found, destination, newName)
}

// move puts something in a folder under a name, which is renaming it when
// the folder is the one it is in
func (v *volume) move(rel string, e entry, folder string, name []uint8) int32 {
	if !validName(name) {
		return errParamErr
	}
	parent, _ := parentOf(rel)
	there, ok, result := v.lookup(folder, name)
	if result != errNoErr {
		return result
	}
	if ok && !(folder == parent && there.host == e.host) {
		return errObjectExists
	}

	to := folderJoin(folder, hostFileName(name))
	if to == rel {
		return errNoErr
	}
	if err := os.Rename(v.host(rel), v.host(to)); err != nil {
		return hostError(err)
	}
	v.meta.renamed(v.host(rel), v.host(to))
	v.moved(rel, to)
	v.desktop.moved(rel, to)
	v.touch()
	return errNoErr
}
