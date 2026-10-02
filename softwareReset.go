package izmac

/*
The RESET instruction, which is how the Finder of the first Systems restarts
the machine, and how Shut Down ends on them.

The Finder of System 2.0 has no Shut Down Manager to call. Its Shut Down puts
the disks away, ejects the diskettes from both drives and executes RESET, and
on a real Macintosh Plus that is the end of it: the machine starts again,
finds no disk, and shows the flashing question mark, which is when it is
switched off. That code was read off the Finder of the MacPaint diskette, in
memory after it had run; it checks the ROM header for the Macintosh XL first,
which needs something else.

On a 68000 the RESET instruction only asserts the reset line, for the chips on
the board, and carries on with the next instruction; the processor itself is
not reset. That the Macintosh restarts all the same is something of the board,
not of the processor, and Mini vMac makes the same call in the same place: on
the machines up to the Plus, a RESET instruction resets the whole machine.
Without that the Finder carries on after its Shut Down as though nothing had
happened, and the first thing it reads is on the diskette it has just ejected.

The ROM never executes RESET itself, so a machine starting up does not come
back here.

iz68000 tells the board when RESET asserts the line, through the ResetLine
interface the memory manager implements. That call comes from inside the
instruction, and starting the machine again resets the processor as well, so
the memory manager only takes note and the run loop does the rest once the
instruction has returned.
*/

/*
softwareReset restarts the machine the way the reset line does. When it
leaves the machine nothing to start from, no diskette in a drive and no disk
on the bus, it is a Shut Down: the machine is ready to be switched off for as
long as it waits for a disk.
*/
func (m *Mac) softwareReset() {
	m.reset()
	m.shutDownByReset = !m.hasStartupDisk()
}

// hasStartupDisk tells whether there is anything the machine could start from
func (m *Mac) hasStartupDisk() bool {
	if len(m.scsi.Attached()) != 0 {
		return true
	}
	for _, d := range m.GetDiskettes() {
		if d.Image != "" {
			return true
		}
	}
	return false
}
