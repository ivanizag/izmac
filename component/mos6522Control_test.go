package component

import "testing"

const (
	regPortB   = 0
	regPortA   = 1
	regPcr     = 12
	regIfr     = 13
	regIer     = 14
	regPortANH = 15
)

func TestCA1RaisesItsFlagOnTheSelectedEdge(t *testing.T) {
	var v MOS6522

	// The positive edge
	v.Write(regPcr, mos6522PcrCA1PositiveEdge)
	v.SetCA1(false)
	v.Write(regIfr, mos6522IntCA1)

	v.SetCA1(true)
	if v.Read(regIfr)&mos6522IntCA1 == 0 {
		t.Error("the rising edge did not raise the CA1 flag")
	}

	// And the negative one
	v.Write(regPcr, 0)
	v.SetCA1(true)
	v.Write(regIfr, mos6522IntCA1)

	v.SetCA1(false)
	if v.Read(regIfr)&mos6522IntCA1 == 0 {
		t.Error("the falling edge did not raise the CA1 flag")
	}
}

func TestCA1IgnoresTheEdgeNotSelected(t *testing.T) {
	var v MOS6522

	v.Write(regPcr, mos6522PcrCA1PositiveEdge)
	v.SetCA1(true)
	v.Write(regIfr, mos6522IntCA1)

	v.SetCA1(false)
	if v.Read(regIfr)&mos6522IntCA1 != 0 {
		t.Error("the falling edge raised the flag with the rising one selected")
	}
}

func TestAStableLineRaisesNothing(t *testing.T) {
	var v MOS6522

	v.Write(regPcr, mos6522PcrCA1PositiveEdge)
	v.SetCA1(true)
	v.Write(regIfr, mos6522IntCA1)

	// Driving the same level again is not a transition
	v.SetCA1(true)
	v.SetCA1(true)
	if v.Read(regIfr)&mos6522IntCA1 != 0 {
		t.Error("a level that did not change raised the flag")
	}
}

/*
The port A is reachable through two registers. The register 1 clears the
flags of the control lines as a side effect, the register 15 does not, and
the Macintosh uses the register 15 exactly so that reading the port does not
lose a vertical blanking.
*/
func TestTheRegister15DoesNotClearTheControlFlags(t *testing.T) {
	var v MOS6522

	v.Write(regPcr, mos6522PcrCA1PositiveEdge)
	v.SetCA1(false)
	v.SetCA1(true)

	v.Read(regPortANH)
	if v.Read(regIfr)&mos6522IntCA1 == 0 {
		t.Error("reading the port A without handshake cleared the CA1 flag")
	}

	v.Read(regPortA)
	if v.Read(regIfr)&mos6522IntCA1 != 0 {
		t.Error("reading the port A through the register 1 did not clear the CA1 flag")
	}
}

func TestWritingThePortAlsoClearsTheFlags(t *testing.T) {
	var v MOS6522

	v.Write(regPcr, mos6522PcrCA1PositiveEdge)
	v.SetCA1(false)
	v.SetCA1(true)

	v.Write(regPortA, 0)
	if v.Read(regIfr)&mos6522IntCA1 != 0 {
		t.Error("writing the port A did not clear the CA1 flag")
	}
}

func TestThePortBClearsItsOwnFlags(t *testing.T) {
	var v MOS6522

	v.Write(regPcr, mos6522PcrCB1PositiveEdge)
	v.SetCB1(false)
	v.SetCB1(true)

	if v.Read(regIfr)&mos6522IntCB1 == 0 {
		t.Fatal("the CB1 flag was not raised")
	}

	v.Read(regPortB)
	if v.Read(regIfr)&mos6522IntCB1 != 0 {
		t.Error("reading the port B did not clear the CB1 flag")
	}
}

func TestCA1InterruptsOnlyWhenEnabled(t *testing.T) {
	var v MOS6522

	v.Write(regPcr, mos6522PcrCA1PositiveEdge)
	v.SetCA1(false)
	v.SetCA1(true)

	if v.InterruptAsserted() {
		t.Error("the flag asserted the interrupt line while it was not enabled")
	}

	v.Write(regIer, 0x80|mos6522IntCA1)
	if !v.InterruptAsserted() {
		t.Error("enabling the CA1 interrupt did not assert the line")
	}

	v.Write(regIfr, mos6522IntCA1)
	if v.InterruptAsserted() {
		t.Error("clearing the flag did not release the line")
	}
}

func TestCA2IsIgnoredWhileItIsAnOutput(t *testing.T) {
	var v MOS6522

	v.Write(regPcr, mos6522PcrCA2Output)
	v.SetCA2(false)
	v.SetCA2(true)

	if v.Read(regIfr)&mos6522IntCA2 != 0 {
		t.Error("a pin programmed as an output raised its input flag")
	}
}
