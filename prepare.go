package izmac

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ivanizag/izmac/storage"
	"github.com/ivanizag/izmac/unwrap"
)

/*
Turning whatever was named on the command line, or dropped on the window, into
disk images the machine can use. The idea comes from macprep
(https://github.com/mastorak/macprep), which turns the downloads of classic
Macintosh software into images for Mini vMac: an archive is unpacked, as many
times as it takes, and the disk images that come out of it are attached.

Files that are not disk images, the application in an archive or a folder of
documents on the host, are put on a new volume made for them, an 800Kb
diskette when they fit on one and a hard disk when they do not.

A file that is already a disk image goes on as it always has, read and written
in place. Anything that had to be unpacked or mended is held in memory instead,
and gone when izmac stops: the file named is never touched. The machine can
write to it, and what it writes lasts until izmac stops.

With -persist the images are written out instead, as izmac_ files on the
working directory named after what they came out of, and attached from there
like any other image, writable. A record of which came from where, kept.go,
lets the next run with -persist go straight to them, so what the machine saved
on them is kept rather than unpacked over.
*/

// persistPrefix is what the images kept with -persist are named with, the
// prefix everything izmac writes for itself carries
const persistPrefix = "izmac_"

// preparedImage is a disk image out of something named, ready to attach
type preparedImage struct {
	/*
		name is what the image goes by in the lists of the configuration.
		For an image on the host it is the file. For one held in memory it
		is made of the file it came out of and its own name, as though the
		archive were a folder, which is unique and reads well in a menu.
	*/
	name string
	kind storage.Kind

	// data is the image if it is held in memory, nil if it is a file
	data []uint8

	/*
		report says what became of an image that had to be unpacked or
		mended, empty for a disk image named as it is. It is printed when
		the image finds its place, so that an image left out for want of
		room is not first announced as added.
	*/
	report string

	// label is the name the image had inside what it came out of
	label string
}

/*
prepare works out the disk images a file named holds. A disk image is one, and
is taken as it is. An archive can hold any number of them, and what else is in
it goes on a new volume. A folder, or a file that is neither a disk image nor
an archive, goes on a new volume of its own.

It changes nothing on the configuration, so that a file dropped on the window
of a running machine goes through it as well.
*/
func (c *Configuration) prepare(filename string) ([]preparedImage, error) {
	// What was kept on an earlier run is used without looking any further,
	// so a folder that has changed since is not packed for nothing
	if c.Persist {
		images, found, err := c.keptImages(filename)
		if err != nil {
			return nil, err
		}
		if found {
			return images, nil
		}
	}

	info, err := os.Stat(filename)
	if err != nil {
		return nil, fmt.Errorf("can not open the disk image: %w", err)
	}
	if info.IsDir() {
		return c.prepareLoose(filename, "a folder")
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("can not open the disk image: %w", err)
	}
	defer file.Close()
	size := info.Size()

	head := make([]uint8, min(size, unwrap.HeadSize))
	if _, err := io.ReadFull(file, head); err != nil {
		return nil, fmt.Errorf("can not read %v: %w", filename, err)
	}

	unwrapper := unwrap.NewUnwrapper()
	format := unwrapper.Identify(filename, head, size)

	var found []unwrap.File
	if format == "" {
		floppy, padded := storage.PaddedFloppySize(head, size)
		if !padded {
			if isLooseFile(filename, head, size) {
				return c.prepareLoose(filename, "not a disk image")
			}

			// A disk image as it comes, the usual case by far
			kind, err := storage.Classify(filename)
			if err != nil {
				return nil, err
			}
			return []preparedImage{{name: filename, kind: kind}}, nil
		}

		data := make([]uint8, floppy)
		if _, err := file.ReadAt(data, 0); err != nil {
			return nil, fmt.Errorf("can not read %v: %w", filename, err)
		}
		fmt.Fprintf(c.out(), "%v is a diskette padded to %v bytes, cut back to %vKb\n",
			filename, size, floppy/1024)
		found = []unwrap.File{{Name: filepath.Base(filename), Data: data}}
	} else {
		fmt.Fprintf(c.out(), "Unpacking %v, %v\n", filename, unwrap.Describe(format))

		data := make([]uint8, size)
		_, err := file.ReadAt(data, 0)
		if err != nil {
			return nil, fmt.Errorf("can not read %v: %w", filename, err)
		}
		found, err = c.diskImagesIn(unwrapper, filename, data)
		if err != nil {
			return nil, err
		}
	}

	return c.place(filename, found, format == "")
}

