package storage

import "fmt"

/*
The group coded recording the Macintosh drives use, and the layout of a track
made out of it.

A diskette holds bits, not bytes, and the drive has no way to say where one
byte ends and the next begins. The encoding solves that by never writing a
byte with the top bit clear and never leaving more than two zero bits in a
row, so the shift register can be trusted to have a whole byte once the top
bit arrives. That leaves 64 usable values out of the 256, six bits carried in
eight, which is where the six and two of the name comes from: three bytes are
split into their low six bits, three values, plus a fourth holding the six
bits left over, and the four are written as four of the sixty four.

Everything here is verified against the Sony driver in the ROM,
plus/resources/res_drvr_sony.s of the mac_rom disassembly, which is the code
on the other side of the wire and so the authority on what has to come out.
The nibble table is DT_Sony_NiblTbl, copied value for value.
*/

/*
gcr carries the two tables the encoding needs. They would be the obvious pair
of package level tables and izmac keeps none, so they hang off something: a
diskette builds one when it is opened and uses it for every track.
*/
type gcr struct {
	// nibble turns a six bit value into the byte written to the disk
	nibble [64]uint8

	/*
		decode is the reverse, with gcrInvalid for the bytes that are not a
		nibble at all. The driver keeps the same table and tests the sign of
		what comes out of it, which is why the invalid entry has to be a
		value with the top bit set.
	*/
	decode [256]uint8
}

// gcrInvalid is what the decoding gives for a byte that is not a nibble
const gcrInvalid uint8 = 0xff

func newGcr() *gcr {
	g := &gcr{nibble: [64]uint8{
		0x96, 0x97, 0x9a, 0x9b, 0x9d, 0x9e, 0x9f, 0xa6,
		0xa7, 0xab, 0xac, 0xad, 0xae, 0xaf, 0xb2, 0xb3,
		0xb4, 0xb5, 0xb6, 0xb7, 0xb9, 0xba, 0xbb, 0xbc,
		0xbd, 0xbe, 0xbf, 0xcb, 0xcd, 0xce, 0xcf, 0xd3,
		0xd6, 0xd7, 0xd9, 0xda, 0xdb, 0xdc, 0xdd, 0xde,
		0xdf, 0xe5, 0xe6, 0xe7, 0xe9, 0xea, 0xeb, 0xec,
		0xed, 0xee, 0xef, 0xf2, 0xf3, 0xf4, 0xf5, 0xf6,
		0xf7, 0xf9, 0xfa, 0xfb, 0xfc, 0xfd, 0xfe, 0xff,
	}}

	for i := range g.decode {
		g.decode[i] = gcrInvalid
	}
	for value, nibble := range g.nibble {
		g.decode[nibble] = uint8(value)
	}

	return g
}

/*
The marks around the two fields of a sector. An address field says which
sector is about to come by and a data field carries it, and each is announced
by three bytes and closed by three more.

Neither $d5 nor $aa is a nibble, so the pair that opens a field can not turn
up inside encoded data and a scan for it can not be fooled. The closing three
are not scanned for at all, which is just as well: $de is a nibble.
*/
const (
	addressMark = "\xd5\xaa\x96"
	dataMark    = "\xd5\xaa\xad"
	fieldEnd    = "\xde\xaa\xff"
)

const (
	// TagSize is the tag bytes carried with every sector, ahead of the data.
	// The file system used them and a plain image has nowhere to keep them,
	// so they come out as zeros unless the image is a DiskCopy one.
	TagSize = 12

	// sectorSize is what a sector holds once the tags are counted in, and
	// what the encoding of a data field covers
	sectorSize = TagSize + BlockSize

	/*
		The self sync the drive needs to find the byte boundary again is a
		pattern of bytes with a zero bit slipped in, which only means
		something to a shift register reading bits. izmac hands whole bytes
		to the controller, having never taken them apart, so a run of $ff
		does the same job here: the driver waits for bytes with the top bit
		set and then looks for a mark, and $ff is a byte and is not a mark.
	*/
	syncByte uint8 = 0xff

	/*
		The shape of a sector on the track. A data field is the three mark
		bytes, the sector number, the 699 nibbles the 524 bytes come to, the
		four of the checksum and the three closing ones. An address field is
		three marks, five values and three closing bytes.

		The sync before each field is what is left of the byte budget below,
		and the budget is the point: a sector takes 778 bytes whatever zone
		it is in, so a track of twelve is longer than a track of eight in
		the same proportion as the disk turns slower under it, and the
		rotation comes out right without being worked out separately.
	*/
	bytesPerSector = 778

	addressFieldSize = 3 + 5 + 3
	dataFieldSize    = 3 + 1 + 699 + 4 + 3

	syncBeforeData    = 5
	syncBeforeAddress = bytesPerSector - addressFieldSize - syncBeforeData - dataFieldSize
)

