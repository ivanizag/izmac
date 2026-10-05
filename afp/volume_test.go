package afp

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

/*
The volume calls, as the AppleShare client sends them, on a folder made for
each test
*/

type testClient struct {
	t      *testing.T
	server *Server
	folder string
}

func newTestClient(t *testing.T) *testClient {
	folder := t.TempDir()
	s := NewServer("izmac", "Shared", folder, nil)
	s.OpenSession(1)
	if _, result := s.Command(1, loginRequest(version20, uamGuest)); result != errNoErr {
		t.Fatalf("login gave %v", result)
	}
	return &testClient{t: t, server: s, folder: folder}
}

// call sends a call made of its fields, each a byte, a uint16, a uint32 or
// bytes as they are
func (c *testClient) call(want int32, fields ...any) []uint8 {
	c.t.Helper()
	reply, result := c.server.Command(1, request(fields...))
	if result != want {
		c.t.Fatalf("call %v gave %v, wanted %v", fields[0], result, want)
	}
	return reply
}

func request(fields ...any) []uint8 {
	var out []uint8
	for _, f := range fields {
		switch v := f.(type) {
		case uint8:
			out = append(out, v)
		case int:
			out = append(out, uint8(v))
		case uint16:
			out = binary.BigEndian.AppendUint16(out, v)
		case uint32:
			out = binary.BigEndian.AppendUint32(out, v)
		case []uint8:
			out = append(out, v...)
		}
	}
	return out
}

// longPath is a path of long names, the names separated by nulls
func longPath(names ...string) []uint8 {
	p := []uint8{}
	for i, n := range names {
		if i > 0 {
			p = append(p, 0)
		}
		p = append(p, n...)
	}
	return append([]uint8{pathLong, uint8(len(p))}, p...)
}

func (c *testClient) write(name string, data string) {
	c.t.Helper()
	if err := os.WriteFile(filepath.Join(c.folder, name), []uint8(data), 0o644); err != nil {
		c.t.Fatal(err)
	}
}

func TestTheVolumeOpens(t *testing.T) {
	c := newTestClient(t)
	reply := c.call(errNoErr, fpOpenVol, 0, uint16(volID|volName|volSignature), []uint8{6}, []uint8("shared"))
	if binary.BigEndian.Uint16(reply[2:]) != signatureFixed || binary.BigEndian.Uint16(reply[4:]) != volumeID {
		t.Errorf("the volume parameters are %x", reply)
	}
	if name := pascalAt(t, reply[2:], int(binary.BigEndian.Uint16(reply[6:]))); name != "Shared" {
		t.Errorf("the volume is called %q", name)
	}
	c.call(errObjectNotFound, fpOpenVol, 0, uint16(volID), []uint8{5}, []uint8("Other"))
}

// enumerate lists a folder, and gives the names in it and whether each is a
// folder
func (c *testClient) enumerate(dir uint32, path []uint8) map[string]bool {
	c.t.Helper()
	found := make(map[string]bool)
	for start := 1; ; {
		reply, result := c.server.Command(1, request(fpEnumerate, 0, uint16(volumeID), dir,
			uint16(paramLongName), uint16(paramLongName), uint16(2), uint16(start), uint16(quantumSize), path))
		if result == errObjectNotFound {
			return found
		}
		if result != errNoErr {
			c.t.Fatalf("enumerate gave %v", result)
		}
		count := int(binary.BigEndian.Uint16(reply[4:]))
		at := 6
		for i := 0; i < count; i++ {
			length := int(reply[at])
			parms := reply[at+2 : at+length]
			found[pascalAt(c.t, parms, int(binary.BigEndian.Uint16(parms)))] = reply[at+1]&isDirectoryFlag != 0
			at += length
		}
		start += count
	}
}

