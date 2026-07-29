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
			}
		default:
			return false
		}
	}
}
