package izmac

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac/afp"
	"github.com/ivanizag/izmac/localtalk"
)

/*
The file server of -share used the way it is used: documents edited in place,
files copied by the Finder of System 7, and the host changing the folder while
the machine has it open. Each of these found a bug the tests of the server on
its own did not.
*/

/*
TeachText opens a file of the shared folder, a word is typed at its start, and
it is saved and closed: on the host, the file has the word. That is the whole
of a document's life on the server, a fork opened to read and write, read,
written, its length set, and closed, from a real application.
*/
func TestTeachTextSavesToTheSharedFolder(t *testing.T) {
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

	// TeachText is on the diskette, and opens the text file as its own
	doubleClickAt(t, m, 178, 125)
	waitForApplication(t, m, "TeachText", 20)

	// Its window, with the document read over the network
	m.RunFrames(600)
	typeText(m, "edited ")
	pressCommand(m, "S")
	saved := func() bool {
		data, _ := os.ReadFile(filepath.Join(share, "readme.txt"))
		return string(data) != text
	}
	waitUntil(m, 120, saved)
	pressCommand(m, "Q")
	waitForApplication(t, m, "Finder", 60)

	data, err := os.ReadFile(filepath.Join(share, "readme.txt"))
	if err != nil || string(data) != "edited "+text {
		t.Errorf("the host has %q, %v, wanted the word typed and the text", data, err)
	}
}

/*
openSharedFolderOnSystemSeven mounts the shared folder on System 7 and opens
its window, as openSharedFolder does on System 6: the Chooser is elsewhere in
the Apple menu, and the client, AppleShare 3.5, is another one, with its
dialogs elsewhere too. A folder with a file in it shows it at 38,92.
*/
func openSharedFolderOnSystemSeven(t *testing.T, m *Mac) {
	t.Helper()

	// The Chooser, and AppleShare in it
	moveMouseTo(t, m, 16, 10)
	m.SetMouseButton(true)
	m.RunFrames(30)
	moveMouseTo(t, m, 60, 78)
	m.RunFrames(10)
	m.SetMouseButton(false)
	m.RunFrames(1800)
	moveMouseTo(t, m, 84, 75)
	clickMouse(m)
	m.RunFrames(1200)

	// The server, OK, as a guest, OK, the volume, OK, the Chooser closed
	for _, click := range []struct {
		h, v   int16
		frames uint64
	}{
		{330, 78, 60}, {364, 266, 900}, {390, 258, 900}, {336, 258, 1200}, {34, 32, 1200},
	} {
		moveMouseTo(t, m, click.h, click.v)
		clickMouse(m)
		m.RunFrames(click.frames)
	}

	// Its icon, under the disk's
	doubleClickAt(t, m, 472, 95)
	m.RunFrames(1500)
}

