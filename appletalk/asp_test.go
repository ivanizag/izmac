package appletalk

import (
	"bytes"
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"github.com/ivanizag/izmac/localtalk"
)

/*
The server is tested from a client made of the same pieces, a node of its own
on the same network doing what the AppleShare client of a Macintosh does: an
NBP lookup for =:AFPServer@*, a GetStatus, an OpenSess and commands. The
Macintosh doing it for real is the end to end test in the izmac package.
*/

// fakeServer is AFP as far as these tests need it: it echoes commands and
// keeps what is written
type fakeServer struct {
	mutex    sync.Mutex
	status   []uint8
	opened   []int
	closed   []int
	commands int
	written  []uint8
}

func (f *fakeServer) Status() []uint8 { return f.status }

func (f *fakeServer) OpenSession(id int) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.opened = append(f.opened, id)
}

func (f *fakeServer) CloseSession(id int) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.closed = append(f.closed, id)
}

func (f *fakeServer) Command(id int, request []uint8) ([]uint8, int32) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.commands++
	return append([]uint8("echo "), request...), -5000 - int32(len(request))
}

func (f *fakeServer) Write(id int, request []uint8, data []uint8) ([]uint8, int32) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.written = append(f.written, data...)
	return []uint8("written"), 0
}

func (f *fakeServer) closedSessions() []int {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return append([]int(nil), f.closed...)
}

func (f *fakeServer) count() int {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return f.commands
}

// client is the workstation side
type client struct {
	t      *testing.T
	node   *Node
	atp    *atpSocket
	wss    *atpSocket
	writes []uint8
	nbp    chan Datagram
}

const (
	clientSocket    = 0x90
	clientWSS       = 0x91
	clientNBPSocket = 0x92
)

func newClient(t *testing.T, network *localtalk.Network) *client {
	c := &client{t: t, node: NewNode(network), nbp: make(chan Datagram, 4)}
	c.atp = c.node.openATP(clientSocket, nil)
	// The workstation session socket answers the WriteContinue with the
	// data of the write
	c.wss = c.node.openATP(clientWSS, func(r atpRequestIn, respond func([]atpPacket)) {
		if r.user[0] == aspWriteContinue {
			respond(splitPackets(c.writes, UserBytes{}))
		}
	})
	c.node.listen(clientNBPSocket, func(d Datagram) { c.nbp <- d })
	c.node.Start()
	t.Cleanup(c.node.Stop)
	return c
}

// call makes a request and waits for its response
func (c *client) call(node, socket uint8, user UserBytes, data []uint8, xo bool) ([]atpPacket, bool) {
	c.t.Helper()
	result := make(chan []atpPacket, 1)
	failed := make(chan struct{})
	c.node.do(func() {
		c.atp.call(node, socket, user, data, 0xff, xo, 3, 200*time.Millisecond,
			func(packets []atpPacket, ok bool) {
				if ok {
					result <- packets
				} else {
					close(failed)
				}
			})
	})
	select {
	case p := <-result:
		return p, true
	case <-failed:
		return nil, false
	case <-time.After(3 * time.Second):
		c.t.Fatalf("no answer to the request")
		return nil, false
	}
}

// lookup does what the Chooser does to find AppleShare servers
func (c *client) lookup() (uint8, uint8, []uint8) {
	c.t.Helper()
	packet := []uint8{nbpLookup<<4 | 1, 42, 0, 0, c.node.Address(), clientNBPSocket, 0}
	for _, part := range []string{"=", "AFPServer", "*"} {
		packet = append(packet, uint8(len(part)))
		packet = append(packet, part...)
	}
	c.node.do(func() { c.node.send(lapBroadcast, socketNBP, clientNBPSocket, ddpTypeNBP, packet) })

	select {
	case d := <-c.nbp:
		if d.Data[0] != nbpLookupReply<<4|1 || d.Data[1] != 42 {
			c.t.Fatalf("the lookup was answered with %x", d.Data[:2])
		}
		tuple, ok := parseTuple(d.Data[2:])
		if !ok {
			c.t.Fatalf("the reply's tuple does not parse")
		}
		return tuple.node, tuple.socket, tuple.name[0]
	case <-time.After(2 * time.Second):
		c.t.Fatalf("nobody answered the lookup")
	}
	return 0, 0, nil
}

func serverAndClient(t *testing.T) (*fakeServer, *Listener, *client) {
	network := localtalk.NewNetwork()
	server := &fakeServer{status: bytes.Repeat([]uint8("status "), 100)}
	l := Listen(network, []uint8("izmac"), server)
	t.Cleanup(l.Stop)
	return server, l, newClient(t, network)
}

func TestTheServerTakesAServerAddress(t *testing.T) {
	_, l, c := serverAndClient(t)

	if a := l.Node().Address(); a < lastServerNode || a > firstServerNode {
		t.Errorf("the server took node %v, wanted one of 128 to 254", a)
	}
	// The client probed the server's address first, was answered, and
	// took the next one
	if c.node.Address() == l.Node().Address() {
		t.Errorf("two nodes took the address %v", c.node.Address())
	}
}

func TestTheChooserFindsTheServer(t *testing.T) {
	_, l, c := serverAndClient(t)

	node, socket, name := c.lookup()
	if node != l.Node().Address() || socket != socketListening || string(name) != "izmac" {
		t.Errorf("the lookup found %q at %v:%v", name, node, socket)
	}
}

