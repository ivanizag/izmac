package main

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ivanizag/izmac"
)

/*
Putting a diskette in a drive by dropping the image on the window, which is
the one thing the machine can not ask for itself. Taking one out it can: the
Finder drags a disk to the trash and the driver drives the eject line, and the
menu offers it as well for a disk the machine has stopped believing in.

Nothing here is more than glue. Ebiten hands the dropped files over as a file
system rather than as paths and izmac needs the path, but working it out is
izmac.PathOfDroppedImage rather than something here: this package cannot be
tested, since importing ebiten brings in an initializer that wants a window
and a test runs without one.
*/

// droppedFile returns the path of a file dropped on the window, if one was
func droppedFile() (string, bool) {
	return izmac.PathOfDroppedImage(ebiten.DroppedFiles())
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

	return fmt.Sprintf("Put %v in the %v drive", izmac.ShortImageName(filename), name)
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
		return fmt.Sprintf("Eject %v%v", izmac.ShortImageName(diskette.Image), locked)
	}
}

func ejectDiskette(drive int) func(mn *menu) {
	return func(mn *menu) {
		diskette := mn.m.GetDiskettes()[drive]
		if diskette.Image == "" {
			return
		}

		mn.m.SendDisketteCommand(izmac.CommandEjectDiskette, drive, "")
		mn.say(fmt.Sprintf("Ejected %v", izmac.ShortImageName(diskette.Image)))
		mn.open = false
	}
}