func TestAFolderIsListed(t *testing.T) {
	c := newTestClient(t)
	c.write("one.txt", "1")
	c.write("two", "2")
	c.write(".hidden", "")
	c.write("._one.txt", "")
	os.Mkdir(filepath.Join(c.folder, "Folder"), 0o755)
	c.write("Folder/inside", "")
	c.write("a name much longer than thirty one characters.txt", "")

	found := c.enumerate(rootID, longPath())
	if len(found) != 4 || !found["Folder"] || found["one.txt"] || found["two"] {
		t.Errorf("the root lists %v", found)
	}
	for name := range found {
		if len(name) > longestName {
			t.Errorf("%q is too long for the machine", name)
		}
	}
	if inside := c.enumerate(rootID, longPath("folder")); len(inside) != 1 || inside["inside"] {
		t.Errorf("the folder lists %v", inside)
	}
}

func TestTheRootIsFoundByTheVolumeName(t *testing.T) {
	c := newTestClient(t)
	reply := c.call(errNoErr, fpGetFileDirParms, 0, uint16(volumeID), uint32(parentOfRootID),
		uint16(0), uint16(dirID|paramParentID), longPath("Shared"))
	if reply[4] != isDirectoryFlag || binary.BigEndian.Uint32(reply[6:]) != parentOfRootID ||
		binary.BigEndian.Uint32(reply[10:]) != rootID {
		t.Errorf("the root's parameters are %x", reply)
	}
}

// openFork opens a fork and gives its reference number
func (c *testClient) openFork(resource bool, access uint16, names ...string) uint16 {
	c.t.Helper()
	flag := 0
	if resource {
		flag = resourceForkFlag
	}
	reply := c.call(errNoErr, fpOpenFork, flag, uint16(volumeID), uint32(rootID), uint16(0), access, longPath(names...))
	return binary.BigEndian.Uint16(reply[2:])
}

func (c *testClient) writeFork(ref uint16, offset uint32, data string) {
	c.t.Helper()
	_, result := c.server.Write(1, request(fpWrite, 0, ref, offset, uint32(len(data))), []uint8(data))
	if result != errNoErr {
		c.t.Fatalf("the write gave %v", result)
	}
}

func TestAFileIsMadeWrittenAndRead(t *testing.T) {
	c := newTestClient(t)
	c.call(errNoErr, fpCreateFile, 0, uint16(volumeID), uint32(rootID), longPath("New/File"))
	c.call(errObjectExists, fpCreateFile, 0, uint16(volumeID), uint32(rootID), longPath("new/file"))

	ref := c.openFork(false, accessRead|accessWrite, "New/File")
	c.writeFork(ref, 0, "hello\rworld")
	reply, result := c.server.Command(1, request(fpRead, 0, ref, uint32(0), uint32(100), 0, 0))
	if result != errEOFErr || string(reply) != "hello\rworld" {
		t.Errorf("reading gave %q and %v", reply, result)
	}
	reply = c.call(errNoErr, fpRead, 0, ref, uint32(0), uint32(100), uint8(0xff), uint8('\r'))
	if string(reply) != "hello\r" {
		t.Errorf("reading a line gave %q", reply)
	}
	c.call(errNoErr, fpCloseFork, 0, ref)

	// The slash of the machine is a colon on the host
	data, err := os.ReadFile(filepath.Join(c.folder, "New:File"))
	if err != nil || string(data) != "hello\rworld" {
		t.Errorf("the host has %q, %v", data, err)
	}
}

func TestTheResourceForkAndFinderInfoAreKept(t *testing.T) {
	c := newTestClient(t)
	c.call(errNoErr, fpCreateFile, 0, uint16(volumeID), uint32(rootID), longPath("App"))

	ref := c.openFork(true, accessRead|accessWrite, "App")
	c.writeFork(ref, 0, "resources")
	c.call(errNoErr, fpCloseFork, 0, ref)

	finder := make([]uint8, 32)
	copy(finder, "APPLizmc")
	c.call(errNoErr, fpSetFileParms, 0, uint16(volumeID), uint32(rootID), uint16(paramFinderInfo),
		longPath("App"), uint8(0), finder)

	// A new server on the same folder finds them where the host keeps them
	s := NewServer("izmac", "Shared", c.folder, nil)
	s.OpenSession(1)
	s.Command(1, loginRequest(version20, uamGuest))
	reply, result := s.Command(1, request(fpGetFileDirParms, 0, uint16(volumeID), uint32(rootID),
		uint16(paramFinderInfo|fileResourceLength), uint16(0), longPath("App")))
	if result != errNoErr {
		t.Fatalf("FPGetFileDirParms gave %v", result)
	}
	if !bytes.Equal(reply[6:14], []uint8("APPLizmc")) || binary.BigEndian.Uint32(reply[38:]) != 9 {
		t.Errorf("the file has %x", reply[6:])
	}
}

