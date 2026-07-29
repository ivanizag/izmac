package izmac

import (
	"testing"

	"github.com/ivanizag/izmac/component"
)

const sccReadBase = 0x9f_fff8

func sccAddress(offset uint32) uint32 {
	return sccReadBase + offset
}

/*
The chip reached the way the machine reaches it. What is checked here is the
wiring and not the chip: that the four ports land on the right channel and
side, and that the two axes of the mouse arrive on the channels the ROM reads
them from.
*/
func TestTheFourPortsAreDecoded(t *testing.T) {
	for _, c := range []struct {
		offset  uint32
		channel int
		control bool
	}{
		{sccOffsetBControl, component.ChannelB, true},
		{sccOffsetAControl, component.ChannelA, true},
		{sccOffsetBData, component.ChannelB, false},
		{sccOffsetAData, component.ChannelA, false},
	} {
		channel, control := sccPort(sccAddress(c.offset))
		if channel != c.channel || control != c.control {
			t.Errorf("the offset %v reached the channel %v control %v, wanted %v and %v",
				c.offset, channel, control, c.channel, c.control)
		}
	}
}

func TestTheSccIsReachableThroughTheMemoryManager(t *testing.T) {
	mm := newTestMemoryManager(1024)
	s := newScc()
	mm.scc = s
	mm.setOverlay(false)

	// Point at the register 1 and enable the external status interrupt
	mm.Poke(sccAddress(sccOffsetBControl), 1)
	mm.Poke(sccAddress(sccOffsetBControl), 1)

	// Reading the status back is the round trip through the map
	mm.Poke(sccAddress(sccOffsetBControl), 0)
	if mm.Peek(sccAddress(sccOffsetBControl)) != s.chip.Read(component.ChannelB, true) {
		t.Error("the SCC is not reachable through the memory manager")
	}
}

/*
The x axis of the mouse interrupts on the channel A and the y on the channel
B. Crossing them over is not something a test of the chip can catch, because
the chip has no idea which is which.
*/
func TestTheMouseReachesTheRightChannels(t *testing.T) {
	m := newMouse()
	s := newScc()

	/*
		The lines settle before the interrupts are enabled, which is the
		order the machine comes up in: the ROM programs the chip well
		after the first scan line has driven the pins, so the level it
		finds is already the idle one.
	*/
	x1, _, y1, _ := m.tick(true, true)
	s.setDcd(component.ChannelA, !x1)
	s.setDcd(component.ChannelB, !y1)

	// Both channels set up to interrupt on a carrier detect change
	for _, ch := range []int{component.ChannelA, component.ChannelB} {
		offset := uint32(sccOffsetAControl)
		if ch == component.ChannelB {
			offset = sccOffsetBControl
		}
		s.poke(sccAddress(offset), 1)
		s.poke(sccAddress(offset), 1)
		s.poke(sccAddress(offset), 0x08|7) // Point high, register 15
		s.poke(sccAddress(offset), 0x08)   // The carrier detect interrupt
	}

	m.move(4, 0)
	for i := 0; i < 8; i++ {
		m.quadratureRead()
		x1, _, y1, _ = m.tick(true, true)
		s.setDcd(component.ChannelA, !x1)
		s.setDcd(component.ChannelB, !y1)
	}

	if !s.pending(component.ChannelA) {
		t.Error("moving along x did not interrupt on the channel A")
	}
	if s.pending(component.ChannelB) {
		t.Error("moving along x interrupted on the channel B")
	}
}
