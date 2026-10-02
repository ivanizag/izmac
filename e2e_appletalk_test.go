package izmac

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac/afp"
	"github.com/ivanizag/izmac/component"
	"github.com/ivanizag/izmac/localtalk"
)

/*
AppleTalk end to end: the ROM's own .MPP driving the emulated SCC in SDLC mode.
There is nothing on the network here but a recorder, and what the tests look
at is what the driver does on its own: the frames it sends to take a node
address, and where it keeps that address in low memory.
*/

const (
	// portBUse says what has the printer port; its low nibble is 1 once
	// AppleTalk does
	portBUseAddress  = 0x0291
	portUseAppleTalk = 0x01

	// abusVarsAddress points at the driver's variables, its node address
	// first
	abusVarsAddress = 0x02d8
)

// addressTaker is a network on which the first address the machine probes is
// already somebody else's, who answers its lapENQ with a lapACK
type addressTaker struct {
	networkRecorder
	port  *localTalkPort
	taken uint8
}

func (n *addressTaker) send(frame []uint8) {
	n.networkRecorder.send(frame)
	if frame[2] != lapEnq || n.port == nil {
		return
	}
	if n.taken == 0 {
		n.taken = frame[0]
	}
	if frame[0] == n.taken {
		n.port.deliver([]uint8{frame[1], frame[0], lapAck})
	}
}

// appleTalkMac is the e2e machine with AppleTalk on and a recorder on the
// network
func appleTalkMac(t *testing.T, disk string, ramKb int) (*Mac, *networkRecorder) {
	t.Helper()

	config := realConfig(t)
	if _, err := os.Stat(disk); err != nil {
		t.Skipf("%v is not here, this test needs it", disk)
	}
	config.DiskFiles = []string{disk}
	config.RamSizeKb = ramKb
	config.AppleTalk = appleTalkLocal
	config.PrinterPort = ""
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}

	m, err := NewMac(config)
	if err != nil {
		t.Fatal(err)
	}
	recorder := &networkRecorder{}
	m.localTalk.network = recorder
	return m, recorder
}

// nodeAddress is the node the driver took, or zero while it has none
func nodeAddress(m *Mac) uint8 {
	vars := uint32(m.mm.Peek(abusVarsAddress+1))<<16 |
		uint32(m.mm.Peek(abusVarsAddress+2))<<8 | uint32(m.mm.Peek(abusVarsAddress+3))
	if vars == 0 || vars >= 0x400000 || m.mm.Peek(portBUseAddress)&0x0f != portUseAppleTalk {
		return 0
	}
	return m.mm.Peek(vars)
}

// openChooser opens the Chooser from the Apple menu of System 6, which is what
// opens AppleTalk there
func openChooser(t *testing.T, m *Mac) {
	t.Helper()
	moveMouseTo(t, m, 16, 10)
	m.SetMouseButton(true)
	m.RunFrames(30)
	moveMouseTo(t, m, 60, 91)
	m.RunFrames(10)
	m.SetMouseButton(false)
	m.RunFrames(600)
}

// probes counts the lapENQ the machine sent for each address
func probes(frames [][]uint8) map[uint8]int {
	counts := make(map[uint8]int)
	for _, f := range frames {
		if f[2] == lapEnq {
			counts[f[0]]++
		}
	}
	return counts
}

func TestTheChooserOpensAppleTalkOnSystemSix(t *testing.T) {
	m, recorder := appleTalkMac(t, "frontend/macebiten/HD20SC.vhd", 1024)
	m.RunFrames(bootFrames)

	if nodeAddress(m) != 0 {
		t.Fatalf("AppleTalk is open before anything asked for it")
	}
	openChooser(t, m)

	node := nodeAddress(m)
	if node == 0 || node > 127 {
		t.Fatalf("the driver has node %v after the Chooser opened, wanted one from 1 to 127", node)
	}
	if probes(recorder.frames)[node] == 0 {
		t.Errorf("the driver took node %v without probing it", node)
	}

	// And then it asks the network for a router, with a DDP broadcast
	broadcast := false
	for _, f := range recorder.frames {
		if f[0] == lapBroadcast && f[2] == 0x01 {
			broadcast = true
		}
	}
	if !broadcast {
		t.Errorf("no DDP broadcast went out after the address was taken")
	}
}

