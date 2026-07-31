package storage

import (
	"math/rand"
	"testing"
)

func TestTheNibbleTableIsAPermutationOfValidBytes(t *testing.T) {
	g := newGcr()

	seen := make(map[uint8]bool)
	for value, nibble := range g.nibble {
		if nibble&0x80 == 0 {
			t.Errorf("the nibble for %v is $%02x, which has the top bit clear", value, nibble)
		}
		if seen[nibble] {
			t.Errorf("$%02x is in the table twice", nibble)
		}
		seen[nibble] = true

		if got := g.decode[nibble]; got != uint8(value) {
			t.Errorf("$%02x decodes to %v, expected %v", nibble, got, value)
		}
	}

	/*
		No byte of the encoding may have more than two zero bits in a row,
		which together with the top bit being set is what keeps the drive
		able to find the byte boundary again
	*/
	for _, nibble := range g.nibble {
		for bit := 0; bit < 6; bit++ {
			if nibble&(7<<bit) == 0 {
				t.Errorf("$%02x has three zero bits in a row at %v", nibble, bit)
			}
		}
	}
}

/*
A field is announced by $d5 $aa, and neither is a nibble, so the pair can not
turn up inside encoded data and a scan for it can not be fooled. The third
byte tells the two fields apart and the closing $de $aa $ff is not scanned for
at all, which is just as well: $de is a perfectly good nibble.
*/
func TestNoFieldMarkCanAppearInsideTheData(t *testing.T) {
	g := newGcr()

	for _, mark := range []string{addressMark, dataMark} {
		for i := 0; i < 2; i++ {
			if g.decode[mark[i]] != gcrInvalid {
				t.Errorf("$%02x is both a mark byte and a nibble", mark[i])
			}
		}
	}

	if g.decode[fieldEnd[0]] == gcrInvalid {
		t.Error("$de is expected to be a nibble, the trailer is not scanned for")
	}
}

func TestAGroupSurvivesTheEncoding(t *testing.T) {
	g := newGcr()

	for _, values := range [][3]uint8{
		{0x00, 0x00, 0x00},
		{0xff, 0xff, 0xff},
		{0x12, 0x34, 0x56},
		{0xc0, 0x3f, 0x80},
	} {
		n0, n1, n2, n3 := g.encodeGroup(values[0], values[1], values[2])
		a, b, c, ok := g.decodeGroup(n0, n1, n2, n3)

		if !ok {
			t.Fatalf("%x did not decode", values)
		}
		if a != values[0] || b != values[1] || c != values[2] {
			t.Errorf("%x came back as %02x %02x %02x", values, a, b, c)
		}
	}
}

// buildSectorData fills a track's worth of sectors with something that is not
// the same twice, so that a sector landing in the wrong place is noticed
func buildSectorData(sectors int, seed int64) []uint8 {
	random := rand.New(rand.NewSource(seed))

	data := make([]uint8, sectors*sectorSize)
	for i := range data {
		data[i] = uint8(random.Intn(256))
	}
	return data
}

func TestATrackSurvivesTheEncoding(t *testing.T) {
	// One track of each of the five bands, so that every sector count is
	// covered
	for _, track := range []int{0, 16, 32, 48, 79} {
		sectors := sectorsInTrack(track)
		data := buildSectorData(sectors, int64(track))

		encoded, err := encodeTrack(track, 0, 2, data)
		if err != nil {
			t.Fatalf("the track %v did not encode: %v", track, err)
		}

		if len(encoded) != sectors*bytesPerSector {
			t.Errorf("the track %v came to %v bytes, expected %v",
				track, len(encoded), sectors*bytesPerSector)
		}

		decoded := decodeTrack(encoded)
		if len(decoded) != sectors {
			t.Fatalf("the track %v gave back %v sectors, expected %v",
				track, len(decoded), sectors)
		}

		for sector := 0; sector < sectors; sector++ {
			from := sector * sectorSize
			got, found := decoded[sector]
			if !found {
				t.Fatalf("the sector %v of the track %v was not found", sector, track)
			}
			for i, want := range data[from : from+sectorSize] {
				if got[i] != want {
					t.Fatalf("the sector %v of the track %v differs at %v, $%02x for $%02x",
						sector, track, i, got[i], want)
				}
			}
		}
	}
}

