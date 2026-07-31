package main

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/ivanizag/izmac"
)

/*
Putting a diskette in a drive by dropping the image on the window, and taking
one out from the menu.

Only the first is something the machine cannot do for itself. The Finder ejects
a disk by dragging it to the trash and the driver drives the eject line, so the
menu item is for a disk the machine has stopped believing in rather than the
usual way out.
*/

/*
pathOfDropped returns the path of the first file dropped on the window, if one
was.

Ebiten hands the files over as a file system rather than as paths, and izmac
needs the path: a diskette is written back to where it came from. It is
recovered by opening the file and asking what it was opened as.

Nothing has been dropped on almost every frame there is, and the file system is
nil then, so that has to be the quiet answer rather than a crash. A file that is
not on a real disk is passed over as well, which is what a browser hands out:
the file exists only inside the page and there would be nowhere to write a
changed diskette back to. So is a folder, which holds no diskette.
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
			continue
		}

		file, err := dropped.Open(entry.Name())
		if err != nil {
			continue
		}

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
for the menu to say.
*/
func insertDropped(m *izmac.Mac, filename string) string {
	drive := izmac.DriveInternal
	name := "internal"

	if m.GetDiskette(izmac.DriveInternal).Image != "" &&
		m.GetDiskette(izmac.DriveExternal).Image == "" {
		drive = izmac.DriveExternal
		name = "external"
	}

	m.SendDisketteCommand(izmac.CommandInsertDiskette, drive, filename)

	return fmt.Sprintf("Put %v in the %v drive", baseName(filename), name)
}

/*
baseName is an image without the directories in front of it, shortened from the
middle if it is still long, so that it fits the line it is drawn on.

The cut is by characters and not by bytes: a name with an accent in it would
otherwise be cut through the middle of one and drawn with a replacement
character where it happened.
*/
func baseName(path string) string {
	name := path
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			name = path[i+1:]
			break
		}
	}

	const (
		longest = 24
		gap     = 3 // The "..." that goes where the middle was
	)

	letters := []rune(name)
	if len(letters) > longest {
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
			label: func(m *izmac.Mac) string {
				diskette := m.GetDiskette(drive)
				if diskette.Image == "" {
					return fmt.Sprintf("%v drive: empty", diskette.Name)
				}

				locked := ""
				if diskette.ReadOnly {
					locked = ", locked"
				}
				return fmt.Sprintf("Eject %v%v", baseName(diskette.Image), locked)
			},

			action: func(mn *menu) {
				diskette := mn.m.GetDiskette(drive)
				if diskette.Image == "" {
					return
				}

				mn.m.SendDisketteCommand(izmac.CommandEjectDiskette, drive, "")
				mn.say(fmt.Sprintf("Ejected %v", baseName(diskette.Image)))
				mn.open = false
			},
		})
	}

	return items
}
