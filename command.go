package izmac

import "fmt"

const commandChannelSize = 10

const (
	// CommandKill stops the emulation loop
	CommandKill = iota + 1
	// CommandReset resets the machine
	CommandReset
	// CommandPause pauses the emulation
	CommandPause
	// CommandStart resumes the emulation
	CommandStart
	// CommandPauseUnpause toggles the pause
	CommandPauseUnpause
	// CommandToggleCPUTrace toggles the tracing of the CPU execution
	CommandToggleCPUTrace
	// CommandToggleSpeed switches between the speed configured and running
	// as fast as the host can go
	CommandToggleSpeed
	// CommandShowSpeed prints the speed the emulation is reaching
	CommandShowSpeed
	// CommandInsertDiskette puts an image in one of the drives, and
	// CommandEjectDiskette takes one out. Both are sent with
	// SendDisketteCommand rather than SendCommand, since they carry which
	// drive is meant.
	CommandInsertDiskette
	CommandEjectDiskette
)

type command interface {
	getId() int
}

type commandSimple struct {
	id int
}

func (c *commandSimple) getId() int {
	return c.id
}

// SendCommand enqueues a command to the emulation goroutine
func (m *Mac) SendCommand(commandId int) {
	m.commandChannel <- &commandSimple{id: commandId}
}

// commandDiskette is a command about one of the diskette drives
type commandDiskette struct {
	id       int
	drive    int
	filename string
}

func (c *commandDiskette) getId() int {
	return c.id
}

/*
SendDisketteCommand enqueues a change to one of the diskette drives. It goes
through the channel rather than being done where it is asked for because a
frontend runs on its own goroutine and the drive belongs to the emulation:
taking a disk out from under a read would be a race over more than the drive.

The filename is only read by CommandInsertDiskette.
*/
func (m *Mac) SendDisketteCommand(commandId int, drive int, filename string) {
	m.commandChannel <- &commandDiskette{
		id:       commandId,
		drive:    drive,
		filename: filename,
	}
}

// toggleSpeed switches between the speed configured and full speed, and back
func (m *Mac) toggleSpeed() {
	if !m.IsFullSpeed() {
		m.setCycleDuration(0)
		return
	}

	back := m.config.cycleDurationNs
	if back == 0 {
		// Configured for full speed, so go back to the real one
		back = cycleDurationOf(CPUClockMhz)
	}
	m.setCycleDuration(back)
}

// executeCommands runs the pending commands, returning true when the
// emulation has to stop
func (m *Mac) executeCommands() bool {
	for {
		select {
		case c := <-m.commandChannel:
			switch c.getId() {
			case CommandKill:
				return true
			case CommandReset:
				m.reset()
			case CommandPause:
				m.paused.Store(true)
			case CommandStart:
				m.paused.Store(false)
			case CommandPauseUnpause:
				m.paused.Store(!m.paused.Load())
			case CommandToggleCPUTrace:
				m.cpuTrace = !m.cpuTrace
				m.cpu.SetTrace(m.cpuTrace)
			case CommandToggleSpeed:
				m.toggleSpeed()
			case CommandShowSpeed:
				fmt.Printf("Running at %.2f Mhz\n", m.GetCurrentFreqMHz())
			case CommandInsertDiskette, CommandEjectDiskette:
				m.executeDisketteCommand(c)
			}
		default:
			return false
		}
	}
}

// executeDisketteCommand runs a change to a drive, reporting what went wrong
// on the standard output. There is nowhere else to put it: the frontend that
// asked for it has gone on with its own frame by now.
func (m *Mac) executeDisketteCommand(c command) {
	command, ok := c.(*commandDiskette)
	if !ok {
		return
	}

	var err error
	if command.id == CommandInsertDiskette {
		err = m.InsertDiskette(command.drive, command.filename)
	} else {
		err = m.EjectDiskette(command.drive)
	}

	if err != nil {
		fmt.Printf("Floppy: %v\n", err)
	}
}