/*
A track is a loop and the machine starts reading wherever the head happens to
be, so a sector that straddles the end of the buffer has to be readable too.
Rotating the track is the same disk seen from a different starting point.
*/
func TestASectorIsFoundAcrossTheEndOfTheTrack(t *testing.T) {
	const track = 0
	sectors := sectorsInTrack(track)
	data := buildSectorData(sectors, 1)

	encoded, err := encodeTrack(track, 0, 2, data)
	if err != nil {
		t.Fatal(err)
	}

	// Start half a sector before the end, which puts a data field across
	// the seam
	cut := len(encoded) - bytesPerSector/2
	rotated := append(append([]uint8{}, encoded[cut:]...), encoded[:cut]...)

	decoded := decodeTrack(rotated)
	if len(decoded) != sectors {
		t.Fatalf("%v sectors were read off the rotated track, expected %v",
			len(decoded), sectors)
	}

	for sector := 0; sector < sectors; sector++ {
		from := sector * sectorSize
		for i, want := range data[from : from+sectorSize] {
			if decoded[sector][i] != want {
				t.Fatalf("the sector %v differs at %v", sector, i)
			}
		}
	}
}

func TestABadNibbleLosesOnlyItsOwnSector(t *testing.T) {
	const track = 0
	sectors := sectorsInTrack(track)
	data := buildSectorData(sectors, 2)

	encoded, err := encodeTrack(track, 0, 2, data)
	if err != nil {
		t.Fatal(err)
	}

	// Break a byte in the middle of the data field of the third sector
	encoded[2*bytesPerSector+syncBeforeAddress+addressFieldSize+syncBeforeData+300] = 0x00

	decoded := decodeTrack(encoded)
	if len(decoded) != sectors-1 {
		t.Fatalf("%v sectors survived, expected %v", len(decoded), sectors-1)
	}
}

func TestTheAddressFieldCarriesTheTrackAndSide(t *testing.T) {
	g := newGcr()

	// The track is split over two of the values and the side rides on the
	// second, so the outer tracks of the far side are the ones to check
	for _, c := range []struct {
		track int
		side  int
	}{{0, 0}, {63, 0}, {64, 1}, {79, 1}} {
		field := g.encodeAddressField(nil, uint8(c.track), uint8(c.side), 7, formatDoubleSided)

		at := func(i int) uint8 { return field[i] }
		sector, ok := g.decodeAddressField(at, len(addressMark))
		if !ok {
			t.Fatalf("the address field of the track %v side %v did not decode",
				c.track, c.side)
		}
		if sector != 7 {
			t.Errorf("the sector came back as %v, expected 7", sector)
		}

		low := g.decode[field[3]]
		high := g.decode[field[5]]
		if track := int(high&1)<<6 | int(low); track != c.track {
			t.Errorf("the track came back as %v, expected %v", track, c.track)
		}
		if side := int(high>>5) & 1; side != c.side {
			t.Errorf("the side came back as %v, expected %v", side, c.side)
		}
	}
}

func TestTheSectorsPerTrackAddUpToTheCapacity(t *testing.T) {
	total := 0
	for track := 0; track < TracksPerSide; track++ {
		total += sectorsInTrack(track)
	}

	if total != sectorsPerSide {
		t.Errorf("the tracks hold %v sectors a side, expected %v", total, sectorsPerSide)
	}
	if total*BlockSize != 400*1024 {
		t.Errorf("a side comes to %v bytes, expected 400Kb", total*BlockSize)
	}
}

func TestTheInterleaveVisitsEverySectorOnce(t *testing.T) {
	for track := 0; track < TracksPerSide; track++ {
		sectors := sectorsInTrack(track)
		order := interleavedOrder(sectors)

		seen := make(map[int]bool)
		for _, sector := range order {
			if sector < 0 || sector >= sectors {
				t.Fatalf("the track %v places the sector %v", track, sector)
			}
			if seen[sector] {
				t.Fatalf("the track %v places the sector %v twice", track, sector)
			}
			seen[sector] = true
		}
	}
}

func TestTheBlocksRunAcrossTheSidesOfATrack(t *testing.T) {
	// A double sided disk keeps both sides of a track together, and the
	// blocks of the whole disk have to come out consecutive and complete
	next := 0
	for track := 0; track < TracksPerSide; track++ {
		for side := 0; side < 2; side++ {
			for sector := 0; sector < sectorsInTrack(track); sector++ {
				if block := blockOf(track, side, sector, 2); block != next {
					t.Fatalf("the track %v side %v sector %v is the block %v, expected %v",
						track, side, sector, block, next)
				}
				next++
			}
		}
	}

	if next != 2*sectorsPerSide {
		t.Errorf("the disk came to %v blocks, expected %v", next, 2*sectorsPerSide)
	}
}