/*
A node already using the address answers the probe with a lapACK, which has to
come back in through the receiver, its interrupt and the driver's own polling,
and the driver gives that address up for another
*/
func TestATakenAddressIsGivenUp(t *testing.T) {
	m, _ := appleTalkMac(t, "frontend/macebiten/HD20SC.vhd", 1024)
	taker := &addressTaker{port: m.localTalk}
	m.localTalk.network = taker
	m.RunFrames(bootFrames)
	openChooser(t, m)

	node := nodeAddress(m)
	if taker.taken == 0 || node == 0 {
		t.Fatalf("the driver never probed, or never took a node: probed %v, took %v", taker.taken, node)
	}
	if node == taker.taken {
		t.Errorf("the driver kept node %v after it was answered as taken", node)
	}
	if probes(taker.frames)[taker.taken] != 1 {
		t.Errorf("the taken address was probed %v times, wanted the one answered",
			probes(taker.frames)[taker.taken])
	}
}

/*
System 7 opens AppleTalk as it starts. It keeps the connection to use in the
extended parameter RAM, and with none to read it stops on a dialog saying the
driver could not be found, before AppleTalk opens; with the extended parameter
RAM there, the ROM sets it up on the first start and System 7 goes on.
*/
func TestSystemSevenOpensAppleTalkAsItStarts(t *testing.T) {
	m, recorder := appleTalkMac(t, systemSevenDisk, 4096)
	m.RunFrames(systemSevenBootFrames)

	node := nodeAddress(m)
	if node == 0 {
		t.Fatalf("System 7 did not open AppleTalk as it started")
	}
	if probes(recorder.frames)[node] == 0 {
		t.Errorf("System 7 took node %v without probing it", node)
	}
}

// pramWithNodeHint writes a parameter RAM file whose AppleTalk node hint, the
// address the driver tries first, is the one given
func pramWithNodeHint(t *testing.T, hint uint8) string {
	t.Helper()
	r := component.NewAppleRTC("", false)
	image := r.Image()
	const spATalkB = 0x12 // the classic byte 2, in the extended parameter RAM
	image[spATalkB] = hint

	file := filepath.Join(t.TempDir(), "pram.bin")
	if err := os.WriteFile(file, image, 0o600); err != nil {
		t.Fatal(err)
	}
	return file
}

