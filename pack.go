package izmac

import (
	"errors"
	"fmt"

	"github.com/ivanizag/izmac/hfs"
	"github.com/ivanizag/izmac/unwrap"
)

/*
Putting files that are not disk images on a new HFS volume of their own, the
way macprep (https://github.com/mastorak/macprep) does for Mini vMac.

The volume is an 800Kb diskette when the files fit on one, since a diskette
goes in a drive and can be taken out and swapped while the machine runs. When
they do not it is a hard disk with room to spare, which goes on the bus as a
bare volume. Either way it is not a startup disk: that takes boot blocks, and
those come from the System that made the disk.
*/

const (
	// packedDisketteSize is the diskette the files are put on if they fit
	packedDisketteSize = 800 * 1024

	// packedSpare is the room a hard disk is given beyond what the files
	// take, for the machine to write to
	packedSpare = 1 << 20

	megabyte = 1 << 20
)

// packVolume puts files on a new volume and returns its image, named as the
// volume is
func (c *Configuration) packVolume(name string, files []unwrap.File) (unwrap.File, error) {
	root := &hfs.Folder{}
	for i := range files {
		if files[i].Modified.After(root.Modified) {
			root.Modified = files[i].Modified
		}
	}
	for i := range files {
		f := &files[i]
		root.Add(f.Folders, &hfs.File{
			Name:     f.Name,
			Data:     f.Data,
			Resource: f.Resource,
			Type:     f.Type,
			Creator:  f.Creator,
			Flags:    f.Flags,
			Modified: f.Modified,
		})
	}

	image, err := hfs.Build(name, root, packedDisketteSize)

	// Too much for a diskette. The room the files need is only known
	// roughly until the allocation blocks are sized for the disk, so a
	// disk that turns out short is tried again a size bigger.
	var noRoom *hfs.NoRoomError
	for attempt := 0; errors.As(err, &noRoom) && attempt < 4; attempt++ {
		size := noRoom.Needed*int64(4+attempt)/3 + packedSpare
		image, err = hfs.Build(name, root, (size+megabyte-1)/megabyte*megabyte)
	}
	if err != nil {
		return unwrap.File{}, fmt.Errorf("can not put the files of %v on a volume: %w", name, err)
	}

	fmt.Fprintf(c.out(), "  Packing %v %v on a new volume, %v\n",
		root.Count(), plural(root.Count(), "file", "files"), name)
	return unwrap.File{Name: name, Data: image}, nil
}

func plural(n int, one string, many string) string {
	if n == 1 {
		return one
	}
	return many
}
