package e2e_tests

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ivanizag/izmac"
	"github.com/ivanizag/izmac/afp"
	"github.com/ivanizag/izmac/component"
	"github.com/ivanizag/izmac/localtalk"
)

/*
AppleTalk end to end: the ROM's own .MPP driving the emulated SCC in SDLC mode.
The machine's network is one of the test's own, and on it, beside the
machine, a station of the test's that keeps every frame the machine sends:
what the tests look at is what the driver does on its own, the frames it
sends to take a node address, and where it keeps that address in low memory.
*/

const (
	// portBUse says what has the printer port; its low nibble is 1 once
	// AppleTalk does
	portBUseAddress  = 0x0291
	portUseAppleTalk = 0x01

	// abusVarsAddress points at the driver's variables, its node address
	// first
	abusVarsAddress = 0x02d8

	// The LLAP types and node looked at: the probe for an address, the
	// answer that it is taken, and the node every station listens to
	lapEnq       = 0x81
	lapAck       = 0x82
	lapBroadcast = 0xff
)

// networkRecorder is a station that keeps a copy of every frame it hears
type networkRecorder struct {
	mutex  sync.Mutex
	frames [][]uint8
}

func (n *networkRecorder) Receive(frame []uint8) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.frames = append(n.frames, frame)
}

// heard is what the station has heard so far
func (n *networkRecorder) heard() [][]uint8 {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	return append([][]uint8(nil), n.frames...)
}

/*
addressTaker is a station already using the first address the machine probes:
it answers the lapENQ for it with a lapACK, from that address, as the station
that has it does
*/
type addressTaker struct {
	networkRecorder
	network *localtalk.Network
	taken   uint8
}

func (n *addressTaker) Receive(frame []uint8) {
	n.networkRecorder.Receive(frame)
	if frame[2] != lapEnq {
		return
	}
	n.mutex.Lock()
	if n.taken == 0 {
		n.taken = frame[0]
	}
	taken := n.taken
	n.mutex.Unlock()
	if frame[0] == taken {
		n.network.Send(n, []uint8{frame[1], frame[0], lapAck})
	}
}

// appleTalkMac is the e2e machine on a copy of a test disk, with AppleTalk on
// a network of the test's own, and that network
func appleTalkMac(t *testing.T, disk string, ramKb int) (*izmac.Mac, *localtalk.Network) {
	t.Helper()

	network := localtalk.NewNetwork()
	config := realConfig(t)
	config.DiskFiles = []string{testImage(t, disk)}
	config.RamSizeKb = ramKb
	config.AppleTalk = izmac.AppleTalkLocal
	config.PrinterPort = ""
	config.LocalTalkNetwork = network
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}

	m, err := izmac.NewMac(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	return m, network
}

// nodeAddress is the node the driver took, or zero while it has none
func nodeAddress(m *izmac.Mac) uint8 {
	vars := uint32(m.Peek(abusVarsAddress+1))<<16 |
		uint32(m.Peek(abusVarsAddress+2))<<8 | uint32(m.Peek(abusVarsAddress+3))
	if vars == 0 || vars >= 0x400000 || m.Peek(portBUseAddress)&0x0f != portUseAppleTalk {
		return 0
	}
	return m.Peek(vars)
}

