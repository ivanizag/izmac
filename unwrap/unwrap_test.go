package unwrap

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

/*
The encoders the tests wrap things with. They are written from the formats and
not from the decoders, and real files were checked against unar while this was
written: the BinHex decoding of catfinder-270-x.sit.hqx from info-mac, a StuffIt
5 archive, comes out byte for byte the data fork unar gives.
*/

// encodeBinHex wraps a data fork in BinHex, with some mail in front of it and
// the lines broken where BinHex breaks them
func encodeBinHex(name string, fork []uint8) []uint8 {
	var plain []uint8
	plain = append(plain, uint8(len(name)))
	plain = append(plain, name...)
	plain = append(plain, 0)             // version
	plain = append(plain, "TEXTttxt"...) // type and creator
	plain = append(plain, 0, 0)          // flags
	plain = binary.BigEndian.AppendUint32(plain, uint32(len(fork)))
	plain = binary.BigEndian.AppendUint32(plain, 0) // no resource fork
	plain = binary.BigEndian.AppendUint16(plain, crc16(plain))
	plain = append(plain, fork...)
	plain = binary.BigEndian.AppendUint16(plain, crc16(fork))
	plain = binary.BigEndian.AppendUint16(plain, 0) // the empty resource fork

	// Run length encoding, for runs of more than three
	var packed []uint8
	for i := 0; i < len(plain); {
		b := plain[i]
		run := 1
		for i+run < len(plain) && plain[i+run] == b && run < 255 {
			run++
		}

		if b == binhexRepeat {
			packed = append(packed, binhexRepeat, 0)
		} else {
			packed = append(packed, b)
		}
		if run > 3 {
			packed = append(packed, binhexRepeat, uint8(run))
			i += run
		} else {
			i++
		}
	}

	var text strings.Builder
	text.WriteString("From: someone\r\nSubject: a file\r\n\r\n")
	text.WriteString("(This file must be converted with BinHex 4.0)\r\n:")
	column := 1
	for i := 0; i < len(packed); i += 3 {
		var group [3]uint8
		copy(group[:], packed[i:])
		bits := uint32(group[0])<<16 | uint32(group[1])<<8 | uint32(group[2])
		count := min(4, (len(packed)-i)*8/6+1)
		for j := range count {
			text.WriteByte(binhexAlphabet[bits>>(18-6*j)&0x3f])
			column++
			if column == 64 {
				text.WriteString("\r\n")
				column = 0
			}
		}
	}
	text.WriteString(":\r\n")
	return []uint8(text.String())
}

// encodeMacBinary wraps a data fork in MacBinary II, or the first version
// with no CRC
func encodeMacBinary(name string, fork []uint8, second bool) []uint8 {
	header := make([]uint8, macBinaryHeaderSize)
	header[1] = uint8(len(name))
	copy(header[2:], name)
	copy(header[65:], "TEXTttxt")
	binary.BigEndian.PutUint32(header[83:], uint32(len(fork)))
	if second {
		header[122], header[123] = 129, 129
		binary.BigEndian.PutUint16(header[124:], crc16(header[:124]))
	}

	out := append(header, fork...)
	return append(out, make([]uint8, padded(int64(len(fork)))-int64(len(fork)))...)
}

