package izmac

import "github.com/ivanizag/izmac/component"

/*
The serial controller as the Macintosh wires it. The chip is its own thing in
component; what belongs here is where it sits on the address space and what
is plugged into it.

	sccRBase $9f_fff8   read      bCtl +0   aCtl +2   bData +4   aData +6
	sccWBase $bf_fff9   write

The two data carrier detect inputs are the mouse: the interrupt signal of the
x axis on the channel A and of the y axis on the channel B.
*/
type scc struct {
	chip component.SCC8530
}

const (
	// The offsets from the base of each of the two ports
	sccOffsetBControl = 0
	sccOffsetAControl = 2
	sccOffsetBData    = 4
	sccOffsetAData    = 6
)

func newScc() *scc {
	return &scc{}
}

// sccPort works out which channel and whether it is the control or the data
// address, from the three low bits of the address
func sccPort(address uint32) (channel int, control bool) {
	switch address & 0x06 {
	case sccOffsetBControl:
		return component.ChannelB, true
	case sccOffsetAControl:
		return component.ChannelA, true
	case sccOffsetBData:
		return component.ChannelB, false
	default:
		return component.ChannelA, false
	}
}

func (s *scc) peek(address uint32) uint8 {
	channel, control := sccPort(address)
	return s.chip.Read(channel, control)
}

func (s *scc) poke(address uint32, value uint8) {
	channel, control := sccPort(address)
	s.chip.Write(channel, control, value)
}

func (s *scc) setDcd(channel int, level bool) { s.chip.SetDcd(channel, level) }
func (s *scc) pending(channel int) bool       { return s.chip.Pending(channel) }
func (s *scc) interruptAsserted() bool        { return s.chip.InterruptAsserted() }
func (s *scc) reset()                         { s.chip.Reset() }