// openChooser opens the Chooser from the Apple menu of System 6, which is what
// opens AppleTalk there
func openChooser(t *testing.T, m *izmac.Mac) {
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
	t.Parallel()
	m, network := appleTalkMac(t, testSystemSixDisk, 1024)
	recorder := &networkRecorder{}
	network.Attach(recorder)
	m.RunFrames(bootFrames)

	if nodeAddress(m) != 0 {
		t.Fatalf("AppleTalk is open before anything asked for it")
	}
	openChooser(t, m)

	node := nodeAddress(m)
	if node == 0 || node > 127 {
		t.Fatalf("the driver has node %v after the Chooser opened, wanted one from 1 to 127", node)
	}
	if probes(recorder.heard())[node] == 0 {
		t.Errorf("the driver took node %v without probing it", node)
	}

	// And then it asks the network for a router, with a DDP broadcast
	broadcast := false
	for _, f := range recorder.heard() {
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
	t.Parallel()
	m, network := appleTalkMac(t, testSystemSixDisk, 1024)
	taker := &addressTaker{network: network}
	network.Attach(taker)
	m.RunFrames(bootFrames)
	openChooser(t, m)

	node := nodeAddress(m)
	if taker.taken == 0 || node == 0 {
		t.Fatalf("the driver never probed, or never took a node: probed %v, took %v", taker.taken, node)
	}
	if node == taker.taken {
		t.Errorf("the driver kept node %v after it was answered as taken", node)
	}
	if probes(taker.heard())[taker.taken] != 1 {
		t.Errorf("the taken address was probed %v times, wanted the one answered",
			probes(taker.heard())[taker.taken])
	}
}

/*
System 7 opens AppleTalk as it starts. It keeps the connection to use in the
extended parameter RAM, and with none to read it stops on a dialog saying the
driver could not be found, before AppleTalk opens; with the extended parameter
RAM there, the ROM sets it up on the first start and System 7 goes on.
*/
func TestSystemSevenOpensAppleTalkAsItStarts(t *testing.T) {
	t.Parallel()
	m, network := appleTalkMac(t, testSystemSevenDisk, 4096)
	recorder := &networkRecorder{}
	network.Attach(recorder)
	m.RunFrames(systemSevenBootFrames)

	node := nodeAddress(m)
	if node == 0 {
		t.Fatalf("System 7 did not open AppleTalk as it started")
	}
	if probes(recorder.heard())[node] == 0 {
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
func networkedMac(t *testing.T, network *localtalk.Network, hint uint8) *izmac.Mac {
	t.Helper()
	m, err := newNetworkedMac(t, network, hint, izmac.AppleTalkLocal)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// newNetworkedMac is the same on the AppleTalk network asked for, which can
// fail to be joined
func newNetworkedMac(t *testing.T, network *localtalk.Network, hint uint8, appleTalk string) (*izmac.Mac, error) {
	t.Helper()
	config := realConfig(t)
	config.AppleTalk = appleTalk
	config.PrinterPort = ""
	config.PramFile = pramWithNodeHint(t, hint)
	config.LocalTalkNetwork = network
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	m, err := izmac.NewMac(config)
	if err == nil {
		t.Cleanup(func() { m.Close() })
	}
	return m, err
}

// runBoth runs two machines side by side, a frame at a time, so that each
// answers the other while it waits
func runBoth(a *izmac.Mac, b *izmac.Mac, frames int) {
	for i := 0; i < frames; i++ {
		a.RunFrames(1)
		delivered(a, b)
		b.RunFrames(1)
		delivered(b, a)
	}
}

/*
delivered waits, when two machines are joined over UDP, for what one has sent
to reach the other, so that it is on the other's network before the other runs
its next frame, as on a wire. The machines run as fast as the host can go and
the sockets in the time of the host: without it, an answer the other machine
gives at once can come back after the one asking has given up waiting, more
so on a host busy with other tests. A datagram the host lost is waited for a
second, once.
*/
func delivered(from *izmac.Mac, to *izmac.Mac) {
	a, b := from.LocalTalkUDP(), to.LocalTalkUDP()
	if a != nil && b != nil {
		a.Deliver(b, time.Second)
	}
}

/*
Two machines on one network, both told by their parameter RAM to try the same
node first. The first takes it. The second probes it, and it is the first
machine's own ROM that answers with the lapACK, across the network, which
makes the second take another: two Macintoshes talking to each other.
*/
func TestTwoMachinesShareTheNetwork(t *testing.T) {
	t.Parallel()
	const hint = 0x33
	network := localtalk.NewNetwork()
	a := networkedMac(t, network, hint)
	b := networkedMac(t, network, hint)
	if node := secondNode(t, a, b, hint); node == hint {
		t.Errorf("both machines took node %v: the first did not answer the second's probe", hint)
	}
}

/*
The same, with each machine on a network of its own, as two izmacs started
with -appletalk host are: what joins them is LocalTalk over UDP on the
loopback interface, through the sockets of the host. A host that cannot join
multicast on its loopback skips it.

The answer to a probe is due at once, and through the sockets it comes in the
time of the host, while the machines run many times faster: runBoth waits for
what one machine sent to reach the other before the other runs on, as on a
wire, or the answer could come after the second machine has done probing.
*/
func TestTwoMachinesMeetOverTheLoopback(t *testing.T) {
	const hint = 0x34
	a, err := newNetworkedMac(t, localtalk.NewNetwork(), hint, izmac.AppleTalkHost)
	if err != nil {
		t.Skipf("LocalTalk over UDP is not available here: %v", err)
	}
	b, err := newNetworkedMac(t, localtalk.NewNetwork(), hint, izmac.AppleTalkHost)
	if err != nil {
		t.Skipf("LocalTalk over UDP is not available here: %v", err)
	}
	if node := secondNode(t, a, b, hint); node == hint {
		t.Errorf("both machines took node %v: the first never answered the second's probe", hint)
	}
}

/*
secondNode boots two machines told to try the same node first, opens the
Chooser of the first, which takes it, and then the one of the second, which
probes it and, answered by the first, takes another: the node it takes
*/
func secondNode(t *testing.T, a *izmac.Mac, b *izmac.Mac, hint uint8) uint8 {
	t.Helper()
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
	return node
}

// fileServerMac is System 6.0.8 from the test diskette, the one with
// AppleShare in its System Folder, sharing a folder
func fileServerMac(t *testing.T, share string) *izmac.Mac {
	t.Helper()
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testSystemSixDiskette)}
	config.RamSizeKb = 4096
	config.Share = share
	config.PrinterPort = ""
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	m, err := izmac.NewMac(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	m.RunFrames(3000)
	return m
}

// chooseFileServer goes to the server in the Chooser, as far as the dialog
// asking how to log in
func chooseFileServer(t *testing.T, m *izmac.Mac) {
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
func logInAsGuest(t *testing.T, m *izmac.Mac) {
	t.Helper()
	moveMouseTo(t, m, 390, 258)
	clickMouse(m)
	m.RunFrames(900)
}

/*
openSharedFolder mounts the shared folder on System 6, and opens its window:
the server and the guest in the Chooser, the volume, OK, the Chooser closed,
and the volume's icon, under the diskette's on the desktop, opened. A folder
with a file, readme.txt, and a folder, Folder, shows them at 178,125 and
119,125.
*/
func openSharedFolder(t *testing.T, m *izmac.Mac) {
	t.Helper()
	chooseFileServer(t, m)
	logInAsGuest(t, m)

	moveMouseTo(t, m, 200, 111)
	clickMouse(m)
	m.RunFrames(60)
	moveMouseTo(t, m, 336, 258)
	clickMouse(m)
	m.RunFrames(1200)
	moveMouseTo(t, m, 71, 47)
	clickMouse(m)
	m.RunFrames(1200)

	doubleClickAt(t, m, 472, 104)
	m.RunFrames(1500)
}

/*
The shared folder as a volume of the Finder: mounted from the Chooser, opened,
and a file in it duplicated, which the Finder does by making a file and
copying both forks and the Finder information into it, all through the
server, and all ending up on the host
*/
func TestTheFinderDuplicatesAFileOnTheSharedFolder(t *testing.T) {
	t.Parallel()
	share := t.TempDir()
	const text = "hello from the host\n"
	if err := os.WriteFile(filepath.Join(share, "readme.txt"), []uint8(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(share, "Folder"), 0o755); err != nil {
		t.Fatal(err)
	}

	m := fileServerMac(t, share)
	openSharedFolder(t, m)

	// The file selected, and Duplicate from the File menu
	moveMouseTo(t, m, 178, 125)
	clickMouse(m)
	m.RunFrames(60)
	moveMouseTo(t, m, 55, 10)
	m.SetMouseButton(true)
	m.RunFrames(30)
	moveMouseTo(t, m, 78, 139)
	m.RunFrames(10)
	m.SetMouseButton(false)

	copied := filepath.Join(share, "Copy of readme.txt")
	waitUntil(m, 180, func() bool {
		data, err := os.ReadFile(copied)
		return err == nil && string(data) == text && hasFinderInfo(copied)
	})
	data, err := os.ReadFile(copied)
	if err != nil || string(data) != text {
		t.Fatalf("the copy has %q, %v", data, err)
	}
	finder, _ := afp.FinderInfo(copied)
	if string(finder[0:8]) != "TEXTttxt" {
		t.Errorf("the copy's Finder information is %q", finder[0:8])
	}
}

// hasFinderInfo tells whether the server keeps Finder information for a file
func hasFinderInfo(host string) bool {
	_, ok := afp.FinderInfo(host)
	return ok
}
