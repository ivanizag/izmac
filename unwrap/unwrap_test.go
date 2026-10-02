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
	return encodeBinHexForks(name, fork, nil)
}

// encodeBinHexForks does the same with a resource fork as well
func encodeBinHexForks(name string, fork []uint8, resource []uint8) []uint8 {
	var plain []uint8
	plain = append(plain, uint8(len(name)))
	plain = append(plain, name...)
	plain = append(plain, 0)             // version
	plain = append(plain, "TEXTttxt"...) // type and creator
	plain = append(plain, 0x21, 0x00)    // flags
	plain = binary.BigEndian.AppendUint32(plain, uint32(len(fork)))
	plain = binary.BigEndian.AppendUint32(plain, uint32(len(resource)))
	plain = binary.BigEndian.AppendUint16(plain, crc16(plain))
	plain = append(plain, fork...)
	plain = binary.BigEndian.AppendUint16(plain, crc16(fork))
	plain = append(plain, resource...)
	plain = binary.BigEndian.AppendUint16(plain, crc16(resource))

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
	return encodeMacBinaryForks(name, fork, nil, second)
}

// encodeMacBinaryForks does the same with a resource fork as well
func encodeMacBinaryForks(name string, fork []uint8, resource []uint8, second bool) []uint8 {
	header := make([]uint8, macBinaryHeaderSize)
	header[1] = uint8(len(name))
	copy(header[2:], name)
	copy(header[65:], "APPLGAME")
	header[73] = 0x20 // the high byte of the flags, the bundle bit
	binary.BigEndian.PutUint32(header[83:], uint32(len(fork)))
	binary.BigEndian.PutUint32(header[87:], uint32(len(resource)))
	binary.BigEndian.PutUint32(header[95:], 0xa0000000) // in 1989
	if second {
		header[122], header[123] = 129, 129
		binary.BigEndian.PutUint16(header[124:], crc16(header[:124]))
	}

	out := append(header, fork...)
	out = append(out, make([]uint8, padded(int64(len(fork)))-int64(len(fork)))...)
	out = append(out, resource...)
	return append(out, make([]uint8, padded(int64(len(resource)))-int64(len(resource)))...)
}

