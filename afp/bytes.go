package afp

import (
	"encoding/binary"
	"time"
)

/*
The shapes AFP blocks are made of: big endian numbers, Pascal strings, and
fields aligned to an even byte.
*/

// reader takes the fields of a request in turn; failed says one ran off the
// end, and every read after that gives zeros
type reader struct {
	data   []uint8
	at     int
	failed bool
}

func (r *reader) take(n int) []uint8 {
	if r.failed || r.at+n > len(r.data) {
		r.failed = true
		return make([]uint8, n)
	}
	b := r.data[r.at : r.at+n]
	r.at += n
	return b
}

func (r *reader) byte() uint8    { return r.take(1)[0] }
func (r *reader) uint16() uint16 { return binary.BigEndian.Uint16(r.take(2)) }
func (r *reader) uint32() uint32 { return binary.BigEndian.Uint32(r.take(4)) }

// pad skips the byte that keeps the next field on an even offset
func (r *reader) pad() {
	if r.at%2 != 0 {
		r.take(1)
	}
}

func (r *reader) pascal() []uint8 {
	n := int(r.byte())
	return r.take(n)
}

func appendPascal(b []uint8, s []uint8) []uint8 {
	if len(s) > 255 {
		s = s[:255]
	}
	b = append(b, uint8(len(s)))
	return append(b, s...)
}

func alignEven(b []uint8) []uint8 {
	if len(b)%2 != 0 {
		b = append(b, 0)
	}
	return b
}

// afpEpoch is the start of AFP's clock, midnight of the first of January
// 2000, universal time
const afpEpoch = 946684800

// afpTime is a time on AFP's clock, the seconds since 2000, which can be
// negative for anything older
func afpTime(t time.Time) uint32 {
	return uint32(int32(t.Unix() - afpEpoch))
}

// fromAFPTime is the time an AFP date stands for
func fromAFPTime(seconds uint32) time.Time {
	return time.Unix(int64(int32(seconds))+afpEpoch, 0)
}