/*
The format byte of the address field, from SonyEqu.a: the low nibble is the
interleave the disk was written with and the bit 5 says the disk is double
sided. Two to one is what the driver formats with and what it expects to find.
*/
const (
	formatSingleSided uint8 = 0x02
	formatDoubleSided uint8 = 0x22

	interleave = 2
)

/*
encodeGroup packs three bytes into the four nibbles that carry them: one
holding the top two bits of each, then the low six bits of each in turn.
*/
func (g *gcr) encodeGroup(a uint8, b uint8, c uint8) (uint8, uint8, uint8, uint8) {
	extra := (a>>6)<<4 | (b>>6)<<2 | (c >> 6)

	return g.nibble[extra&0x3f],
		g.nibble[a&0x3f],
		g.nibble[b&0x3f],
		g.nibble[c&0x3f]
}

// decodeGroup undoes encodeGroup, reporting whether all four bytes were
// nibbles at all
func (g *gcr) decodeGroup(n0 uint8, n1 uint8, n2 uint8, n3 uint8) (uint8, uint8, uint8, bool) {
	extra := g.decode[n0]
	a := g.decode[n1]
	b := g.decode[n2]
	c := g.decode[n3]

	if extra == gcrInvalid || a == gcrInvalid || b == gcrInvalid || c == gcrInvalid {
		return 0, 0, 0, false
	}

	return a | (extra>>4)&3<<6,
		b | (extra>>2)&3<<6,
		c | extra&3<<6,
		true
}

/*
The data field is not written as it stands. Three running bytes are carried
through the sector, each one added to as a byte goes by, and every byte is
exclusive ored with one of them before being encoded. The three are the
checksum the driver compares at the end, so the same pass both scrambles the
data and adds it up.

This is the part that has to match the ROM exactly, and the ROM is where it
came from: SONY_RDDATA_CONT_3 and the two loops after it read a group as

	ADD.B D7,D3     the carry out is the top bit of the third running byte
	ROL.B #1,D7     which is rotated before anything is scrambled with it
	EOR.B D7,D2     the first byte of the group
	ADDX.B D2,D5    added in with that carry
	EOR.B D5,D2     the second byte, with the first running byte
	ADDX.B D2,D6
	EOR.B D6,D1     the third, with the second
	ADDX.B D1,D7

so the carry into the first addition is the top bit the rotation pushed out,
and the carry out of each addition goes into the next. The one left at the end
of a group is dropped, since the next group starts from the rotation again.
*/
type gcrChecksum struct {
	a     uint8
	b     uint8
	c     uint8
	carry uint8
}

// rotate starts a group, turning the third running byte over and taking the
// bit that falls off as the carry into the first addition
func (s *gcrChecksum) rotate() {
	s.carry = s.c >> 7
	s.c = s.c<<1 | s.carry
}

// take adds a plain byte into one of the running bytes, carrying on
func (s *gcrChecksum) take(into *uint8, value uint8) {
	sum := uint16(*into) + uint16(value) + uint16(s.carry)
	*into = uint8(sum)
	s.carry = uint8(sum >> 8)
}

// scramble turns three plain bytes into the three the disk carries, taking
// them into the checksum on the way
func (s *gcrChecksum) scramble(p0 uint8, p1 uint8, p2 uint8) (uint8, uint8, uint8) {
	r0, r1 := s.scramblePair(p0, p1)

	r2 := p2 ^ s.b
	s.take(&s.c, p2)

	return r0, r1, r2
}

// scramblePair is the same for the two bytes of the last group of a data
// field, which is short one
func (s *gcrChecksum) scramblePair(p0 uint8, p1 uint8) (uint8, uint8) {
	s.rotate()

	r0 := p0 ^ s.c
	s.take(&s.a, p0)

	r1 := p1 ^ s.a
	s.take(&s.b, p1)

	return r0, r1
}

// unscramble is the same pass the other way round, which is what the driver
// does when it reads
func (s *gcrChecksum) unscramble(r0 uint8, r1 uint8, r2 uint8) (uint8, uint8, uint8) {
	p0, p1 := s.unscramblePair(r0, r1)

	p2 := r2 ^ s.b
	s.take(&s.c, p2)

	return p0, p1, p2
}

func (s *gcrChecksum) unscramblePair(r0 uint8, r1 uint8) (uint8, uint8) {
	s.rotate()

	p0 := r0 ^ s.c
	s.take(&s.a, p0)

	p1 := r1 ^ s.a
	s.take(&s.b, p1)

	return p0, p1
}

