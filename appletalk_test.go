package izmac

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ivanizag/izmac/component"
)

// networkRecorder is a LocalTalk network that keeps what the machine sends
type networkRecorder struct {
	frames [][]uint8
}

func (n *networkRecorder) send(frame []uint8) {
	n.frames = append(n.frames, append([]uint8(nil), frame...))
}

/*
localTalkTestPort is the printer port in SDLC mode, listening for node 9 as the
.MPP driver sets it up, with a recorder on the network side
*/
func localTalkTestPort() (*localTalkPort, *component.SCC8530, *networkRecorder) {
	scc := component.NewSCC8530(0)
	for _, rw := range [][2]uint8{{4, 0x20}, {6, 9}, {3, 0xdd}} {
		scc.Write(component.ChannelB, true, rw[0])
		scc.Write(component.ChannelB, true, rw[1])
	}
	port := newLocalTalkPort(scc)
	recorder := &networkRecorder{}
	port.network = recorder
	return port, scc, recorder
}

// received reads what reached the FIFO of the chip, the CRC included
func received(scc *component.SCC8530) []uint8 {
	var out []uint8
	for i := 0; i < 20; i++ {
		scc.Tick(1)
		for scc.Read(component.ChannelB, true)&1 != 0 {
			out = append(out, scc.Read(component.ChannelB, false))
		}
	}
	return out
}

/*
The handshake never leaves the machine: a directed lapRTS is answered at once
with the lapCTS the other node would have sent, from that node to this one,
and neither goes on the network
*/
func TestARequestToSendIsAnsweredAtHome(t *testing.T) {
	port, scc, recorder := localTalkTestPort()

	port.SendFrame([]uint8{0x20, 9, lapRts})

	if len(recorder.frames) != 0 {
		t.Errorf("the lapRTS went on the network: %x", recorder.frames)
	}
	got := received(scc)
	if len(got) < 3 || !bytes.Equal(got[:3], []uint8{9, 0x20, lapCts}) {
		t.Errorf("the machine got %x back, wanted the lapCTS from node $20", got)
	}
}

func TestABroadcastRequestToSendIsNotAnswered(t *testing.T) {
	port, scc, recorder := localTalkTestPort()

	port.SendFrame([]uint8{lapBroadcast, 9, lapRts})

	if len(recorder.frames) != 0 || len(received(scc)) != 0 {
		t.Errorf("a broadcast lapRTS was answered or sent on")
	}
}

func TestEverythingElseGoesToTheNetwork(t *testing.T) {
	port, _, recorder := localTalkTestPort()

	frames := [][]uint8{
		{9, 9, lapEnq},
		{lapBroadcast, 9, 0x01, 0x00, 0x06, 0x01, 0x01, 0x05, 0x01},
		{0x20, 9, 0x01, 0x00, 0x05, 0x02, 0x02, 0x04},
	}
	for _, f := range frames {
		port.SendFrame(f)
	}
	port.SendFrame([]uint8{0x20, 9, lapCts})

	if len(recorder.frames) != len(frames) {
		t.Fatalf("the network got %v frames, wanted %v and no lapCTS", len(recorder.frames), len(frames))
	}
	for i := range frames {
		if !bytes.Equal(recorder.frames[i], frames[i]) {
			t.Errorf("frame %v went out as %x, wanted %x", i, recorder.frames[i], frames[i])
		}
	}
}

// A frame from the network comes in on another goroutine and waits for the
// run loop to put it on the wire
func TestAFrameFromTheNetworkReachesTheWireOnPoll(t *testing.T) {
	port, scc, _ := localTalkTestPort()

	done := make(chan struct{})
	go func() {
		port.deliver([]uint8{9, 0x30, lapAck})
		close(done)
	}()
	<-done

	if len(received(scc)) != 0 {
		t.Fatalf("the frame reached the wire before the run loop took it")
	}
	port.poll()
	if got := received(scc); len(got) < 3 || !bytes.Equal(got[:3], []uint8{9, 0x30, lapAck}) {
		t.Errorf("the machine got %x, wanted the lapACK", got)
	}
}

// countRts counts the lapRTS frames in what reached the FIFO, each followed by
// its two bytes of CRC
func countRts(bytes []uint8) int {
	n := 0
	for i := 0; i+2 < len(bytes); i++ {
		if bytes[i] == 9 && bytes[i+1] == 0x20 && bytes[i+2] == lapRts {
			n++
		}
	}
	return n
}

/*
A frame for the machine is offered first, with the lapRTS of the station that
sent it, and only goes on the wire once the machine answers with its lapCTS:
a machine still busy with the last frame is not given the next one
*/
func TestAFrameForTheMachineWaitsForItsCts(t *testing.T) {
	port, scc, recorder := localTalkTestPort()
	frame := []uint8{9, 0x20, 0x01, 0x00, 0x05, 0x02, 0x02, 0x04}

	port.deliver(frame)
	port.poll()
	if got := received(scc); len(got) < 3 || !bytes.Equal(got[:3], []uint8{9, 0x20, lapRts}) {
		t.Fatalf("the machine got %x, wanted the lapRTS of node $20", got)
	}

	port.SendFrame([]uint8{0x20, 9, lapCts})
	if got := received(scc); len(got) < len(frame) || !bytes.Equal(got[:len(frame)], frame) {
		t.Errorf("after its lapCTS the machine got %x, wanted the frame", got)
	}
	if len(recorder.frames) != 0 {
		t.Errorf("the lapCTS went on the network: %x", recorder.frames)
	}
}