func TestALookupForAnotherTypeIsNotAnswered(t *testing.T) {
	_, _, c := serverAndClient(t)

	packet := []uint8{nbpLookup<<4 | 1, 7, 0, 0, c.node.Address(), clientNBPSocket, 0}
	for _, part := range []string{"=", "LaserWriter", "*"} {
		packet = append(packet, uint8(len(part)))
		packet = append(packet, part...)
	}
	c.node.do(func() { c.node.send(lapBroadcast, socketNBP, clientNBPSocket, ddpTypeNBP, packet) })

	select {
	case <-c.nbp:
		t.Errorf("a lookup for LaserWriters was answered by a file server")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestTheStatusComesInPackets(t *testing.T) {
	server, l, c := serverAndClient(t)

	packets, ok := c.call(l.Node().Address(), socketListening, UserBytes{aspGetStatus}, nil, false)
	if !ok {
		t.Fatalf("GetStatus failed")
	}
	var status []uint8
	for _, p := range packets {
		status = append(status, p.data...)
	}
	if !bytes.Equal(status, server.status) || len(packets) < 2 {
		t.Errorf("the status came back as %v bytes in %v packets, wanted %v bytes in two",
			len(status), len(packets), len(server.status))
	}
}

// openSession opens a session and returns its socket and id
func openSession(t *testing.T, l *Listener, c *client) (uint8, uint8) {
	t.Helper()
	user := UserBytes{aspOpenSession, clientWSS}
	binary.BigEndian.PutUint16(user[2:], aspVersion)
	packets, ok := c.call(l.Node().Address(), socketListening, user, nil, true)
	if !ok || len(packets) != 1 {
		t.Fatalf("OpenSess failed")
	}
	u := packets[0].user
	if code := int16(binary.BigEndian.Uint16(u[2:])); code != 0 {
		t.Fatalf("OpenSess was refused with %v", code)
	}
	return u[0], u[1]
}

func TestASessionRunsCommands(t *testing.T) {
	server, l, c := serverAndClient(t)
	socket, id := openSession(t, l, c)

	packets, ok := c.call(l.Node().Address(), socket, UserBytes{aspCommand, id, 0, 1}, []uint8("FPGetSrvrParms"), true)
	if !ok {
		t.Fatalf("the command failed")
	}
	result := int32(binary.BigEndian.Uint32(packets[0].user[:]))
	if result != -5014 || string(packets[0].data) != "echo FPGetSrvrParms" {
		t.Errorf("the command gave %q and %v", packets[0].data, result)
	}
	if len(server.opened) != 1 {
		t.Errorf("the server was told of %v sessions opening", len(server.opened))
	}
}

/*
A retried command is not run again: its response is kept, and sent once more,
until the requester releases it. The retry is sent by hand with the same
transaction id, as a requester whose response got lost would send it.
*/
func TestACommandRunsOnceHoweverOftenItIsSent(t *testing.T) {
	server, l, c := serverAndClient(t)
	socket, id := openSession(t, l, c)

	request := []uint8{atpRequest | atpXO, 0xff, 0x12, 0x34, aspCommand, id, 0, 1}
	request = append(request, "once"...)
	for i := 0; i < 3; i++ {
		c.node.do(func() { c.node.send(l.Node().Address(), socket, clientSocket, ddpTypeATP, request) })
		time.Sleep(50 * time.Millisecond)
	}

	if n := server.count(); n != 1 {
		t.Errorf("a command sent three times ran %v times", n)
	}
}

func TestAWriteFetchesItsData(t *testing.T) {
	server, l, c := serverAndClient(t)
	socket, id := openSession(t, l, c)
	c.writes = bytes.Repeat([]uint8{0xab}, 1500)

	packets, ok := c.call(l.Node().Address(), socket, UserBytes{aspWrite, id, 0, 2}, []uint8("FPWrite"), true)
	if !ok || string(packets[0].data) != "written" {
		t.Fatalf("the write was answered with %q", packets[0].data)
	}
	if !bytes.Equal(server.written, c.writes) {
		t.Errorf("the server got %v bytes, wanted the %v written", len(server.written), len(c.writes))
	}
}

func TestASessionCloses(t *testing.T) {
	server, l, c := serverAndClient(t)
	socket, id := openSession(t, l, c)

	if _, ok := c.call(l.Node().Address(), socket, UserBytes{aspCloseSession, id}, nil, true); !ok {
		t.Fatalf("CloseSess failed")
	}
	if closed := server.closedSessions(); len(closed) != 1 || closed[0] != int(id) {
		t.Errorf("the server was told of %v closing", closed)
	}

	// And commands for it are not answered any more
	if _, ok := c.call(l.Node().Address(), socket, UserBytes{aspCommand, id, 0, 3}, nil, true); ok {
		t.Errorf("a closed session still runs commands")
	}
}

func TestAWrongVersionIsRefused(t *testing.T) {
	_, l, c := serverAndClient(t)

	user := UserBytes{aspOpenSession, clientWSS, 0x02, 0x00}
	packets, ok := c.call(l.Node().Address(), socketListening, user, nil, true)
	if !ok {
		t.Fatalf("OpenSess was not answered")
	}
	if code := int16(binary.BigEndian.Uint16(packets[0].user[2:])); code != aspBadVersion {
		t.Errorf("ASP 2.0 was answered with %v, wanted the bad version error", code)
	}
}
