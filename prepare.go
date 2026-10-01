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

A file that is already a disk image goes on as it always has, read and written
in place. Anything that had to be unpacked or mended is held in memory instead,
and gone when izmac stops: the file named is never touched. The machine can
write to it, and what it writes lasts until izmac stops.

With -persist the images are written out instead, as izmac_ files on the
working directory named after what they came out of, and attached from there
like any other image, writable. On the next run with -persist the file is
already there and is used as it is, so what the machine saved on it is kept
rather than unpacked over.
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
is taken as it is. An archive can hold any number of them, and the files in it
that are not disk images are left out: putting those on a volume of their own
is not something izmac does yet.

It changes nothing on the configuration, so that a file dropped on the window
of a running machine goes through it as well.
*/
func (c *Configuration) prepare(filename string) ([]preparedImage, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("can not open the disk image: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
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
diskImagesIn unpacks an archive and keeps the disk images in it, mending the
padded diskettes on the way
*/
func (c *Configuration) diskImagesIn(unwrapper *unwrap.Unwrapper, filename string,
	data []uint8) ([]unwrap.File, error) {

	unpacked, err := unwrapper.Unwrap(filepath.Base(filename), data)
	if err != nil {
		return nil, err
	}

	var images []unwrap.File
	var others []string
	for _, f := range unpacked {
		head := f.Data[:min(len(f.Data), unwrap.HeadSize)]
		if floppy, padded := storage.PaddedFloppySize(head, int64(len(f.Data))); padded {
			f.Data = f.Data[:floppy]
		}

		if storage.IsDiskImage(f.Data) {
			images = append(images, f)
		} else {
			others = append(others, f.Name)
		}
	}

	if len(images) == 0 {
		if len(others) == 0 {
			return nil, fmt.Errorf("%v holds nothing", filename)
		}
		return nil, fmt.Errorf("%v holds no disk image, only files that would have to "+
			"be put on one, which izmac does not do yet: %v", filename, listNames(others))
	}

	if len(others) != 0 {
		fmt.Fprintf(c.out(), "  - left out, not disk images: %v\n", listNames(others))
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

		kept, err := c.persist(filename, f, several, used)
		if err != nil {
			return nil, err
		}
		images = append(images, kept)
	}

	return images, nil
}

/*
persist writes an image out to the working directory, or finds it there from
an earlier run and leaves it alone, which is the whole point: what the machine
saved on it last time is on it still.
*/
func (c *Configuration) persist(filename string, f unwrap.File, several bool,
	used map[string]bool) (preparedImage, error) {

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
	name := unique(persistPrefix+stem, used) + ".dsk"

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
	if c.messages == nil {
		return io.Discard
	}
	return c.messages
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
