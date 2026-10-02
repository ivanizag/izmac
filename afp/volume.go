package afp

import (
	"encoding/binary"
	"os"
	"time"
)

/*
The volume itself: opening it and what it says of itself. There is one, number
1, and it is open to everyone logged in, with no password.
*/

const (
	volumeID = 1

	// signatureFixed says that the directory IDs of the volume do not
	// change, which is what an HFS volume has
	signatureFixed = 2

	// neverBackedUp is the date of a backup that never happened
	neverBackedUp = 0x80000000

	// mostBytes is what the volume says it has at most, free or in total:
	// two gigabytes, which is the most the clients of AFP 2.0 can take
	// without taking it for a negative number
	mostBytes = 0x7fffffff
)

// The volume parameters, by bit
const (
	volAttributes = 1 << iota
	volSignature
	volCreated
	volModified
	volBackedUp
	volID
	volBytesFree
	volBytesTotal
	volName
)

const volAllParameters = volName<<1 - 1

// readVolume reads the volume ID of a call, which has to be the one volume
func (r *reader) volume() bool {
	return r.uint16() == volumeID
}

// openVol opens the volume by its name
func (v *volume) openVol(r *reader) ([]uint8, int32) {
	r.byte()
	bitmap := r.uint16()
	name := r.pascal()
	if r.failed {
		return nil, errParamErr
	}
	if !sameName(name, v.name) {
		return nil, errObjectNotFound
	}
	return v.volumeParms(bitmap)
}

func (v *volume) getVolParms(r *reader) ([]uint8, int32) {
	r.byte()
	if !r.volume() {
		return nil, errParamErr
	}
	bitmap := r.uint16()
	if r.failed {
		return nil, errParamErr
	}
	return v.volumeParms(bitmap)
}

/*
volumeParms is what the volume says of itself. The modification date is what
the client watches to know that something changed and its windows need to be
drawn again: the Finder asks for it every ten seconds or so, and reads the folders
of its windows again when it moves. See modified.
*/
func (v *volume) volumeParms(bitmap uint16) ([]uint8, int32) {
	if bitmap&^volAllParameters != 0 {
		return nil, errBitmapErr
	}

	reply := binary.BigEndian.AppendUint16(nil, bitmap)
	p := &parameters{}
	free, total := freeSpace(v.root)

	if bitmap&volAttributes != 0 {
		p.uint16(0)
	}
	if bitmap&volSignature != 0 {
		p.uint16(signatureFixed)
	}
	if bitmap&volCreated != 0 {
		p.uint32(afpTime(v.created))
	}
	if bitmap&volModified != 0 {
		p.uint32(afpTime(v.modified()))
	}
	if bitmap&volBackedUp != 0 {
		p.uint32(neverBackedUp)
	}
	if bitmap&volID != 0 {
		p.uint16(volumeID)
	}
	if bitmap&volBytesFree != 0 {
		p.uint32(uint32(min(free, mostBytes)))
	}
	if bitmap&volBytesTotal != 0 {
		p.uint32(uint32(min(total, mostBytes)))
	}
	if bitmap&volName != 0 {
		p.name(v.name)
	}
	return append(reply, p.bytes()...), errNoErr
}

/*
modified is when the volume last changed: the last time the machine changed
it, or anything it knows of changed on the host. That is every folder and file
that has a number, which is every one it has listed or named in a call;
something made, deleted or renamed in a folder changes the folder, and a file
written to changes the file. One the machine never looked at it has no window
open for, and no need to hear of.
*/
func (v *volume) modified() time.Time {
	latest := v.changed
	for rel := range v.byPath {
		if info, err := os.Stat(v.host(rel)); err == nil && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	return latest
}

// touch says the volume changed
func (v *volume) touch() {
	v.changed = time.Now()
}

/*
parameters is a block of parameters: the fixed part, in the order of the bits
of the bitmap, and after it the names, each where an offset in the fixed part
says, counted from the start of the block
*/
type parameters struct {
	fixed   []uint8
	names   [][]uint8
	offsets []int
}

func (p *parameters) uint16(n uint16) { p.fixed = binary.BigEndian.AppendUint16(p.fixed, n) }
func (p *parameters) uint32(n uint32) { p.fixed = binary.BigEndian.AppendUint32(p.fixed, n) }
func (p *parameters) raw(b []uint8)   { p.fixed = append(p.fixed, b...) }

// name leaves room for the offset of a name
func (p *parameters) name(name []uint8) {
	p.offsets = append(p.offsets, len(p.fixed))
	p.names = append(p.names, name)
	p.uint16(0)
}

func (p *parameters) bytes() []uint8 {
	out := append([]uint8{}, p.fixed...)
	for i, name := range p.names {
		binary.BigEndian.PutUint16(out[p.offsets[i]:], uint16(len(out)))
		out = appendPascal(out, name)
	}
	return out
}