/*
isLooseFile tells a file named that is no disk image at all, to be put on a
volume of its own, from one that is. A disk image says what it is in its first
blocks, or is a blank one waiting to be formatted from the machine and is all
zeros there; anything else is a file.

Two things settle it before the zeros are looked at. A file too small to hold
a block is no disk, and that is what an application named from macOS looks
like: all of it is in the resource fork and the data fork is empty. And one
the host keeps a resource fork or Finder information for is a Macintosh file,
unless it said it was a disk image first, which a DiskCopy image copied off a
Macintosh does while carrying both.
*/
func isLooseFile(filename string, head []uint8, size int64) bool {
	if size < storage.BlockSize {
		return true
	}
	if storage.LooksLikeDiskImage(head, size) {
		return false
	}
	if unwrap.HasHostMetadata(filename) {
		return true
	}

	blank := size%storage.BlockSize == 0
	for _, b := range head {
		if b != 0 {
			blank = false
			break
		}
	}
	return !blank
}

/*
prepareLoose puts a folder of the host, or a file that is not a disk image, on
a new volume named after it
*/
func (c *Configuration) prepareLoose(filename string, what string) ([]preparedImage, error) {
	files, err := unwrap.ReadHost(filename)
	if err != nil {
		return nil, fmt.Errorf("can not read %v: %w", filename, err)
	}

	var kept []unwrap.File
	for _, f := range files {
		if !f.IsClutter() {
			kept = append(kept, f)
		}
	}
	if len(kept) == 0 {
		return nil, fmt.Errorf("%v holds no files", filename)
	}

	fmt.Fprintf(c.out(), "Packing %v, %v\n", filename, what)
	volume, err := c.packVolume(unwrap.Stem(filepath.Base(filename)), kept)
	if err != nil {
		return nil, err
	}
	return c.place(filename, []unwrap.File{volume}, true)
}

/*
diskImagesIn unpacks an archive and keeps the disk images in it, mending the
padded diskettes on the way. What else is in it goes on a new volume: all of
it when there are no disk images, and only the Macintosh files when there are,
since what comes with a disk image is a read me or a checksum for the host.
*/
func (c *Configuration) diskImagesIn(unwrapper *unwrap.Unwrapper, filename string,
	data []uint8) ([]unwrap.File, error) {

	unpacked, err := unwrapper.Unwrap(filepath.Base(filename), data)
	if err != nil {
		return nil, err
	}

	var images, others []unwrap.File
	for _, f := range unpacked {
		if f.IsClutter() {
			continue
		}

		head := f.Data[:min(len(f.Data), unwrap.HeadSize)]
		if floppy, padded := storage.PaddedFloppySize(head, int64(len(f.Data))); padded {
			f.Data = f.Data[:floppy]
		}

		// A file with a resource fork is an application or a document,
		// whatever its data fork looks like, but for the images DiskCopy
		// made, which keep their checksums in one
		if (len(f.Resource) == 0 || f.IsDiskCopyImage()) && storage.IsDiskImage(f.Data) {
			images = append(images, f)
		} else {
			others = append(others, f)
		}
	}

	stem := unwrap.Stem(filepath.Base(filename))
	if len(images) == 0 {
		if len(others) == 0 {
			return nil, fmt.Errorf("%v holds nothing", filename)
		}
		volume, err := c.packVolume(stem, others)
		if err != nil {
			return nil, err
		}
		return []unwrap.File{volume}, nil
	}

	var packed []unwrap.File
	var left []string
	for _, f := range others {
		if f.IsMacFile() {
			packed = append(packed, f)
		} else {
			left = append(left, f.Name)
		}
	}
	if len(left) != 0 {
		fmt.Fprintf(c.out(), "  - left out, not disk images: %v\n", listNames(left))
	}
	if len(packed) != 0 {
		volume, err := c.packVolume(stem+" Files", packed)
		if err != nil {
			return nil, err
		}
		images = append(images, volume)
	}

	return images, nil
}