/*
encodeDataField writes the mark, the sector number, the 524 bytes of tags and
data, the checksum and the closing bytes.

The 524 do not divide by three, so the last group is two bytes and three
nibbles rather than four, which is where the 699 comes from. The driver reads
it the same way: it counts down to zero and stops after the second byte of the
group without asking for the third.
*/
func (g *gcr) encodeDataField(out []uint8, sector uint8, sectorData []uint8) []uint8 {
	out = append(out, dataMark...)
	out = append(out, g.nibble[sector&0x3f])

	var checksum gcrChecksum

	for i := 0; i < len(sectorData); i += 3 {
		if i+3 > len(sectorData) {
			// The last two bytes, which take three nibbles and not four
			r0, r1 := checksum.scramblePair(sectorData[i], sectorData[i+1])
			extra := (r0>>6)<<4 | (r1>>6)<<2
			out = append(out,
				g.nibble[extra&0x3f], g.nibble[r0&0x3f], g.nibble[r1&0x3f])
			break
		}

		r0, r1, r2 := checksum.scramble(sectorData[i], sectorData[i+1], sectorData[i+2])
		n0, n1, n2, n3 := g.encodeGroup(r0, r1, r2)
		out = append(out, n0, n1, n2, n3)
	}

	// The three running bytes close the field, in a group of their own
	n0, n1, n2, n3 := g.encodeGroup(checksum.a, checksum.b, checksum.c)
	out = append(out, n0, n1, n2, n3)

	return append(out, fieldEnd...)
}

// encodeAddressField says which sector is coming. The track is split over two
// of the five values, with the side riding on the bit 5 of the second, and
// the last is the exclusive or of the four before it.
func (g *gcr) encodeAddressField(out []uint8, track uint8, side uint8,
	sector uint8, format uint8) []uint8 {

	low := track & 0x3f
	high := side<<5 | track>>6
	checksum := low ^ sector ^ high ^ format

	out = append(out, addressMark...)
	out = append(out,
		g.nibble[low],
		g.nibble[sector&0x3f],
		g.nibble[high&0x3f],
		g.nibble[format&0x3f],
		g.nibble[checksum&0x3f])

	return append(out, fieldEnd...)
}

// appendSync puts a run of self sync down
func appendSync(out []uint8, count int) []uint8 {
	for i := 0; i < count; i++ {
		out = append(out, syncByte)
	}
	return out
}

/*
SectorsInTrack is how many sectors a track holds. The disk turns slower the
further out the head is, in five bands of sixteen tracks, so that the bits
stay the same length along the track and the outer ones hold more of them.
Twelve down to eight over the eighty tracks is 800 sectors a side.
*/
func SectorsInTrack(track int) int {
	return 12 - track/16
}

const (
	// TracksPerSide is how far the head can go, which is what stops the
	// stepper of the drive
	TracksPerSide = 80

	// sectorsPerSide is the sum of SectorsInTrack over every track
	sectorsPerSide = 16 * (12 + 11 + 10 + 9 + 8)
)

/*
BlockOf gives the position in the image of a sector. The blocks run along a
track, then over to the other side of the same track, then outwards, which is
the order a single sided image keeps too once the side is always zero.
*/
func BlockOf(track int, side int, sector int, sides int) int {
	block := 0
	for t := 0; t < track; t++ {
		block += SectorsInTrack(t) * sides
	}
	return block + side*SectorsInTrack(track) + sector
}

/*
interleavedOrder is the order the sectors are laid out around the track. The
driver formats two to one, so that a sector and the next one along are half a
turn apart and the machine has time to deal with the first before the second
arrives.

Nothing reads this back: a sector is found by its address field wherever it
is. It is here so that a track izmac writes looks like one the machine wrote.
*/
func interleavedOrder(sectors int) []int {
	order := make([]int, sectors)
	for i := range order {
		order[i] = -1
	}

	position := 0
	for sector := 0; sector < sectors; sector++ {
		for order[position] != -1 {
			position = (position + 1) % sectors
		}
		order[position] = sector
		position = (position + interleave) % sectors
	}

	return order
}