/*
writeAppleDouble gives a file of the host the Finder information and the
resource fork the server finds for it, in an AppleDouble file, version 2, with
the Finder information, entry 9, and the resource fork, entry 2
*/
func writeAppleDouble(t *testing.T, file string, finder string, resource []uint8) {
	t.Helper()
	const header = 26 + 2*12
	raw := make([]uint8, header, header+32+len(resource))
	binary.BigEndian.PutUint32(raw[0:], 0x00051607)
	binary.BigEndian.PutUint32(raw[4:], 0x00020000)
	binary.BigEndian.PutUint16(raw[24:], 2)
	binary.BigEndian.PutUint32(raw[26:], 9)
	binary.BigEndian.PutUint32(raw[30:], header)
	binary.BigEndian.PutUint32(raw[34:], 32)
	binary.BigEndian.PutUint32(raw[38:], 2)
	binary.BigEndian.PutUint32(raw[42:], header+32)
	binary.BigEndian.PutUint32(raw[46:], uint32(len(resource)))
	info := make([]uint8, 32)
	copy(info, finder)
	raw = append(append(raw, info...), resource...)

	sidecar := filepath.Join(filepath.Dir(file), "._"+filepath.Base(file))
	if err := os.WriteFile(sidecar, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

/*
The Finder of System 7 duplicates a file with both forks on the shared folder.
It copies through a buffer of 32K: each fork of the copy is set to that size,
written in full, and set back to the size it has, so a copy that keeps the
longest size it was ever given comes out the size of the buffer. It reads the
original in answers of eight packets, which a machine given them faster than
it takes them loses some of and asks for again seconds later: the copy is
waited for for a time a working one takes a fraction of.
*/
func TestSystemSevenDuplicatesAFileWithAResourceFork(t *testing.T) {
	share := t.TempDir()
	const data = "the data fork"
	resource := bytes.Repeat([]uint8("resource"), 2381)
	original := filepath.Join(share, "Forked")
	if err := os.WriteFile(original, []uint8(data), 0o644); err != nil {
		t.Fatal(err)
	}
	writeAppleDouble(t, original, "TEXTttxt", resource)

	config := testConfig(t)
	config.DiskFiles = []string{testImage(t, testSystemSevenDisk)}
	config.RamSizeKb = 4096
	config.Share = share
	config.PrinterPort = ""
	m := buildTestMac(t, config)
	t.Cleanup(m.fileServer.Stop)
	m.RunFrames(systemSevenBootFrames)
	openSharedFolderOnSystemSeven(t, m)

	moveMouseTo(t, m, 38, 92)
	clickMouse(m)
	m.RunFrames(30)
	pressCommand(m, "D")

	copied := filepath.Join(share, "Forked copy")
	forks := func() (string, []uint8) {
		got, _ := os.ReadFile(copied)
		return string(got), afp.ResourceFork(copied)
	}
	copiedWhole := func() bool {
		got, fork := forks()
		return got == data && bytes.Equal(fork, resource)
	}
	if !waitUntil(m, 180, copiedWhole) {
		got, fork := forks()
		t.Errorf("the copy has %v bytes of data and %v of resource fork, wanted %v and %v",
			len(got), len(fork), len(data), len(resource))
	}
}

/*
A file made on the host in a folder below the top of the shared one shows in
the machine's window of that folder. The Finder reads the folders of its
windows again when the volume's date moves, and the host changing a folder
has to move it, however deep the folder is.
*/
func TestAFileMadeOnTheHostShowsInTheMachinesWindow(t *testing.T) {
	share := t.TempDir()
	if err := os.WriteFile(filepath.Join(share, "readme.txt"), []uint8("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(share, "Folder"), 0o755); err != nil {
		t.Fatal(err)
	}

	m := fileServerMac(t, share)
	openSharedFolder(t, m)
	doubleClickAt(t, m, 119, 125)
	m.RunFrames(1500)

	// The window of the folder, empty, which is what is looked at
	window := func() []uint8 {
		buffer := m.video.frameBuffer()
		const top, bottom, bytesPerLine = 100, 240, 64
		return append([]uint8(nil), buffer[top*bytesPerLine:bottom*bytesPerLine]...)
	}
	before := window()

	if err := os.WriteFile(filepath.Join(share, "Folder", "inside.txt"), []uint8("made on the host\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !waitUntil(m, 90, func() bool { return !bytes.Equal(window(), before) }) {
		t.Errorf("after 90 seconds the window of the folder still looks as it did")
	}
}

/*
startFileSharing starts the File Sharing of System 7 on a machine at its
Finder, the way its owner would: the startup disk opened, then the System
Folder, the Control Panels and Sharing Setup, an owner, a password and a name
for the machine typed in, and Start
*/
func startFileSharing(t *testing.T, m *Mac, owner string, password string, name string) {
	t.Helper()

	pressCommand(m, "O")
	m.RunFrames(600)
	for _, at := range [][2]int16{{160, 92}, {358, 92}, {232, 92}} {
		doubleClickAt(t, m, at[0], at[1])
		m.RunFrames(900)
	}

	moveMouseTo(t, m, 300, 85)
	clickMouse(m)
	typeText(m, owner)
	pressKey(m, "Tab")
	typeText(m, password)
	pressKey(m, "Tab")
	typeText(m, name)

	// Start, and the minute or so a Plus takes to start sharing
	moveMouseTo(t, m, 148, 197)
	clickMouse(m)
	m.RunFrames(3600)
}

/*
Two machines on one LocalTalk, sharing a disk with nothing of izmac's own in
between but the wire: System 7 sharing its disk with its File Sharing, and
System 6 mounting it with AppleShare, logged in as the owner. It is the most
traffic two machines put on the wire, in both directions, and each of them
the other's only judge of whether a frame arrived when it should.

The machine that shares runs on a goroutine of its own while the other is
driven, the two going about as fast, as two machines on one network do.
*/
func TestTwoMachinesShareADiskWithFileSharing(t *testing.T) {
	network := localtalk.NewNetwork()

	config := testConfig(t)
	config.DiskFiles = []string{testImage(t, testSystemSevenDisk)}
	config.RamSizeKb = 4096
	config.AppleTalk = appleTalkLocal
	config.PrinterPort = ""
	config.localTalkNetwork = network
	server := buildTestMac(t, config)
	waitForApplication(t, server, "Finder", 120)
	server.RunFrames(600)
	startFileSharing(t, server, "owner", "pw", "server")

	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			select {
			case <-stop:
				return
			default:
				server.RunFrames(5)
			}
		}
	}()
	defer func() {
		close(stop)
		<-stopped
	}()

	config = testConfig(t)
	config.Diskettes = []string{testImage(t, testSystemSixDiskette)}
	config.RamSizeKb = 4096
	config.AppleTalk = appleTalkLocal
	config.PrinterPort = ""
	config.localTalkNetwork = network
	client := buildTestMac(t, config)
	waitForApplication(t, client, "Finder", 60)
	client.RunFrames(300)

	// The server in the Chooser, as its owner, and its disk
	chooseFileServer(t, client)
	typeText(client, "owner")
	pressKey(client, "Tab")
	typeText(client, "pw")
	logInAsGuest(t, client)
	moveMouseTo(t, client, 200, 111)
	clickMouse(client)
	client.RunFrames(60)
	moveMouseTo(t, client, 336, 258)
	clickMouse(client)
	waitUntil(client, 120, func() bool { return len(mountedVolumes(client)) == 2 })

	volumes := mountedVolumes(client)
	if len(volumes) != 2 || volumes[1] != "System 7 HD" {
		t.Errorf("the machine has %q mounted, wanted its diskette and the other's disk", volumes)
	}
}