/*
place decides where each image lives, in memory or on the working directory,
and says so. An image that is the file named, mended, goes by the name of the
file. One that came out of an archive goes by the archive and its own name,
both in memory and when it is kept.
*/
func (c *Configuration) place(filename string, found []unwrap.File, mended bool) ([]preparedImage, error) {
	several := len(found) > 1
	used := make(map[string]bool)

	var record *keptRecord
	var kept []keptImage
	if c.Persist {
		var err error
		if record, err = loadKeptRecord(); err != nil {
			return nil, err
		}
	}

	images := make([]preparedImage, 0, len(found))
	for _, f := range found {
		image := preparedImage{
			name:  filename,
			kind:  storage.ClassifyData(f.Data),
			data:  f.Data,
			label: f.Name,
		}
		if !mended {
			image.name = unique(filename+"/"+f.Name, used)
		}

		if !c.Persist {
			image.report = fmt.Sprintf("%v, %v, in memory",
				f.Name, describeImage(image.kind, len(f.Data)))
			images = append(images, image)
			continue
		}

		image, err := c.persist(filename, f, several, record, used)
		if err != nil {
			return nil, err
		}
		images = append(images, image)
		kept = append(kept, keptImage{Image: image.name, Label: f.Name})
	}

	if record != nil {
		record.Sources[sourceKey(filename)] = kept
		if err := record.save(); err != nil {
			return nil, err
		}
	}
	return images, nil
}

/*
persist writes an image out to the working directory, or finds it there from
an earlier run and leaves it alone, which is the whole point: what the machine
saved on it last time is on it still.
*/
func (c *Configuration) persist(filename string, f unwrap.File, several bool,
	record *keptRecord, used map[string]bool) (preparedImage, error) {

	stem := unwrap.SafeName(unwrap.Stem(filename))
	if stem == "" {
		stem = "disk"
	}
	if several {
		inner := unwrap.SafeName(unwrap.Stem(f.Name))
		if inner != "" {
			stem += " - " + inner
		}
	}
	name := keptName(persistPrefix+stem, sourceKey(filename), record, used)

	if _, err := os.Stat(name); err == nil {
		kind, err := storage.Classify(name)
		if err != nil {
			return preparedImage{}, err
		}
		return preparedImage{
			name:   name,
			kind:   kind,
			label:  f.Name,
			report: fmt.Sprintf("%v, kept from an earlier run as %v", f.Name, name),
		}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return preparedImage{}, err
	}

	if err := os.WriteFile(name, f.Data, 0o600); err != nil {
		return preparedImage{}, fmt.Errorf("can not keep %v: %w", f.Name, err)
	}

	kind := storage.ClassifyData(f.Data)
	return preparedImage{
		name:  name,
		kind:  kind,
		label: f.Name,
		report: fmt.Sprintf("%v, %v, saved as %v",
			f.Name, describeImage(kind, len(f.Data)), name),
	}, nil
}