/*
encodeAppleDouble makes the ._ file macOS puts next to a file on a volume that
cannot keep its resource fork, with the Finder information macOS writes, an
extended attribute header and all after the 32 bytes that matter
*/
func encodeAppleDouble(resource []uint8, typeCreator string) []uint8 {
	finder := make([]uint8, 32+50)
	copy(finder, typeCreator)
	copy(finder[32:], "ATTR")

	header := make([]uint8, 26+2*12)
	binary.BigEndian.PutUint32(header, appleDoubleMagic)
	binary.BigEndian.PutUint32(header[4:], 0x00020000)
	binary.BigEndian.PutUint16(header[24:], 2)

	at := uint32(len(header))
	binary.BigEndian.PutUint32(header[26:], entryFinder)
	binary.BigEndian.PutUint32(header[30:], at)
	binary.BigEndian.PutUint32(header[34:], uint32(len(finder)))
	binary.BigEndian.PutUint32(header[38:], entryResource)
	binary.BigEndian.PutUint32(header[42:], at+uint32(len(finder)))
	binary.BigEndian.PutUint32(header[46:], uint32(len(resource)))

	return append(append(header, finder...), resource...)
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

func TestBinHexKeepsTheResourceForkAndTheFinderInformation(t *testing.T) {
	resource := someImage(3000)
	file := unwrapOne(t, "app.hqx", encodeBinHexForks("App", []uint8("data"), resource))

	if !bytes.Equal(file.Resource, resource) {
		t.Errorf("the resource fork came back as %v bytes", len(file.Resource))
	}
	if string(file.Type[:]) != "TEXT" || string(file.Creator[:]) != "ttxt" || file.Flags != 0x2100 {
		t.Errorf("the Finder information came back as %q %q $%04x",
			file.Type, file.Creator, file.Flags)
	}
	if !file.IsMacFile() {
		t.Errorf("a file with a resource fork is not taken for a Macintosh one")
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

func TestMacBinaryKeepsTheResourceForkAndTheFinderInformation(t *testing.T) {
	resource := someImage(1000)
	file := unwrapOne(t, "game.bin", encodeMacBinaryForks("Game", nil, resource, true))

	if !bytes.Equal(file.Resource, resource) || len(file.Data) != 0 {
		t.Errorf("the forks came back as %v and %v bytes", len(file.Data), len(file.Resource))
	}
	if string(file.Type[:]) != "APPL" || string(file.Creator[:]) != "GAME" || file.Flags != 0x2000 {
		t.Errorf("the Finder information came back as %q %q $%04x",
			file.Type, file.Creator, file.Flags)
	}
	if file.Modified.Year() != 1989 {
		t.Errorf("the file was modified on %v, wanted in 1989", file.Modified)
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

func TestTheHalvesOfMacintoshFilesInAZipGoBackTogether(t *testing.T) {
	one, two := someImage(3000), someImage(4000)
	resource := someImage(500)
	archive := encodeZip(t, map[string][]uint8{
		"Disks/One.dsk":            one,
		"__MACOSX/Disks/._One.dsk": encodeAppleDouble(resource, "dImgdCpy"),
		"Disks/._Two.dsk":          encodeAppleDouble(nil, "TEXTttxt"),
		"Disks/Two.dsk":            two,
		"Disks/._App":              encodeAppleDouble(resource, "APPLGAME"),
	}, []string{"Disks/One.dsk", "__MACOSX/Disks/._One.dsk", "Disks/._Two.dsk",
		"Disks/Two.dsk", "Disks/._App"})

	files, err := NewUnwrapper().Unwrap("disks.zip", archive)
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 3 {
		t.Fatalf("the zip gave %v files, wanted One.dsk, Two.dsk and App", len(files))
	}
	gotOne, gotTwo, app := files[0], files[1], files[2]

	if gotOne.Name != "One.dsk" || !bytes.Equal(gotOne.Data, one) ||
		!bytes.Equal(gotOne.Resource, resource) || string(gotOne.Type[:]) != "dImg" {
		t.Errorf("One.dsk did not get its resource fork and type back from __MACOSX")
	}
	if gotTwo.Name != "Two.dsk" || !bytes.Equal(gotTwo.Data, two) || string(gotTwo.Creator[:]) != "ttxt" {
		t.Errorf("Two.dsk did not get its creator back from the ._ file beside it")
	}
	if len(gotOne.Folders) != 1 || gotOne.Folders[0] != "Disks" {
		t.Errorf("One.dsk is in the folders %v, wanted Disks", gotOne.Folders)
	}

	// An application with no data fork leaves nothing but the ._ file
	if app.Name != "App" || len(app.Data) != 0 || !bytes.Equal(app.Resource, resource) ||
		string(app.Type[:]) != "APPL" {
		t.Errorf("the application made of its ._ file alone is %q, %v bytes of resource fork",
			app.Name, len(app.Resource))
	}
}

func TestWhatIsInAnArchiveInAFolderStaysInTheFolder(t *testing.T) {
	inner := encodeZip(t, map[string][]uint8{"Levels/One": {1}}, []string{"Levels/One"})
	outer := encodeZip(t, map[string][]uint8{"Game/levels.zip": inner}, []string{"Game/levels.zip"})

	file := unwrapOne(t, "game.zip", outer)
	if strings.Join(file.Folders, "/") != "Game/Levels" || file.Name != "One" {
		t.Errorf("the file came out as %v in %v, wanted One in Game/Levels",
			file.Name, file.Folders)
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

	// unar takes zips as well, and a zip is the archive the tests can make.
	// The resource fork comes back out of unar in a ._ file of its own.
	image := someImage(3000)
	resource := someImage(700)
	archive := encodeZip(t, map[string][]uint8{
		"Disks/One.dsk":   image,
		"Disks/._One.dsk": encodeAppleDouble(resource, "dImgdCpy"),
	}, []string{"Disks/One.dsk", "Disks/._One.dsk"})

	files, err := NewUnwrapper().openWithUnar("zip", File{Name: "disks.zip", Data: archive})
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 1 || files[0].Name != "One.dsk" || !bytes.Equal(files[0].Data, image) {
		t.Fatalf("unar gave %v files, wanted One.dsk as it went in", len(files))
	}
	if !bytes.Equal(files[0].Resource, resource) || string(files[0].Type[:]) != "dImg" {
		t.Errorf("One.dsk lost its resource fork or its type on the way through unar")
	}
}

func TestClutterIsToldFromFiles(t *testing.T) {
	clutter := []File{
		{Name: ".DS_Store"},
		{Name: "._Game"},
		{Name: "Desktop", Type: [4]uint8{'F', 'N', 'D', 'R'}},
		{Name: "Desktop DB"},
	}
	for _, f := range clutter {
		if !f.IsClutter() {
			t.Errorf("%v was not taken for clutter", f.Name)
		}
	}

	if f := (File{Name: "Desktop", Type: [4]uint8{'T', 'E', 'X', 'T'}}); f.IsClutter() {
		t.Errorf("a document called Desktop was taken for clutter")
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
