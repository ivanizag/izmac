package unwrap

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

/*
The archivers left to unar, the command line half of The Unarchiver: StuffIt
in all its versions, Compact Pro, 7-Zip and RAR. Between them they have more
compression methods than are worth writing again, and unar has every one.

It is only run when it is installed, and only for a file that looks like one of
these, so a machine named with nothing but disk images never goes looking for
it. Without it, an archive that needs it is turned away with a reason rather
than being attached as the hard disk it is not.
*/

const (
	unarCommand = "unar"

	// unarTimeout bounds how long an archive is given to unpack
	unarTimeout = 2 * time.Minute
)

func defaultLookPath(file string) (string, error) {
	return exec.LookPath(file)
}

/*
identifyForUnar names the archivers unar is used for. Most say what they are
in their first bytes. Compact Pro does not say it clearly enough to be told
from anything else, so it is known by its extension.
*/
func identifyForUnar(name string, head []uint8) string {
	switch {
	case isStuffItClassic(head):
		return "StuffIt"
	case hasPrefix(head, "StuffIt (c)1997-"):
		return "StuffIt 5"
	case hasPrefix(head, "StuffIt!") || hasPrefix(head, "StuffIt?"):
		return "StuffIt X"
	case hasPrefix(head, "7z\xbc\xaf\x27\x1c"):
		return "7-Zip"
	case hasPrefix(head, "Rar!\x1a\x07"):
		return "RAR"
	}

	switch strings.ToLower(path.Ext(name)) {
	case ".cpt":
		return "Compact Pro"
	case ".sitx":
		return "StuffIt X"
	}

	return ""
}

/*
isStuffItClassic recognises the archives of StuffIt 1 to 4: one of a handful of
four letter signatures that changed with the versions, and "rLau" ten bytes in,
which never did.
*/
func isStuffItClassic(head []uint8) bool {
	if len(head) < 14 || string(head[10:14]) != "rLau" {
		return false
	}

	switch string(head[:4]) {
	case "SIT!", "ST46", "ST50", "ST60", "ST65", "STin", "STi2", "STi3", "STi4":
		return true
	}
	return false
}

/*
openWithUnar writes the archive out to a directory of its own, has unar unpack
it next to it, and reads back every file that comes out. The directory goes
away afterwards, so nothing is left on the host.

Resource forks come out as AppleDouble files, the ._ ones, which are stepped
over on the way back in. Asking unar to skip them instead would be the obvious
way, and fails: it gives up on any file that has nothing but a resource fork.
*/
func (u *Unwrapper) openWithUnar(format string, file File) ([]File, error) {
	unar, err := u.lookPath(unarCommand)
	if err != nil {
		return nil, fmt.Errorf("%v is %v, which izmac unpacks with unar, "+
			"and unar is not installed. It comes with The Unarchiver, and "+
			"most package managers carry it as unar",
			file.Name, Describe(format))
	}

	work, err := os.MkdirTemp("", "izmac-unar-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)

	// The archive keeps its name, extension and all, which is how unar
	// tells a Compact Pro archive from anything else
	name := SafeName(file.Name)
	if name == "" {
		name = "archive"
	}
	archive := filepath.Join(work, name)
	if err := os.WriteFile(archive, file.Data, 0o600); err != nil {
		return nil, err
	}

	out := filepath.Join(work, "out")
	ctx, cancel := context.WithTimeout(context.Background(), unarTimeout)
	defer cancel()

	command := exec.CommandContext(ctx, unar, "-quiet", "-force-overwrite",
		"-no-directory", "-forks", "hidden", "-output-directory", out, archive)
	if output, err := command.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("unar failed: %w: %v", err, strings.TrimSpace(string(output)))
	}

	return readUnpacked(out)
}

// readUnpacked reads back what unar left, every file in every folder, with
// the AppleDouble files put back with the files they belong to
func readUnpacked(dir string) ([]File, error) {
	loose, err := readFolder(dir)
	if err != nil {
		return nil, err
	}
	return assemble(loose), nil
}

// readFolder reads every file under a folder of the host, by its path from
// the folder
func readFolder(dir string) ([]looseFile, error) {
	var loose []looseFile

	err := filepath.WalkDir(dir, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}

		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(dir, name)
		if err != nil {
			return err
		}

		loose = append(loose, looseFile{
			path:     filepath.ToSlash(relative),
			data:     data,
			modified: info.ModTime(),
		})
		return nil
	})

	return loose, err
}

/*
SafeName makes a name off the machine usable as a file on the host, where the
slash and the colon mean something. A name with nothing usable in it comes
back empty.
*/
func SafeName(name string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r < 0x20, r == 0x7f:
			return '_'
		case strings.ContainsRune(`/\:*?"<>|`, r):
			return '_'
		}
		return r
	}, name)

	return strings.Trim(safe, " .")
}