func TestThingsAreRenamedMovedAndDeleted(t *testing.T) {
	c := newTestClient(t)
	reply := c.call(errNoErr, fpCreateDir, 0, uint16(volumeID), uint32(rootID), longPath("Folder"))
	folder := binary.BigEndian.Uint32(reply)
	c.write("file", "data")

	c.call(errNoErr, fpRename, 0, uint16(volumeID), uint32(rootID), longPath("file"), longPath("renamed"))
	c.call(errNoErr, fpMoveAndRename, 0, uint16(volumeID), uint32(rootID), folder,
		longPath("renamed"), longPath(), longPath("moved"))
	if found := c.enumerate(folder, longPath()); len(found) != 1 || found["moved"] {
		t.Errorf("the folder has %v", found)
	}

	// A folder cannot go into itself
	c.call(errCantMove, fpMoveAndRename, 0, uint16(volumeID), uint32(rootID), folder,
		longPath("Folder"), longPath(), longPath())

	c.call(errDirNotEmpty, fpDelete, 0, uint16(volumeID), uint32(rootID), longPath("Folder"))
	ref := c.openFork(false, accessRead, "Folder", "moved")
	c.call(errFileBusy, fpDelete, 0, uint16(volumeID), folder, longPath("moved"))
	c.call(errNoErr, fpCloseFork, 0, ref)
	c.call(errNoErr, fpDelete, 0, uint16(volumeID), folder, longPath("moved"))
	c.call(errNoErr, fpDelete, 0, uint16(volumeID), uint32(rootID), longPath("Folder"))
	if entries, _ := os.ReadDir(c.folder); len(entries) != 0 {
		t.Errorf("the folder still has %v things", len(entries))
	}
}

func TestDenyModesAreKept(t *testing.T) {
	c := newTestClient(t)
	c.write("file", "data")
	c.openFork(false, accessRead|accessWrite|accessDenyWrite, "file")
	c.call(errDenyConflict, fpOpenFork, 0, uint16(volumeID), uint32(rootID), uint16(0),
		uint16(accessWrite), longPath("file"))
}

func TestAPathGoesUpWithNulls(t *testing.T) {
	parts := splitPath([]uint8("a\x00b\x00\x00c"))
	if len(parts) != 4 || string(parts[0]) != "a" || string(parts[1]) != "b" || parts[2] != nil || string(parts[3]) != "c" {
		t.Errorf("the path splits as %q", parts)
	}
}

func TestBundlesGiveIcons(t *testing.T) {
	icn := bytes.Repeat([]uint8{0xaa}, 256)
	bundle := request([]uint8("izmc"), uint16(0), uint16(1),
		[]uint8("ICN#"), uint16(0), uint16(0), uint16(128),
		[]uint8("FREF"), uint16(0), uint16(0), uint16(129))
	fref := request([]uint8("APPL"), uint16(0), uint8(0))
	fork := resourceFork_(map[string]map[int16][]uint8{
		"BNDL": {128: bundle}, "FREF": {129: fref}, "ICN#": {128: icn},
	})
	icons := bundleIcons(parseResources(fork))
	if len(icons) != 1 || string(icons[0].creator[:]) != "izmc" || string(icons[0].fileType[:]) != "APPL" ||
		!bytes.Equal(icons[0].data, icn) {
		t.Errorf("the bundle gives %v icons", len(icons))
	}
}