// networkedMac is the e2e machine on a copy of the System 6 disk, on the
// network given
func networkedMac(t *testing.T, network *localtalk.Network, hint uint8) *Mac {
	t.Helper()
	config := realConfig(t)
	config.DiskFiles = []string{copyFile(t, config.DiskFiles[0])}
	config.AppleTalk = appleTalkLocal
	config.PrinterPort = ""
	config.PramFile = pramWithNodeHint(t, hint)
	config.localTalkNetwork = network
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	m, err := NewMac(config)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// runBoth runs two machines side by side, a frame at a time, so that each
// answers the other while it waits
func runBoth(a *Mac, b *Mac, frames int) {
	for i := 0; i < frames; i++ {
		a.RunFrames(1)
		b.RunFrames(1)
	}
}

/*
Two machines on one network, both told by their parameter RAM to try the same
node first. The first takes it. The second probes it, and it is the first
machine's own ROM that answers with the lapACK, across the network, which
makes the second take another: two Macintoshes talking to each other.
*/
func TestTwoMachinesShareTheNetwork(t *testing.T) {
	const hint = 0x33
	network := localtalk.NewNetwork()
	a := networkedMac(t, network, hint)
	b := networkedMac(t, network, hint)

	runBoth(a, b, int(bootFrames))
	openChooser(t, a)
	if node := nodeAddress(a); node != hint {
		t.Fatalf("the first machine took node %v, wanted the hint %v", node, hint)
	}

	// The second opens its Chooser with the first one running beside it
	moveMouseTo(t, b, 16, 10)
	b.SetMouseButton(true)
	b.RunFrames(30)
	moveMouseTo(t, b, 60, 91)
	b.RunFrames(10)
	b.SetMouseButton(false)
	runBoth(a, b, 600)

	node := nodeAddress(b)
	if node == 0 {
		t.Fatalf("the second machine took no node")
	}
	if node == hint {
		t.Errorf("both machines took node %v: the first did not answer the second's probe", hint)
	}
}

// fileServerMac is System 6.0.8 from the Utilities 1 diskette, the one with
// AppleShare in its System Folder, sharing a folder
func fileServerMac(t *testing.T, share string) *Mac {
	t.Helper()
	const utilities = "izmac_sys608 - Utilities 1.dsk"
	if _, err := os.Stat(utilities); err != nil {
		t.Skipf("%v is not here, this test needs it", utilities)
	}
	config := realConfig(t)
	config.DiskFiles = nil
	config.Diskettes = []string{copyFile(t, utilities)}
	config.RamSizeKb = 4096
	config.Share = share
	config.PrinterPort = ""
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	m, err := NewMac(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.fileServer.Stop)
	m.RunFrames(3000)
	return m
}

// chooseFileServer goes to the server in the Chooser, as far as the dialog
// asking how to log in
func chooseFileServer(t *testing.T, m *Mac) {
	t.Helper()
	moveMouseTo(t, m, 16, 10)
	m.SetMouseButton(true)
	m.RunFrames(30)
	moveMouseTo(t, m, 50, 59)
	m.RunFrames(10)
	m.SetMouseButton(false)
	m.RunFrames(1800)
	moveMouseTo(t, m, 100, 85)
	clickMouse(m)
	m.RunFrames(1200)

	// The server, the one in the list, and OK
	moveMouseTo(t, m, 300, 88)
	clickMouse(m)
	m.RunFrames(60)
	moveMouseTo(t, m, 330, 177)
	clickMouse(m)
	m.RunFrames(900)
}

// logInAsGuest presses OK in the dialog asking how to log in, as a guest,
// which lists the volumes
func logInAsGuest(t *testing.T, m *Mac) {
	t.Helper()
	moveMouseTo(t, m, 390, 258)
	clickMouse(m)
	m.RunFrames(900)
}

/*
The file server, from a Macintosh. The Chooser finds the server by NBP, gets
its status by ASP to ask how to log in, and logs in as a guest, which opens a
session and asks the server for its volumes; Quit in that dialog closes the
session again.
*/
func TestTheChooserLogsInToTheFileServer(t *testing.T) {
	m := fileServerMac(t, t.TempDir())
	chooseFileServer(t, m)
	if n := m.fileServer.Sessions(); n != 0 {
		t.Fatalf("%v sessions are open before logging in", n)
	}

	logInAsGuest(t, m)
	if n := m.fileServer.Sessions(); n != 1 {
		t.Fatalf("%v sessions are open after logging in, wanted one", n)
	}

	// Quit leaves the server
	moveMouseTo(t, m, 166, 258)
	clickMouse(m)
	m.RunFrames(600)
	if n := m.fileServer.Sessions(); n != 0 {
		t.Errorf("%v sessions are still open after quitting", n)
	}
}

/*
The shared folder as a volume of the Finder: mounted from the Chooser, opened,
and a file in it duplicated, which the Finder does by making a file and
copying both forks and the Finder information into it, all through the
server, and all ending up on the host
*/
func TestTheFinderDuplicatesAFileOnTheSharedFolder(t *testing.T) {
	share := t.TempDir()
	const text = "hello from the host\n"
	if err := os.WriteFile(filepath.Join(share, "readme.txt"), []uint8(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(share, "Folder"), 0o755); err != nil {
		t.Fatal(err)
	}

	m := fileServerMac(t, share)
	chooseFileServer(t, m)
	logInAsGuest(t, m)

	// The volume, OK, and the Chooser closed
	moveMouseTo(t, m, 200, 111)
	clickMouse(m)
	m.RunFrames(60)
	moveMouseTo(t, m, 336, 258)
	clickMouse(m)
	m.RunFrames(1200)
	moveMouseTo(t, m, 71, 47)
	clickMouse(m)
	m.RunFrames(1200)

	// The volume opened, under the disk on the desktop
	moveMouseTo(t, m, 472, 104)
	clickMouse(m)
	m.RunFrames(8)
	clickMouse(m)
	m.RunFrames(1500)

	// The file selected, and Duplicate from the File menu
	moveMouseTo(t, m, 173, 110)
	clickMouse(m)
	m.RunFrames(60)
	moveMouseTo(t, m, 55, 10)
	m.SetMouseButton(true)
	m.RunFrames(30)
	moveMouseTo(t, m, 78, 139)
	m.RunFrames(10)
	m.SetMouseButton(false)
	m.RunFrames(1500)

	copied := filepath.Join(share, "Copy of readme.txt")
	data, err := os.ReadFile(copied)
	if err != nil || string(data) != text {
		t.Fatalf("the copy has %q, %v", data, err)
	}
	finder, _ := afp.FinderInfo(copied)
	if string(finder[0:8]) != "TEXTttxt" {
		t.Errorf("the copy's Finder information is %q", finder[0:8])
	}
}
