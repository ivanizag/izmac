package afp

import (
	"encoding/binary"
	"io"
	"os"
	"strings"
)

/*
The forks of files, opened by a session and read and written by the reference
number it gets for them.

The data fork is the file of the host, read and written where it is. The
resource fork is wherever the host keeps it, which is a whole thing to read
and write, so it is held in memory while it is open, one copy for everyone who
has it open, and written back when it is flushed or the last one closes it.

The access modes say what the opener does and what it lets others do, and an
open that asks for what another one denies is refused.
*/

const (
	accessRead      = 1 << 0
	accessWrite     = 1 << 1
	accessDenyRead  = 1 << 4
	accessDenyWrite = 1 << 5

	resourceForkFlag = 0x80
	fromEndFlag      = 0x80
	unlockFlag       = 0x01

	// quantumSize is the most a reply can carry, eight packets of ATP
	quantumSize = 8 * 578
)

// openFork is a fork open in a session
type openFork struct {
	session  int
	rel      string
	resource bool
	access   uint16
	file     *os.File
	shared   *resourceFork
}

// resourceFork is a resource fork while it is open
type resourceFork struct {
	rel   string
	data  []uint8
	dirty bool
	users int
}

// length is how long the fork is
func (f *openFork) length() int64 {
	if f.resource {
		return int64(len(f.shared.data))
	}
	info, err := f.file.Stat()
	if err != nil {
		return 0
	}
	return info.Size()
}

// openForks tells whether a file has its data fork or its resource fork open
func (v *volume) openForks(rel string) (bool, bool) {
	data, resource := false, false
	for _, f := range v.forks {
		if f.rel == rel {
			if f.resource {
				resource = true
			} else {
				data = true
			}
		}
	}
	return data, resource
}

// resourceLength is how long a resource fork is, open or not
func (v *volume) resourceLength(rel string) int64 {
	if shared, ok := v.resources[rel]; ok {
		return int64(len(shared.data))
	}
	return v.meta.resourceLength(v.host(rel))
}

// movedForks follows the open forks of files that moved
func (v *volume) movedForks(from string, to string) {
	for _, f := range v.forks {
		if rest, ok := within(f.rel, from); ok {
			f.rel = to + rest
		}
	}
	for rel, shared := range v.resources {
		if rest, ok := within(rel, from); ok {
			delete(v.resources, rel)
			shared.rel = to + rest
			v.resources[shared.rel] = shared
		}
	}
}

// conflicts tells whether an open asks for what another one denies, or
// denies what another one has
func (v *volume) conflicts(rel string, resource bool, access uint16) bool {
	for _, f := range v.forks {
		if f.rel != rel || f.resource != resource {
			continue
		}
		if access&accessRead != 0 && f.access&accessDenyRead != 0 ||
			access&accessWrite != 0 && f.access&accessDenyWrite != 0 ||
			access&accessDenyRead != 0 && f.access&accessRead != 0 ||
			access&accessDenyWrite != 0 && f.access&accessWrite != 0 {
			return true
		}
	}
	return false
}

// openFork opens a fork, and answers with its reference number and the
// parameters of the file
func (v *volume) openFork(r *reader, session int) ([]uint8, int32) {
	flag := r.byte()
	if !r.volume() {
		return nil, errParamErr
	}
	dirID := r.uint32()
	bitmap := r.uint16()
	access := r.uint16()
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
	if at.found.info.IsDir() {
		return nil, errObjectTypeErr
	}
	rel := at.rel()
	resource := flag&resourceForkFlag != 0

	parms, result := v.fileParms(rel, at.found, bitmap)
	if result != errNoErr {
		return nil, result
	}
	if access&accessWrite != 0 && at.found.info.Mode().Perm()&0o200 == 0 {
		return nil, errAccessDenied
	}
	if v.conflicts(rel, resource, access) {
		return nil, errDenyConflict
	}

	f := &openFork{session: session, rel: rel, resource: resource, access: access}
	if resource {
		shared, ok := v.resources[rel]
		if !ok {
			shared = &resourceFork{rel: rel, data: v.meta.resource(v.host(rel))}
			v.resources[rel] = shared
		}
		shared.users++
		f.shared = shared
	} else {
		mode := os.O_RDONLY
		if access&accessWrite != 0 {
			mode = os.O_RDWR
		}
		file, err := os.OpenFile(v.host(rel), mode, 0)
		if err != nil {
			return nil, hostError(err)
		}
		f.file = file
	}

	ref := v.newForkRef()
	v.forks[ref] = f
	reply := binary.BigEndian.AppendUint16(nil, bitmap)
	reply = binary.BigEndian.AppendUint16(reply, ref)
	return append(reply, parms...), errNoErr
}