// resourceFork_ builds a resource fork with the resources given
func resourceFork_(res map[string]map[int16][]uint8) []uint8 {
	var data []uint8
	type ref struct {
		id     int16
		offset int
	}
	refs := make(map[string][]ref)
	var types []string
	for t, byID := range res {
		types = append(types, t)
		for id, d := range byID {
			refs[t] = append(refs[t], ref{id, len(data)})
			data = binary.BigEndian.AppendUint32(data, uint32(len(d)))
			data = append(data, d...)
		}
	}

	typeList := request(uint16(len(types) - 1))
	refList := []uint8{}
	listStart := 2 + 8*len(types)
	for _, t := range types {
		typeList = append(typeList, t...)
		typeList = binary.BigEndian.AppendUint16(typeList, uint16(len(refs[t])-1))
		typeList = binary.BigEndian.AppendUint16(typeList, uint16(listStart+len(refList)))
		for _, r := range refs[t] {
			refList = request(refList, uint16(r.id), uint16(0xffff), uint32(r.offset), uint32(0))
		}
	}
	resourceMap := append(make([]uint8, 24), 0, 28, 0, 0)
	resourceMap = append(append(resourceMap, typeList...), refList...)

	header := request(uint32(256), uint32(256+len(data)), uint32(len(data)), uint32(len(resourceMap)))
	fork := append(header, make([]uint8, 256-len(header))...)
	return append(append(fork, data...), resourceMap...)
}

func TestAppleDoubleFilesKeepWhatTheHostCannot(t *testing.T) {
	folder := t.TempDir()
	file := filepath.Join(folder, "file")
	os.WriteFile(file, nil, 0o644)
	store := newMetadataStore(folder)

	var finder [32]uint8
	copy(finder[:], "TEXTttxt")
	if err := store.setFinderInfo(file, finder); err != nil {
		t.Fatal(err)
	}
	if err := store.setResource(file, []uint8("fork")); err != nil {
		t.Fatal(err)
	}
	got, ok := store.finderInfo(file)
	if !ok || got != finder || string(store.resource(file)) != "fork" || store.resourceLength(file) != 4 {
		t.Errorf("the AppleDouble file has %q and %q", got[:8], store.resource(file))
	}
	if _, err := os.Stat(filepath.Join(folder, "._file")); err != nil {
		t.Errorf("there is no ._file next to the file: %v", err)
	}

	store.renamed(file, file+"2")
	if _, err := os.Stat(filepath.Join(folder, "._file2")); err != nil {
		t.Errorf("the AppleDouble file did not follow the rename")
	}
	store.setFinderInfo(file+"2", [32]uint8{})
	store.setResource(file+"2", nil)
	if entries, _ := os.ReadDir(folder); len(entries) != 1 {
		t.Errorf("an empty AppleDouble file was kept")
	}
}

// The shared folder keeps its Finder information inside it, not next to it
func TestTheSharedFolderKeepsItsOwnInside(t *testing.T) {
	c := newTestClient(t)
	finder := make([]uint8, 32)
	copy(finder[10:], "window")
	c.call(errNoErr, fpSetDirParms, 0, uint16(volumeID), uint32(rootID), uint16(paramFinderInfo),
		longPath(), finder)

	if _, err := os.Stat(filepath.Join(c.folder, rootSidecar)); err != nil {
		t.Errorf("the shared folder has no %v inside: %v", rootSidecar, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(c.folder), "._"+filepath.Base(c.folder))); err == nil {
		t.Errorf("the shared folder's Finder information went outside it")
	}
	if found := c.enumerate(rootID, longPath()); len(found) != 0 {
		t.Errorf("the AppleDouble file of the folder is listed: %v", found)
	}
}

