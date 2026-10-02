package unwrap

import (
	"os"
	"path/filepath"
)

/*
Reading Macintosh files that are on the host as they are, a file or a folder of
them, named on the command line or dropped on the window rather than packed in
an archive.

What makes them Macintosh files is kept wherever the host keeps it. macOS has
the resource fork and the Finder information as part of the file, and they are
read from there. Any host can have them in AppleDouble files next to the
others, which is what macOS itself leaves on a volume that cannot hold them, a
USB stick most of the time.
*/

/*
ReadHost reads a file or a folder of the host. A folder comes back as the files
in it, each in the folders it is in below the one named; a file comes back
alone.
*/
func ReadHost(name string) ([]File, error) {
	info, err := os.Stat(name)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		loose, err := readFolder(name)
		if err != nil {
			return nil, err
		}

		files := assemble(loose)
		for i := range files {
			where := filepath.Join(append(append([]string{name}, files[i].Folders...),
				files[i].Name)...)
			readHostMetadata(where, &files[i])
		}
		return files, nil
	}

	data, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	file := File{
		Name:     filepath.Base(name),
		Data:     data,
		Modified: info.ModTime(),
	}

	sidecar := filepath.Join(filepath.Dir(name), appleDoublePrefix+file.Name)
	if raw, err := os.ReadFile(sidecar); err == nil {
		if a, ok := parseAppleDouble(raw); ok {
			a.apply(&file)
		}
	}
	readHostMetadata(name, &file)

	return []File{file}, nil
}

/*
HasHostMetadata tells whether the host keeps a resource fork or Finder
information for a file, which is what says a file is a Macintosh one before
it is read
*/
func HasHostMetadata(name string) bool {
	sidecar := filepath.Join(filepath.Dir(name), appleDoublePrefix+filepath.Base(name))
	if _, err := os.Stat(sidecar); err == nil {
		return true
	}

	var f File
	readHostMetadata(name, &f)
	return f.IsMacFile()
}