func encodeZip(t *testing.T, files map[string][]uint8, order []string) []uint8 {
	t.Helper()
	var buffer bytes.Buffer
	w := zip.NewWriter(&buffer)
	for _, name := range order {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write(files[name])
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func encodeGzip(name string, data []uint8) []uint8 {
	var buffer bytes.Buffer
	w := gzip.NewWriter(&buffer)
	w.Name = name
	w.Write(data)
	w.Close()
	return buffer.Bytes()
}

// someImage is not a disk image, but stands for one: bytes that are in no
// wrapper, with runs and $90 in them to give BinHex something to do
func someImage(size int) []uint8 {
	data := make([]uint8, size)
	for i := range data {
		switch {
		case i%300 < 100:
			data[i] = 0
		case i%7 == 0:
			data[i] = binhexRepeat
		default:
			data[i] = uint8(i * 31)
		}
	}
	data[0] = 'L'
	return data
}

func unwrapOne(t *testing.T, name string, data []uint8) File {
	t.Helper()
	files, err := NewUnwrapper().Unwrap(name, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("%v unwrapped to %v files, wanted one", name, len(files))
	}
	return files[0]
}

func TestTheCRCIsTheOneXMODEMUses(t *testing.T) {
	// The check value of CRC-16/XMODEM
	if crc := crc16([]uint8("123456789")); crc != 0x31c3 {
		t.Errorf("the CRC of 123456789 is $%04x, wanted $31c3", crc)
	}
}

func TestBinHexIsUndone(t *testing.T) {
	image := someImage(20000)
	file := unwrapOne(t, "game.hqx", encodeBinHex("Game.image", image))

	if file.Name != "Game.image" {
		t.Errorf("the name is %q, wanted the one in the header", file.Name)
	}
	if !bytes.Equal(file.Data, image) {
		t.Errorf("the data fork did not come back as it went in")
	}
}

func TestBinHexNamesAreMacRoman(t *testing.T) {
	file := unwrapOne(t, "game.hqx", encodeBinHex("Game \xc4", someImage(100)))

	if file.Name != "Game ƒ" {
		t.Errorf("the name is %q, wanted the florin the machine shows", file.Name)
	}
}

func TestABinHexFileThatFailsItsCRCIsRefused(t *testing.T) {
	text := encodeBinHex("Game.image", someImage(2000))

	// A character in the middle of the data fork, swapped for another one
	// of the alphabet
	at := bytes.IndexByte(text, ':') + 500
	if text[at] == '!' {
		text[at] = '"'
	} else {
		text[at] = '!'
	}

	if _, err := NewUnwrapper().Unwrap("game.hqx", text); err == nil {
		t.Errorf("a damaged BinHex file was unwrapped")
	}
}

func TestTheBinHexMarkerInsideABinaryFileIsNotBinHex(t *testing.T) {
	image := someImage(4000)
	copy(image[1000:], "(This file must be converted with BinHex 4.0)\r\n:!!!:")

	u := NewUnwrapper()
	if format := u.Identify("utility.dsk", image, int64(len(image))); format != "" {
		t.Errorf("a disk image with the marker in it was taken for %v", format)
	}
}

func TestMacBinaryIsUndone(t *testing.T) {
	image := someImage(5000)

	for _, second := range []bool{true, false} {
		file := unwrapOne(t, "game.bin", encodeMacBinary("Game.image", image, second))

		if file.Name != "Game.image" || !bytes.Equal(file.Data, image) {
			t.Errorf("MacBinary (second version %v) gave %q and %v bytes",
				second, file.Name, len(file.Data))
		}
	}
}

func TestAnImageIsNotTakenForMacBinary(t *testing.T) {
	u := NewUnwrapper()

	// A blank disk, and one with boot blocks, are the two that start with
	// the zero MacBinary does
	blank := make([]uint8, 800*1024)
	booting := someImage(800 * 1024)
	booting[0], booting[1] = 'L', 'K'

	for _, image := range [][]uint8{blank, booting} {
		if format := u.Identify("disk.dsk", image[:HeadSize], int64(len(image))); format != "" {
			t.Errorf("a disk image was taken for %v", format)
		}
	}
}

func TestEveryFileInAZipComesOutButTheMacintoshOnes(t *testing.T) {
	one, two := someImage(3000), someImage(4000)
	archive := encodeZip(t, map[string][]uint8{
		"Disks/One.dsk":            one,
		"__MACOSX/Disks/._One.dsk": {1, 2, 3},
		"Disks/._Two.dsk":          {4, 5, 6},
		"Disks/Two.dsk":            two,
	}, []string{"Disks/One.dsk", "__MACOSX/Disks/._One.dsk", "Disks/._Two.dsk", "Disks/Two.dsk"})

	files, err := NewUnwrapper().Unwrap("disks.zip", archive)
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 2 || files[0].Name != "One.dsk" || files[1].Name != "Two.dsk" {
		t.Fatalf("the zip gave %v files, wanted One.dsk and Two.dsk", len(files))
	}
	if !bytes.Equal(files[0].Data, one) || !bytes.Equal(files[1].Data, two) {
		t.Errorf("the files did not come out as they went in")
	}
}

func TestTheWrappersArePeeledOneAfterTheOther(t *testing.T) {
	image := someImage(6000)

	// A disk image, in MacBinary, in BinHex, in a zip, in a gzip
	wrapped := encodeMacBinary("Game.image", image, true)
	wrapped = encodeBinHex("Game.image.bin", wrapped)
	wrapped = encodeZip(t, map[string][]uint8{"Game.hqx": wrapped}, []string{"Game.hqx"})
	wrapped = encodeGzip("Game.zip", wrapped)

	file := unwrapOne(t, "Game.zip.gz", wrapped)
	if file.Name != "Game.image" || !bytes.Equal(file.Data, image) {
		t.Errorf("four wrappers gave %q and %v bytes", file.Name, len(file.Data))
	}
}

func TestTheWrappersAreFollowedOnlySoDeep(t *testing.T) {
	wrapped := someImage(100)
	for range maxDepth + 1 {
		wrapped = encodeGzip("", wrapped)
	}

	if _, err := NewUnwrapper().Unwrap("deep.gz", wrapped); err == nil {
		t.Errorf("a file %v wrappers deep was unwrapped", maxDepth+1)
	}
}

func TestAFileInNoWrapperComesBackAsItIs(t *testing.T) {
	image := someImage(1000)
	file := unwrapOne(t, "disk.dsk", image)

	if file.Name != "disk.dsk" || !bytes.Equal(file.Data, image) {
		t.Errorf("a plain file came back as %q and %v bytes", file.Name, len(file.Data))
	}
}

func TestTheArchiversLeftToUnarAreRecognised(t *testing.T) {
	classic := make([]uint8, 100)
	copy(classic, "SIT!")
	copy(classic[10:], "rLau")

	cases := []struct {
		name   string
		head   []uint8
		format string
	}{
		{"game.sit", classic, "StuffIt"},
		{"game.sit", []uint8("StuffIt (c)1997-2002 Aladdin Systems"), "StuffIt 5"},
		{"game.7z", []uint8("7z\xbc\xaf\x27\x1c\x00\x04"), "7-Zip"},
		{"game.rar", []uint8("Rar!\x1a\x07\x00"), "RAR"},
		{"game.cpt", []uint8{1, 1, 0, 0}, "Compact Pro"},
		{"game.sit", []uint8("SIT!but not the rest"), ""},
	}

	u := NewUnwrapper()
	for _, c := range cases {
		if format := u.Identify(c.name, c.head, 1000); format != c.format {
			t.Errorf("%q was taken for %q, wanted %q", c.head[:4], format, c.format)
		}
	}
}

func TestAnArchiveForUnarSaysSoWhenItIsMissing(t *testing.T) {
	archive := make([]uint8, 100)
	copy(archive, "StuffIt (c)1997-")

	u := NewUnwrapper()
	u.lookPath = func(string) (string, error) { return "", errors.New("not found") }

	_, err := u.Unwrap("game.sit", archive)
	if err == nil || !strings.Contains(err.Error(), "unar is not installed") {
		t.Errorf("a StuffIt archive with no unar gave %v", err)
	}
}

func TestUnarUnpacks(t *testing.T) {
	if _, err := exec.LookPath(unarCommand); err != nil {
		t.Skip("unar is not installed")
	}

	// unar takes zips as well, and a zip is the archive the tests can make
	image := someImage(3000)
	archive := encodeZip(t, map[string][]uint8{
		"Disks/One.dsk":   image,
		"Disks/._One.dsk": {1, 2, 3},
	}, []string{"Disks/One.dsk", "Disks/._One.dsk"})

	files, err := NewUnwrapper().openWithUnar("zip", File{Name: "disks.zip", Data: archive})
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 1 || files[0].Name != "One.dsk" || !bytes.Equal(files[0].Data, image) {
		t.Errorf("unar gave %v files, wanted One.dsk as it went in", len(files))
	}
}

func TestTheStemLosesTheExtensionsOfWrappersAndImages(t *testing.T) {
	cases := map[string]string{
		"Game.dsk.sit.hqx":           "Game",
		"/downloads/System 6.0.8.7z": "System 6.0.8",
		"Utilities 1.img":            "Utilities 1",
		"read.me":                    "read.me",
		".hqx":                       ".hqx",
	}

	for name, stem := range cases {
		if got := Stem(name); got != stem {
			t.Errorf("the stem of %q is %q, wanted %q", name, got, stem)
		}
	}
}

func TestSafeNamesKeepOffTheHostsSeparators(t *testing.T) {
	if got := SafeName("Disk 1/2: Tools\x01"); got != "Disk 1_2_ Tools_" {
		t.Errorf("the safe name is %q", got)
	}
	if got := SafeName(" .. "); got != "" {
		t.Errorf("a name of dots and spaces gave %q, wanted nothing", got)
	}
}
