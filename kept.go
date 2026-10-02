package izmac

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ivanizag/izmac/storage"
)

/*
The record of the images kept with -persist: which izmac_ files on the working
directory came out of which archive, folder or file, and in what order.

It is what lets a second run go straight to the kept images without unpacking
the archive or packing the folder again, work that is thrown away once the
images are found to be there already, and that fails outright for a folder
that has since grown too big or gone. And it is what tells two sources with
the same name apart: a folder called Letters in one place and another in
another place would both be kept as izmac_Letters.dsk, and the second would be
handed what the first one saved.

The sources are known by their absolute path, so an archive that is moved is a
new one, and is unpacked again under a name of its own.
*/

// keptRecordFile is the record, on the working directory with the images
const keptRecordFile = "izmac_kept.json"

// keptImage is one image kept, and the name it had in what it came out of
type keptImage struct {
	Image string `json:"image"`
	Label string `json:"label"`
}

// keptRecord is the record as it is kept, the images of each source in order
type keptRecord struct {
	Sources map[string][]keptImage `json:"sources"`
}

// loadKeptRecord reads the record, which is empty until something is kept
func loadKeptRecord() (*keptRecord, error) {
	record := &keptRecord{Sources: make(map[string][]keptImage)}

	data, err := os.ReadFile(keptRecordFile)
	if errors.Is(err, os.ErrNotExist) {
		return record, nil
	}
	if err != nil {
		return nil, fmt.Errorf("can not read %v: %w", keptRecordFile, err)
	}
	if err := json.Unmarshal(data, record); err != nil {
		return nil, fmt.Errorf("%v is not a record of kept images: %w", keptRecordFile, err)
	}
	if record.Sources == nil {
		record.Sources = make(map[string][]keptImage)
	}
	return record, nil
}

func (r *keptRecord) save() error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(keptRecordFile, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("can not write %v: %w", keptRecordFile, err)
	}
	return nil
}

// owner is the source an image was kept for, if the record knows of it
func (r *keptRecord) owner(image string) (string, bool) {
	for source, images := range r.Sources {
		for _, kept := range images {
			if kept.Image == image {
				return source, true
			}
		}
	}
	return "", false
}

// sourceKey is how a source is known in the record
func sourceKey(filename string) string {
	if absolute, err := filepath.Abs(filename); err == nil {
		return absolute
	}
	return filename
}

/*
keptImages finds the images kept for a source on an earlier run. When one of
them is missing the source is prepared again, which makes the missing ones
anew and finds the others where they were.
*/
func (c *Configuration) keptImages(filename string) ([]preparedImage, bool, error) {
	record, err := loadKeptRecord()
	if err != nil {
		return nil, false, err
	}

	kept := record.Sources[sourceKey(filename)]
	if len(kept) == 0 {
		return nil, false, nil
	}
	for _, k := range kept {
		if _, err := os.Stat(k.Image); err != nil {
			return nil, false, nil
		}
	}

	fmt.Fprintf(c.out(), "Using what was kept of %v on an earlier run\n", filename)
	images := make([]preparedImage, 0, len(kept))
	for _, k := range kept {
		kind, err := storage.Classify(k.Image)
		if err != nil {
			return nil, false, err
		}
		images = append(images, preparedImage{
			name:   k.Image,
			kind:   kind,
			label:  k.Label,
			report: fmt.Sprintf("%v, kept as %v", k.Label, k.Image),
		})
	}
	return images, true, nil
}

/*
keptName chooses the file an image is kept in. The name it would have is taken
as long as it is free or this source's own already. A file there that the
record does not know of was kept before there was a record, and is taken as
this source's. One the record gives to another source is not, and the name
is numbered until one is found that is not.
*/
func keptName(base string, source string, record *keptRecord, used map[string]bool) string {
	for n := 1; ; n++ {
		candidate := base
		if n > 1 {
			candidate = fmt.Sprintf("%v %v", base, n)
		}
		name := candidate + ".dsk"
		if used[name] {
			continue
		}
		if owner, owned := record.owner(name); owned && owner != source {
			continue
		}
		used[name] = true
		return name
	}
}