/*
A lapRTS that is not answered is sent again, as a station whose lapRTS met no
lapCTS backs off and tries again; but only once the last one has gone by, so
that lapRTS frames do not pile up for a machine to answer later
*/
func TestAnUnansweredOfferIsMadeAgain(t *testing.T) {
	port, scc, _ := localTalkTestPort()
	port.deliver([]uint8{9, 0x20, 0x01, 0x00, 0x05, 0x02, 0x02, 0x04})

	// The receiver off, as while the machine transmits: the lapRTS waits
	writeSccRegister(scc, 3, 0xd0)
	for i := 0; i < 3*offerLines; i++ {
		port.poll()
	}
	writeSccRegister(scc, 3, 0xdd)
	if n := countRts(received(scc)); n != 1 {
		t.Fatalf("%v lapRTS reached the machine for one frame waiting to go, wanted one", n)
	}

	// It went by unanswered, so a while later there is another
	for i := 0; i < offerLines; i++ {
		port.poll()
	}
	if n := countRts(received(scc)); n != 1 {
		t.Errorf("an unanswered lapRTS was followed by %v more, wanted one", n)
	}
}

// What is for every station goes on the wire as it comes, with no handshake
func TestABroadcastIsNotOffered(t *testing.T) {
	port, scc, _ := localTalkTestPort()
	frame := []uint8{lapBroadcast, 0x20, 0x01, 0x00, 0x05, 0x02, 0x02, 0x04}

	port.deliver(frame)
	port.poll()
	if got := received(scc); len(got) < len(frame) || !bytes.Equal(got[:len(frame)], frame) {
		t.Errorf("the machine got %x, wanted the broadcast as it is", got)
	}
}

// writeSccRegister sets a register of channel B
func writeSccRegister(scc *component.SCC8530, register uint8, value uint8) {
	scc.Write(component.ChannelB, true, register)
	scc.Write(component.ChannelB, true, value)
}

func TestAppleTalkMovesThePrinterToTheModemPort(t *testing.T) {
	c := NewConfiguration()
	if err := c.ParseFlags("izmac", []string{"-rom", "rom.bin", "-appletalk", "local"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if c.PrinterPort != printerPortModem {
		t.Errorf("with AppleTalk on the printer is on the %v port, wanted the modem one", c.PrinterPort)
	}

	off := NewConfiguration()
	if err := off.ParseFlags("izmac", []string{"-rom", "rom.bin"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if off.PrinterPort != printerPortPrinter {
		t.Errorf("with AppleTalk off the printer is on the %v port", off.PrinterPort)
	}
}

func TestAPrinterCanNotShareThePortWithAppleTalk(t *testing.T) {
	c := NewConfiguration()
	err := c.ParseFlags("izmac",
		[]string{"-rom", "rom.bin", "-appletalk", "local", "-printerport", "printer"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "modem port") {
		t.Errorf("a printer on the AppleTalk port gave %v", err)
	}

	// With no printer there is nothing in the way
	none := NewConfiguration()
	err = none.ParseFlags("izmac", []string{"-rom", "rom.bin", "-appletalk", "local",
		"-printer", "none", "-printerport", "printer"}, io.Discard)
	if err != nil {
		t.Errorf("AppleTalk with no printer was refused: %v", err)
	}
}

func TestAnUnknownAppleTalkNetworkIsRefused(t *testing.T) {
	c := NewConfiguration()
	if err := c.ParseFlags("izmac", []string{"-rom", "rom.bin", "-appletalk", "ethernet"}, io.Discard); err == nil {
		t.Errorf("an unknown AppleTalk network was accepted")
	}
}

func TestSharingAFolderTurnsAppleTalkOn(t *testing.T) {
	c := NewConfiguration()
	if err := c.ParseFlags("izmac", []string{"-rom", "rom.bin", "-share", t.TempDir()}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if c.AppleTalk != appleTalkLocal {
		t.Errorf("sharing a folder left AppleTalk as %q", c.AppleTalk)
	}

	// On the network it was asked for, if one was
	udp := NewConfiguration()
	if err := udp.ParseFlags("izmac", []string{"-rom", "rom.bin", "-share", t.TempDir(),
		"-appletalk", "udp"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if udp.AppleTalk != appleTalkUDP {
		t.Errorf("sharing a folder changed AppleTalk to %q", udp.AppleTalk)
	}
}

func TestOnlyAFolderCanBeShared(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, share := range []string{file, filepath.Join(t.TempDir(), "missing")} {
		c := NewConfiguration()
		if err := c.ParseFlags("izmac", []string{"-rom", "rom.bin", "-share", share}, io.Discard); err == nil {
			t.Errorf("sharing %v was accepted", share)
		}
	}
}
