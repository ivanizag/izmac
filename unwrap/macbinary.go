package unwrap

import (
	"encoding/binary"
	"fmt"
	"time"
)

/*
MacBinary, the two forks of a Macintosh file and its Finder information put
one after the other in a single file, so that it can sit on a host that has
one fork per file. It is the .bin of the downloads, and what a self extracting
.sea usually travels in.

A 128 byte header, then the data fork and the resource fork, each padded to a
multiple of 128:

	  0  1  zero
	  1  1  the length of the name, 1 to 63
	  2 63  the name, in Mac OS Roman
	 65  4  the type
	 69  4  the creator
	 73  1  the high byte of the Finder flags
	 74  1  zero
	 82  1  zero
	 83  4  the length of the data fork
	 87  4  the length of the resource fork
	 91  4  created, on the clock of the machine
	 95  4  modified
	101  1  the low byte of the Finder flags, MacBinary II
	120  2  the length of a secondary header, MacBinary II
	122  1  the version that wrote it, 129 or 130, MacBinary II and III
	124  2  the CRC of the first 124 bytes, MacBinary II and III

Nothing says a file is MacBinary other than all of that holding together, so
all of it is checked. The first version has no CRC, and is only taken when
the bytes the later ones use are zero and the forks fill the file.
*/

const (
	macBinaryHeaderSize = 128
	macBinaryMaxName    = 63

	// macBinaryMaxFork is far more than a fork on a Macintosh Plus could be,
	// and is there to turn away a length that is not one
	macBinaryMaxFork = 1 << 30
)

func isMacBinary(head []uint8, size int64) bool {
	_, _, ok := parseMacBinary(head, size)
	return ok
}

/*
parseMacBinary checks the header and returns where the data fork starts and
how long it is. The resource fork starts after the data fork, padded.
*/
func parseMacBinary(head []uint8, size int64) (int64, int64, bool) {
	if len(head) < macBinaryHeaderSize {
		return 0, 0, false
	}
	h := head[:macBinaryHeaderSize]

	if h[0] != 0 || h[74] != 0 || h[82] != 0 {
		return 0, 0, false
	}
	if h[1] == 0 || h[1] > macBinaryMaxName {
		return 0, 0, false
	}

	dataLength := int64(binary.BigEndian.Uint32(h[83:87]))
	resourceLength := int64(binary.BigEndian.Uint32(h[87:91]))
	if dataLength > macBinaryMaxFork || resourceLength > macBinaryMaxFork {
		return 0, 0, false
	}

	secondary := int64(0)
	crc := binary.BigEndian.Uint16(h[124:126])
	if crc != 0 && crc == crc16(h[:124]) {
		secondary = padded(int64(binary.BigEndian.Uint16(h[120:122])))
	} else {
		// The first version, which left everything from 99 on zero
		for _, b := range h[99:] {
			if b != 0 {
				return 0, 0, false
			}
		}
	}

	start := macBinaryHeaderSize + secondary
	end := start + padded(dataLength) + padded(resourceLength)

	// The last fork is not always padded, and some files carry a little
	// more at the end than they should, but never another block of it
	if size < start+dataLength || size > end+macBinaryHeaderSize {
		return 0, 0, false
	}

	return start, dataLength, true
}

// openMacBinary takes the file out, both forks and its Finder information
func openMacBinary(data []uint8) (File, error) {
	start, length, ok := parseMacBinary(data, int64(len(data)))
	if !ok {
		return File{}, fmt.Errorf("the MacBinary header does not hold together")
	}

	file := File{
		Name:     macRoman(data[2 : 2+int(data[1])]),
		Data:     data[start : start+length],
		Flags:    uint16(data[73])<<8 | uint16(data[101]),
		Modified: fromMacTime(binary.BigEndian.Uint32(data[95:99])),
	}
	copy(file.Type[:], data[65:69])
	copy(file.Creator[:], data[69:73])

	// The resource fork can be cut short in a file that was padded with
	// less than it should have been, and is then taken as far as it goes
	resourceStart := start + padded(length)
	resourceLength := int64(binary.BigEndian.Uint32(data[87:91]))
	if resourceStart < int64(len(data)) {
		file.Resource = data[resourceStart:min(int64(len(data)), resourceStart+resourceLength)]
	}

	return file, nil
}

/*
fromMacTime turns a time on the clock of the machine, the seconds since 1904 in
local time, into one on the host. Zero is no time at all.
*/
func fromMacTime(seconds uint32) time.Time {
	if seconds == 0 {
		return time.Time{}
	}
	t := time.Unix(int64(seconds)-macEpoch, 0)
	_, zone := t.Zone()
	return t.Add(-time.Duration(zone) * time.Second)
}

// macEpoch is 1904 on the Unix clock
const macEpoch = 2082844800

// padded rounds a length up to the 128 bytes MacBinary pads to
func padded(length int64) int64 {
	return (length + macBinaryHeaderSize - 1) / macBinaryHeaderSize * macBinaryHeaderSize
}