/*
The Finder of System 7 copies a resource fork by setting it to the size of its
buffer, flushing, writing, and setting it to the size it has. What the host
keeps is that last size, not the longest, which on macOS needs the attribute
written anew rather than into.
*/
func TestAResourceForkThatShrinksIsKeptShort(t *testing.T) {
	c := newTestClient(t)
	c.call(errNoErr, fpCreateFile, 0, uint16(volumeID), uint32(rootID), longPath("App"))

	ref := c.openFork(true, accessRead|accessWrite, "App")
	c.call(errNoErr, fpSetForkParms, 0, ref, uint16(fileResourceLength), uint32(32768))
	c.call(errNoErr, fpFlushFork, 0, ref)
	c.writeFork(ref, 0, "resources")
	c.call(errNoErr, fpSetForkParms, 0, ref, uint16(fileResourceLength), uint32(9))
	c.call(errNoErr, fpCloseFork, 0, ref)

	if n := newMetadataStore(c.folder).resourceLength(filepath.Join(c.folder, "App")); n != 9 {
		t.Errorf("the host keeps a resource fork of %v bytes, wanted 9", n)
	}
}

// volumeModified is the modification date the volume gives
func (c *testClient) volumeModified() time.Time {
	c.t.Helper()
	reply := c.call(errNoErr, fpGetVolParms, 0, uint16(volumeID), uint16(volModified))
	return fromAFPTime(binary.BigEndian.Uint32(reply[2:]))
}

/*
What changes on the host in a folder the machine has listed changes the
volume's date, which is what the Finder watches to read its windows again,
however deep the folder is; and so does a file it knows of being written to
*/
func TestAChangeOnTheHostMovesTheVolumeDate(t *testing.T) {
	c := newTestClient(t)
	inner := filepath.Join(c.folder, "Folder", "Inner")
	os.MkdirAll(inner, 0o755)
	c.write("Folder/Inner/file", "data")
	c.enumerate(rootID, longPath("Folder", "Inner"))
	before := c.volumeModified()

	// The dates of the host change by the second, which the test does not
	// wait for: they are moved by hand, as making a file there would
	later := before.Add(time.Hour)
	os.Chtimes(inner, later, later)
	if got := c.volumeModified(); !got.Equal(later) {
		t.Errorf("after a change in a folder two deep the volume date is %v, wanted %v", got, later)
	}

	evenLater := later.Add(time.Hour)
	os.Chtimes(filepath.Join(inner, "file"), evenLater, evenLater)
	if got := c.volumeModified(); !got.Equal(evenLater) {
		t.Errorf("after a file was written to the volume date is %v, wanted %v", got, evenLater)
	}
}

/*
A server given a time of its own answers with it, and dates with it what the
machine changes: a file made and written to, and the folder it was made in,
although the host dated them with its own time first
*/
func TestAServerGoesByTheTimeItIsGiven(t *testing.T) {
	now := time.Date(1987, 3, 2, 10, 0, 0, 0, time.UTC)
	folder := t.TempDir()
	os.Mkdir(filepath.Join(folder, "Folder"), 0o755)
	before := now.Add(-time.Hour)
	os.Chtimes(filepath.Join(folder, "Folder"), before, before)
	os.Chtimes(folder, before, before)

	s := NewServer("izmac", "Shared", folder, func() time.Time { return now })
	s.OpenSession(1)
	if _, result := s.Command(1, loginRequest(version20, uamGuest)); result != errNoErr {
		t.Fatalf("login gave %v", result)
	}
	c := &testClient{t: t, server: s, folder: folder}

	reply := c.call(errNoErr, fpGetSrvrParms, 0)
	if got := fromAFPTime(binary.BigEndian.Uint32(reply)); !got.Equal(now) {
		t.Errorf("the server's time is %v, wanted %v", got, now)
	}

	c.call(errNoErr, fpCreateFile, 0, uint16(volumeID), uint32(rootID), longPath("Folder", "File"))
	ref := c.openFork(false, accessRead|accessWrite, "Folder", "File")
	c.writeFork(ref, 0, "written")
	c.call(errNoErr, fpCloseFork, 0, ref)

	for _, name := range []string{"Folder/File", "Folder"} {
		info, err := os.Stat(filepath.Join(folder, name))
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(now) {
			t.Errorf("%v is dated %v, wanted the server's time, %v", name, info.ModTime(), now)
		}
	}
	if got := c.volumeModified(); !got.Equal(now) {
		t.Errorf("the volume is dated %v, wanted %v", got, now)
	}
}