// newForkRef is a reference number no open fork has
func (v *volume) newForkRef() uint16 {
	for {
		v.nextFork++
		if _, taken := v.forks[v.nextFork]; !taken && v.nextFork != 0 {
			return v.nextFork
		}
	}
}

// fork reads the reference number of a fork, which has to be one the session
// has open
func (v *volume) fork(r *reader, session int) (*openFork, uint16, bool) {
	ref := r.uint16()
	f, ok := v.forks[ref]
	if !ok || f.session != session {
		return nil, ref, false
	}
	return f, ref, true
}

/*
read reads a fork from an offset, as much as was asked for and fits in a
reply, or up to the first byte that, masked, is the newline character asked
for. Reading up to the end of the fork gives what there was and EOFErr.
*/
func (v *volume) read(r *reader, session int) ([]uint8, int32) {
	r.byte()
	f, _, ok := v.fork(r, session)
	offset := int64(int32(r.uint32()))
	count := int64(int32(r.uint32()))
	mask := r.byte()
	newline := r.byte()
	if !ok || r.failed || offset < 0 || count < 0 {
		return nil, errParamErr
	}
	if f.access&accessRead == 0 {
		return nil, errAccessDenied
	}

	data := make([]uint8, min(count, quantumSize))
	var n int
	if f.resource {
		if offset < int64(len(f.shared.data)) {
			n = copy(data, f.shared.data[offset:])
		}
	} else {
		var err error
		n, err = f.file.ReadAt(data, offset)
		if err != nil && err != io.EOF {
			return nil, hostError(err)
		}
	}
	data = data[:n]

	if mask != 0 {
		for i, b := range data {
			if b&mask == newline {
				return data[:i+1], errNoErr
			}
		}
	}
	if int64(n) < count && offset+int64(n) >= f.length() {
		return data, errEOFErr
	}
	return data, errNoErr
}

// write writes data to a fork, at an offset from its start or its end, and
// answers with where the data ended
func (v *volume) write(r *reader, session int, data []uint8) ([]uint8, int32) {
	flag := r.byte()
	f, _, ok := v.fork(r, session)
	offset := int64(int32(r.uint32()))
	count := int64(int32(r.uint32()))
	if !ok || r.failed || count < 0 {
		return nil, errParamErr
	}
	if f.access&accessWrite == 0 {
		return nil, errAccessDenied
	}
	if flag&fromEndFlag != 0 {
		offset += f.length()
	}
	if offset < 0 {
		return nil, errParamErr
	}
	if int64(len(data)) > count {
		data = data[:count]
	}

	if f.resource {
		shared := f.shared
		if end := offset + int64(len(data)); end > int64(len(shared.data)) {
			shared.data = append(shared.data, make([]uint8, end-int64(len(shared.data)))...)
		}
		copy(shared.data[offset:], data)
		shared.dirty = true
	} else if _, err := f.file.WriteAt(data, offset); err != nil {
		return nil, hostError(err)
	}
	v.touch(f.rel)
	return binary.BigEndian.AppendUint32(nil, uint32(offset+int64(len(data)))), errNoErr
}

