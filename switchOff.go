package izmac

/*
Knowing when the machine has been shut down, so that a frontend can close by
itself rather than leave the window up on a machine nobody can switch off.

A Macintosh Plus cannot switch itself off. Shut Down in the Finder flushes and
puts away the volumes, ejects the diskettes, and then puts up "You may now
switch off your Macintosh safely", with a Restart button, and waits for the
user to reach for the power switch. That message is a system error alert:
the System calls _SysError with shutDownAlert, 42, and the SysError of the ROM
keeps the code it was called with in DSErrCode in low memory, which is
debugger.s in the disassembly.

So the machine is ready to be switched off while DSErrCode holds 42, and that
is looked at once a frame, which is two bytes read and is soon enough for a
window that is to stay up a couple of seconds longer anyway. Restart undoes
it by itself: the ROM tests the memory on the way up and DSErrCode is written
over within a fraction of a second.

Nothing here asks which System is running. System 6.0.8 and System 7.0 were
both watched shutting down, with different words on the alert and the same
call behind it.
*/

const (
	// dsErrCodeAddress is the low memory word SysError keeps its code in
	dsErrCodeAddress = 0x0af0

	// shutDownAlert is the code of the alert that says the power can go
	shutDownAlert = 42
)

// watchSwitchOff looks at the last system error once a frame
func (m *Mac) watchSwitchOff() {
	code := uint16(m.mm.Peek(dsErrCodeAddress))<<8 | uint16(m.mm.Peek(dsErrCodeAddress+1))
	m.readyToSwitchOff.Store(code == shutDownAlert)
}

/*
IsReadyToSwitchOff tells whether the machine has been shut down and is waiting
for the power to be switched off. It is safe to call from a frontend while the
machine runs.
*/
func (m *Mac) IsReadyToSwitchOff() bool {
	return m.readyToSwitchOff.Load()
}
