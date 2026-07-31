package storage

import (
	"encoding/binary"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// writeFloppyImage puts a plain image of the given size on disk, filled with
// something different in every sector
func writeFloppyImage(t *testing.T, name string, size int) (string, []uint8) {
	t.Helper()

	data := make([]uint8, size)
	random := rand.New(rand.NewSource(int64(size)))
	for i := range data {
		data[i] = uint8(random.Intn(256))
	}

	filename := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(filename, data, 0666); err != nil {
		t.Fatal(err)
	}

	return filename, data
}

// buildDiskCopy wraps sectors and tags in a DiskCopy 4.2 header
func buildDiskCopy(name string, data []uint8, tags []uint8) []uint8 {
	out := make([]uint8, diskCopyHeaderSize+len(data)+len(tags))

	out[0] = uint8(len(name))
	copy(out[1:], name)

	binary.BigEndian.PutUint32(out[64:68], uint32(len(data)))
	binary.BigEndian.PutUint32(out[68:72], uint32(len(tags)))
	binary.BigEndian.PutUint32(out[72:76], diskCopyChecksum(data))
	binary.BigEndian.PutUint32(out[76:80], diskCopyChecksum(tags[min(TagSize, len(tags)):]))
	out[80] = diskCopyEncoding800K
	out[81] = formatDoubleSided
	binary.BigEndian.PutUint16(out[82:84], diskCopyPrivate)

	copy(out[diskCopyHeaderSize:], data)
	copy(out[diskCopyHeaderSize+len(data):], tags)

	return out
}

func TestAPlainImageIsRecognisedBySize(t *testing.T) {
	for _, c := range []struct {
		size  int
		sides int
	}{{floppySize400K, 1}, {floppySize800K, 2}} {
		filename, _ := writeFloppyImage(t, "plain.dsk", c.size)

		kind, err := Classify(filename)
		if err != nil {
			t.Fatal(err)
		}
		if kind != KindFloppy {
			t.Errorf("a %v byte image was taken for a %v", c.size, kind)
		}

		disk, err := NewFloppyDisk(filename, false)
		if err != nil {
			t.Fatal(err)
		}
		if disk.Sides() != c.sides {
			t.Errorf("a %v byte image has %v sides, wanted %v",
				c.size, disk.Sides(), c.sides)
		}
	}
}

/*
A single sided diskette has nothing on its far side, and asking for it has to
say so rather than handing back a track of something. The drive is double
sided either way, which is what the machine is told, so it is entitled to ask.
*/
func TestTheFarSideOfASingleSidedDisketteIsNotThere(t *testing.T) {
	filename, _ := writeFloppyImage(t, "single.dsk", floppySize400K)

	disk, err := NewFloppyDisk(filename, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := disk.ReadTrack(0, 0); err != nil {
		t.Errorf("the near side of a single sided diskette did not read: %v", err)
	}
	if _, err := disk.ReadTrack(0, 1); err == nil {
		t.Error("the far side of a single sided diskette read as if it were there")
	}
}

func TestATrackPastTheEndIsRefused(t *testing.T) {
	filename, _ := writeFloppyImage(t, "edge.dsk", floppySize800K)

	disk, err := NewFloppyDisk(filename, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := disk.ReadTrack(TracksPerSide, 0); err == nil {
		t.Error("a track past the last one read as if it were there")
	}
	if _, err := disk.ReadTrack(TracksPerSide-1, 1); err != nil {
		t.Errorf("the last track did not read: %v", err)
	}
}

func TestAnImageOfTheWrongSizeIsRefused(t *testing.T) {
	filename, _ := writeFloppyImage(t, "odd.dsk", 123456)

	if _, err := NewFloppyDisk(filename, false); err == nil {
		t.Error("an image that is neither 400K nor 800K was accepted")
	}
}

/*
A DiskCopy image is recognised by its header rather than by its size, and has
to be, since it is bigger than the diskette it holds. The name it carries is
what the diskette is called from then on, and the tags come back as they were.
*/
func TestADiskCopyImageIsReadAndWrittenBack(t *testing.T) {
	data := make([]uint8, floppySize800K)
	random := rand.New(rand.NewSource(1))
	for i := range data {
		data[i] = uint8(random.Intn(256))
	}

	tags := make([]uint8, len(data)/BlockSize*TagSize)
	for i := range tags {
		tags[i] = uint8(random.Intn(256))
	}

	filename := filepath.Join(t.TempDir(), "image.dc42")
	if err := os.WriteFile(filename, buildDiskCopy("Work Disk", data, tags), 0666); err != nil {
		t.Fatal(err)
	}

	if kind, err := Classify(filename); err != nil || kind != KindFloppy {
		t.Fatalf("a DiskCopy image was taken for a %v, %v", kind, err)
	}

	disk, err := NewFloppyDisk(filename, false)
	if err != nil {
		t.Fatal(err)
	}

	if disk.Name() != "Work Disk" {
		t.Errorf("the diskette is called %q, wanted the name in the header", disk.Name())
	}
	if disk.Sides() != 2 {
		t.Errorf("the diskette has %v sides, wanted 2", disk.Sides())
	}

	/*
		Read a track and write it straight back. Nothing has changed, but
		the diskette does not know that, so the image is rebuilt with a
		header of izmac's own and has to come back the same.
	*/
	const track, side = 5, 1

	nibbles, err := disk.ReadTrack(track, side)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := disk.WriteTrack(track, side, nibbles)
	if err != nil {
		t.Fatal(err)
	}
	if stored != SectorsInTrack(track) {
		t.Fatalf("%v sectors came back off the track, wanted %v",
			stored, SectorsInTrack(track))
	}
	if err := disk.Flush(); err != nil {
		t.Fatal(err)
	}

	back, err := NewFloppyDisk(filename, false)
	if err != nil {
		t.Fatalf("the image izmac wrote does not read back: %v", err)
	}

	for i := range data {
		if back.data[i] != data[i] {
			t.Fatalf("the sectors differ at %v after the round trip", i)
		}
	}
	for i := range tags {
		if back.tags[i] != tags[i] {
			t.Fatalf("the tags differ at %v after the round trip", i)
		}
	}
}

/*
A plain image has nowhere to keep the tags, so they are zeros. They still have
to survive a track going out and coming back, or every write would put twelve
bytes of something else in front of every sector.
*/
func TestAPlainImageKeepsItsTagsZero(t *testing.T) {
	filename, data := writeFloppyImage(t, "plain.dsk", floppySize800K)

	disk, err := NewFloppyDisk(filename, false)
	if err != nil {
		t.Fatal(err)
	}

	const track, side = 0, 0

	nibbles, err := disk.ReadTrack(track, side)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := disk.WriteTrack(track, side, nibbles); err != nil {
		t.Fatal(err)
	}
	if err := disk.Flush(); err != nil {
		t.Fatal(err)
	}

	written, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	if len(written) != len(data) {
		t.Fatalf("the image is %v bytes after the round trip, wanted %v",
			len(written), len(data))
	}
	for i := range data {
		if written[i] != data[i] {
			t.Fatalf("the image differs at %v after a track went out and came back", i)
		}
	}
}

func TestALockedDisketteIsNeverWritten(t *testing.T) {
	filename, data := writeFloppyImage(t, "locked.dsk", floppySize800K)

	disk, err := NewFloppyDisk(filename, true)
	if err != nil {
		t.Fatal(err)
	}
	if !disk.IsReadOnly() {
		t.Fatal("a diskette opened locked does not report itself locked")
	}

	nibbles, err := disk.ReadTrack(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := disk.WriteTrack(0, 0, nibbles); err == nil {
		t.Error("a locked diskette accepted a track")
	}

	written, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	for i := range data {
		if written[i] != data[i] {
			t.Fatalf("the image of a locked diskette changed at %v", i)
		}
	}
}

/*
A file the host will not let izmac write to is still a usable diskette, and
the machine sees it as a locked one. That is the read only path that is not
asked for: the caller wanted to write to it and could not.
*/
func TestAnUnwritableImageComesUpLocked(t *testing.T) {
	filename, _ := writeFloppyImage(t, "readonly.dsk", floppySize800K)

	if err := os.Chmod(filename, 0444); err != nil {
		t.Fatal(err)
	}

	disk, err := NewFloppyDisk(filename, false)
	if err != nil {
		t.Fatal(err)
	}
	if !disk.IsReadOnly() {
		t.Error("an image the host refuses to open for writing is not reported locked")
	}
}