// flush writes back a resource fork that was written to
func (v *volume) flush(shared *resourceFork) int32 {
	if !shared.dirty {
		return errNoErr
	}
	if err := v.meta.setResource(v.host(shared.rel), shared.data); err != nil {
		return hostError(err)
	}
	v.touch(shared.rel)
	shared.dirty = false
	return errNoErr
}

func (v *volume) flushFork(r *reader, session int) ([]uint8, int32) {
	r.byte()
	f, _, ok := v.fork(r, session)
	if !ok {
		return nil, errParamErr
	}
	if f.resource {
		return nil, v.flush(f.shared)
	}
	return nil, errNoErr
}

func (v *volume) closeFork(r *reader, session int) ([]uint8, int32) {
	r.byte()
	f, ref, ok := v.fork(r, session)
	if !ok {
		return nil, errParamErr
	}
	return nil, v.close(ref, f)
}

// close closes a fork, and writes back a resource fork nobody has open any
// more
func (v *volume) close(ref uint16, f *openFork) int32 {
	delete(v.forks, ref)
	if !f.resource {
		return hostError(f.file.Close())
	}
	f.shared.users--
	result := v.flush(f.shared)
	if f.shared.users == 0 {
		delete(v.resources, f.shared.rel)
	}
	return result
}

// closeForks closes what a session left open
func (v *volume) closeForks(session int) {
	for ref, f := range v.forks {
		if f.session == session {
			v.close(ref, f)
		}
	}
}

func (v *volume) getForkParms(r *reader, session int) ([]uint8, int32) {
	r.byte()
	f, _, ok := v.fork(r, session)
	bitmap := r.uint16()
	if !ok || r.failed {
		return nil, errParamErr
	}
	e, result := v.object(f.rel)
	if result != errNoErr {
		return nil, result
	}
	parms, result := v.fileParms(f.rel, e, bitmap)
	if result != errNoErr {
		return nil, result
	}
	return append(binary.BigEndian.AppendUint16(nil, bitmap), parms...), errNoErr
}

// setForkParms sets the length of a fork, cutting it or making it longer
func (v *volume) setForkParms(r *reader, session int) ([]uint8, int32) {
	r.byte()
	f, _, ok := v.fork(r, session)
	bitmap := r.uint16()
	length := int64(int32(r.uint32()))
	if !ok || r.failed || length < 0 {
		return nil, errParamErr
	}
	if bitmap != fileDataLength && bitmap != fileResourceLength {
		return nil, errBitmapErr
	}
	if f.access&accessWrite == 0 {
		return nil, errAccessDenied
	}

	if f.resource {
		shared := f.shared
		if length <= int64(len(shared.data)) {
			shared.data = shared.data[:length]
		} else {
			shared.data = append(shared.data, make([]uint8, length-int64(len(shared.data)))...)
		}
		shared.dirty = true
	} else if err := f.file.Truncate(length); err != nil {
		return nil, hostError(err)
	}
	v.touch(f.rel)
	return nil, errNoErr
}

/*
byteRangeLock would lock a range of a fork from the other sessions. No locks
are kept: it answers with where the range starts, as if it had locked it,
which is what the programs that ask need to go on.
*/
func (v *volume) byteRangeLock(r *reader, session int) ([]uint8, int32) {
	flags := r.byte()
	f, _, ok := v.fork(r, session)
	offset := int64(int32(r.uint32()))
	r.uint32()
	if !ok || r.failed {
		return nil, errParamErr
	}
	if flags&fromEndFlag != 0 && flags&unlockFlag == 0 {
		offset += f.length()
	}
	return binary.BigEndian.AppendUint32(nil, uint32(offset)), errNoErr
}

// busy tells whether a file, or anything in a folder, has a fork open
func (v *volume) busy(rel string) bool {
	for _, f := range v.forks {
		if f.rel == rel || strings.HasPrefix(f.rel, rel+"/") {
			return true
		}
	}
	return false
}