/*
encodeTrack builds the bytes that go round one track. The sectors come in the
order they sit on the disk and each is a run of sync, an address field, a
short run of sync and a data field.

The caller passes the tags and data of every sector of the track, one after
the other, sectorSize bytes each.
*/
func (g *gcr) encodeTrack(track int, side int, sides int, sectorData []uint8) ([]uint8, error) {
	sectors := SectorsInTrack(track)
	if len(sectorData) != sectors*sectorSize {
		return nil, fmt.Errorf("the track %v holds %v sectors, %v bytes were given",
			track, sectors, len(sectorData))
	}

	format := formatSingleSided
	if sides == 2 {
		format = formatDoubleSided
	}

	out := make([]uint8, 0, sectors*bytesPerSector)
	for _, sector := range interleavedOrder(sectors) {
		out = appendSync(out, syncBeforeAddress)
		out = g.encodeAddressField(out, uint8(track), uint8(side), uint8(sector), format)
		out = appendSync(out, syncBeforeData)

		from := sector * sectorSize
		out = g.encodeDataField(out, uint8(sector), sectorData[from:from+sectorSize])
	}

	return out, nil
}

/*
decodeTrack reads the sectors back out of the bytes of a track, which is what
turns what the machine wrote into something to store in the image. It returns
the tags and data of every sector it could make sense of, keyed by the sector
number in the address field.

A track is a loop, so a sector can start near the end of the buffer and finish
at the beginning. Reading is done through an index that wraps for that reason,
over a window of one and a bit turns so that such a sector is seen whole.
*/
func (g *gcr) decodeTrack(track []uint8) map[int][]uint8 {
	sectors := make(map[int][]uint8)
	if len(track) < dataFieldSize {
		return sectors
	}

	at := func(i int) uint8 { return track[i%len(track)] }

	matches := func(i int, mark string) bool {
		for j := 0; j < len(mark); j++ {
			if at(i+j) != mark[j] {
				return false
			}
		}
		return true
	}

	/*
		The address field of a sector and its data field are read together:
		the number to store the data under comes from the address field, and
		a data field on its own says only which sector the writer thought it
		was, which is the same thing but is not checked against the track.
	*/
	limit := len(track) + bytesPerSector
	for i := 0; i < limit; i++ {
		if !matches(i, addressMark) {
			continue
		}

		sector, ok := g.decodeAddressField(at, i+len(addressMark))
		if !ok {
			continue
		}

		// The data field follows within a sector's worth of bytes
		for j := i; j < i+bytesPerSector; j++ {
			if !matches(j, dataMark) {
				continue
			}
			if data, ok := g.decodeDataField(at, j+len(dataMark)); ok {
				sectors[sector] = data
			}
			break
		}
	}

	return sectors
}

// decodeAddressField returns the sector number, once the checksum agrees
func (g *gcr) decodeAddressField(at func(int) uint8, i int) (int, bool) {
	var values [5]uint8
	for j := range values {
		values[j] = g.decode[at(i+j)]
		if values[j] == gcrInvalid {
			return 0, false
		}
	}

	if values[0]^values[1]^values[2]^values[3] != values[4] {
		return 0, false
	}

	return int(values[1]), true
}

// decodeDataField returns the tags and data of a sector, once the checksum
// agrees. The first byte is the sector number the writer put there, which is
// not used: the address field is what says where the sector belongs.
func (g *gcr) decodeDataField(at func(int) uint8, i int) ([]uint8, bool) {
	if g.decode[at(i)] == gcrInvalid {
		return nil, false
	}
	i++

	data := make([]uint8, 0, sectorSize)
	var checksum gcrChecksum

	for len(data) < sectorSize {
		if sectorSize-len(data) == 2 {
			// The last group carries two bytes in three nibbles
			extra := g.decode[at(i)]
			r0 := g.decode[at(i+1)]
			r1 := g.decode[at(i+2)]
			if extra == gcrInvalid || r0 == gcrInvalid || r1 == gcrInvalid {
				return nil, false
			}
			p0, p1 := checksum.unscramblePair(
				r0|(extra>>4)&3<<6, r1|(extra>>2)&3<<6)
			data = append(data, p0, p1)
			i += 3
			break
		}

		r0, r1, r2, ok := g.decodeGroup(at(i), at(i+1), at(i+2), at(i+3))
		if !ok {
			return nil, false
		}
		p0, p1, p2 := checksum.unscramble(r0, r1, r2)
		data = append(data, p0, p1, p2)
		i += 4
	}

	a, b, c, ok := g.decodeGroup(at(i), at(i+1), at(i+2), at(i+3))
	if !ok || a != checksum.a || b != checksum.b || c != checksum.c {
		return nil, false
	}

	return data, true
}

/*
DecodeTrack reads the sectors out of the bytes that go round one track, the
inverse of what a drive hands to the machine. A diskette uses it to store what
was written to it, and it is offered on its own because the bytes are what a
drive deals in and a caller with a track of them has nothing else to do
with it.

The sectors come back keyed by the number in their address field, tags first
and then the data. Whatever does not decode is simply not there.
*/
func DecodeTrack(nibbles []uint8) map[int][]uint8 {
	return newGcr().decodeTrack(nibbles)
}