/*
expand runs prepare over a list of images named with -hd or -floppy, which go
where they were told whatever they turn out to be. Names already prepared go
through untouched, so that this can run after AddFiles over the same lists. A
file that can not be found is left for opening it to complain about, which it
does better than this could.
*/
func (c *Configuration) expand(names []string, room int, what string) ([]string, error) {
	expanded := make([]string, 0, len(names))

	for _, name := range names {
		if _, inMemory := c.memoryImages[name]; inMemory {
			expanded = append(expanded, name)
			continue
		}
		if _, err := os.Stat(name); err != nil {
			expanded = append(expanded, name)
			continue
		}

		images, err := c.prepare(name)
		if err != nil {
			return nil, err
		}

		for _, image := range images {
			if len(images) > 1 && len(expanded) >= room {
				c.leftOut(image, what)
				continue
			}
			c.placed(image)
			expanded = append(expanded, image.name)
		}
	}

	return expanded, nil
}

/*
placed reports an image that found its place, and keeps it if it is held in
memory, for NewMac to find by its name
*/
func (c *Configuration) placed(image preparedImage) {
	c.report(image)

	if image.data == nil {
		return
	}
	if c.memoryImages == nil {
		c.memoryImages = make(map[string][]uint8)
	}
	c.memoryImages[image.name] = image.data
}

// report says what became of an image that had to be unpacked or mended
func (c *Configuration) report(image preparedImage) {
	if image.report != "" {
		fmt.Fprintf(c.out(), "  + %v\n", image.report)
	}
}

/*
leftOut says an image out of an archive found no room. It is not an error: an
archive of four diskettes is a set to be worked through, not a mistake, and
the first two are as good as ever. One that was kept says where, since that
is where to find it to put it in later.
*/
func (c *Configuration) leftOut(image preparedImage, what string) {
	said := image.label
	if image.data == nil {
		said = image.report
	}
	if said == "" {
		said = filepath.Base(image.name)
	}
	fmt.Fprintf(c.out(), "  - %v, left out: %v\n", said, what)
}

// openDiskette opens an image for a drive, from memory or from the host
func (c *Configuration) openDiskette(name string) (*storage.FloppyDisk, error) {
	if data, inMemory := c.memoryImages[name]; inMemory {
		return storage.NewFloppyDiskData(name, data)
	}
	return storage.NewFloppyDisk(name, false)
}

// openDiskette opens a prepared image for a drive
func (image preparedImage) openDiskette() (*storage.FloppyDisk, error) {
	if image.data != nil {
		return storage.NewFloppyDiskData(image.name, image.data)
	}
	return storage.NewFloppyDisk(image.name, false)
}

// openDisk opens an image for the bus, from memory or from the host
func (c *Configuration) openDisk(name string, scsiDriver *storage.ScsiDriver) (storage.BlockDisk, error) {
	if data, inMemory := c.memoryImages[name]; inMemory {
		return storage.NewBlockDiskData(name, data, scsiDriver)
	}
	return storage.NewBlockDisk(name, scsiDriver, false)
}

// out is where the preparing is reported
func (c *Configuration) out() io.Writer {
	if c.Messages == nil {
		return io.Discard
	}
	return c.Messages
}

// describeImage says what an image is, for the report
func describeImage(kind storage.Kind, size int) string {
	described := fmt.Sprintf("%v of %vKb", kind, size/1024)
	if kind == storage.KindFloppy {
		described = fmt.Sprintf("%vKb diskette", size/1024)
	}

	// The sizes are said aloud, and eight hundred takes an "an"
	if strings.HasPrefix(described, "8") {
		return "an " + described
	}
	return "a " + described
}

// unique gives a name not given before, numbering the ones that repeat
func unique(name string, used map[string]bool) string {
	candidate := name
	for i := 2; used[candidate]; i++ {
		candidate = fmt.Sprintf("%v %v", name, i)
	}
	used[candidate] = true
	return candidate
}

// listNames lists a few names for a message, and counts the rest
func listNames(names []string) string {
	const shown = 3
	if len(names) <= shown {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%v and %v more", strings.Join(names[:shown], ", "), len(names)-shown)
}
