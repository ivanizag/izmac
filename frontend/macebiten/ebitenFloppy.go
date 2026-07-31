package main

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ivanizag/izmac"
)

/*
Putting a diskette in a drive by dropping the image on the window, which is
the one thing the machine can not ask for itself. Taking one out it can: the
Finder drags a disk to the trash and the driver drives the eject line, and the
menu offers it as well for a disk the machine has stopped believing in.

Ebiten hands the dropped files over as a file system rather than as paths, and
izmac needs the path: a diskette is written back to where it came from when
the machine changes it, and a file system that only opens for reading has
nowhere to put it. The path is recoverable all the same, because on a desktop
the file the virtual system opens is a real one and an os.File remembers what
it was opened as.
*/

// droppedFile returns the path of a file dropped on the window, if one was
func droppedFile() (string, bool) {
	return pathOfDropped(ebiten.DroppedFiles())
}

/*
pathOfDropped picks the path out of the file system ebiten hands over. It
takes the file system rather than fetching it so that it can be tested, which
is worth the extra function: nothing dropped is the answer on almost every
frame and it has to be the quiet one.

Ebiten leaves the file system nil until something is dropped and sets it back
to nil once the frame that saw it is over, so a drop arrives exactly once and
nil is what this is normally asked about.
*/
func pathOfDropped(dropped fs.FS) (string, bool) {
	if dropped == nil {
		return "", false
	}

	entries, err := fs.ReadDir(dropped, ".")
	if err != nil {
		return "", false
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// A folder was dropped, and there is no diskette in that
			continue
		}

		file, err := dropped.Open(entry.Name())
		if err != nil {
			continue
		}

		/*
			The path, if this is a real file on a real disk. It is not on a
			browser, where a dropped file only exists inside the page, and
			there izmac would have nowhere to write a changed diskette back
			to. An os.File remembers what it was opened as, which is how the
			path is recovered from a file system that only offers names.
		*/
		real, ok := file.(*os.File)
		if !ok {
			file.Close()
			continue
		}

		name := real.Name()
		file.Close()

		return name, true
	}

	return "", false
}

/*
insertDropped puts a dropped image in a drive, the internal one unless it is
taken and the external one is free. Which drive it went in is what comes back,
or a message saying why it did not.
*/
func insertDropped(m *izmac.Mac, filename string) string {
	drive := izmac.DriveInternal
	name := "internal"

	diskettes := m.GetDiskettes()
	if diskettes[izmac.DriveInternal].Image != "" &&
		diskettes[izmac.DriveExternal].Image == "" {
		drive = izmac.DriveExternal
		name = "external"
	}

	m.SendDisketteCommand(izmac.CommandInsertDiskette, drive, filename)

	return fmt.Sprintf("Put %v in the %v drive", baseName(filename), name)
}

/*
baseName is the file without the directories in front of it, shortened from
the middle if it is still too long. A menu line is drawn at a fixed width and
a name that runs past it would be drawn over the screen of the machine.
*/
func baseName(filename string) string {
	name := filename
	for i := len(filename) - 1; i >= 0; i-- {
		if filename[i] == '/' || filename[i] == '\\' {
			name = filename[i+1:]
			break
		}
	}

	/*
		Cut by characters and not by bytes: a name with an accent in it
		would otherwise be cut through the middle of one and drawn with a
		replacement character where it happened.
	*/
	const longest = 24

	letters := []rune(name)
	if len(letters) > longest {
		const gap = 3 // The "..." that goes where the middle was

		front := (longest - gap) / 2
		back := longest - gap - front
		name = string(letters[:front]) + "..." + string(letters[len(letters)-back:])
	}

	return name
}

/*
disketteItems are the menu lines for the two drives: what is in each of them,
and taking it out again. A drive with nothing in it says so and does nothing,
rather than disappearing, so that the lines of the menu stay where they were
between one look and the next.
*/
func disketteItems() []menuItem {
	items := make([]menuItem, 0, izmac.DriveCount)

	for drive := 0; drive < izmac.DriveCount; drive++ {
		items = append(items, menuItem{
			label:  disketteLabel(drive),
			action: ejectDiskette(drive),
		})
	}

	return items
}

func disketteLabel(drive int) func(m *izmac.Mac) string {
	return func(m *izmac.Mac) string {
		diskette := m.GetDiskettes()[drive]

		if diskette.Image == "" {
			return fmt.Sprintf("%v drive: empty", diskette.Name)
		}

		locked := ""
		if diskette.ReadOnly {
			locked = ", locked"
		}
		return fmt.Sprintf("Eject %v%v", baseName(diskette.Image), locked)
	}
}

func ejectDiskette(drive int) func(mn *menu) {
	return func(mn *menu) {
		diskette := mn.m.GetDiskettes()[drive]
		if diskette.Image == "" {
			return
		}

		mn.m.SendDisketteCommand(izmac.CommandEjectDiskette, drive, "")
		mn.say(fmt.Sprintf("Ejected %v", baseName(diskette.Image)))
		mn.open = false
	}
}
