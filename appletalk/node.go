// Package appletalk is a node on a LocalTalk network that speaks the
// protocols a Macintosh uses to find and open a file server, knowing nothing of
// the emulator or of files.
package appletalk

import (
	"encoding/binary"
	"time"

	"github.com/ivanizag/izmac/localtalk"
)

/*
A node of izmac's own on a LocalTalk network, the server side of what an
AppleShare client talks to: LLAP to be a node at all, DDP to have sockets,
NBP to answer to a name, ATP to answer requests exactly once, and ASP to hold
sessions over it. What is asked within a session is the business of a Handler,
which is where the file server goes.

The layers are from Inside AppleTalk, second edition, with the exact bytes
checked against what the Macintosh's own drivers send, the ROM's .MPP and
.ATP and the AppleShare client of System 7.

Everything runs on one goroutine: frames from the network are queued and
taken in turn, and so are the timers, so no state is ever shared. Frames go
out with Network.Send, which hands them to the other stations without
waiting.
*/

const (
	// The LLAP types
	lapShortDDP = 0x01
	lapLongDDP  = 0x02
	lapEnq      = 0x81
	lapAck      = 0x82

	lapBroadcast = 0xff

	// The node addresses a server takes, 128 to 254: 1 to 127 are the
	// workstations', which is how the Macintosh picks its own
	firstServerNode = 0xfe
	lastServerNode  = 0x80

	// The DDP types
	ddpTypeNBP = 2
	ddpTypeATP = 3

	// The sockets with fixed numbers: NBP's names information socket
	socketNBP = 2

	// The short header, the long one, and the most data either carries
	shortHeaderLength = 5
	longHeaderLength  = 13
	maxDatagramData   = 586

	// probes is how many lapENQ a node sends for an address before it takes
	// it, and probeInterval how long it waits after each for an answer.
	// Much longer than on a wire, so that a node at the other end of
	// LocalTalk over UDP has time to answer.
	probes        = 8
	probeInterval = 25 * time.Millisecond

	// tickInterval is how often the timers of ATP and ASP are looked at
	tickInterval = 100 * time.Millisecond

	// incomingFrames is how many frames can wait for the node
	incomingFrames = 256
)

// Datagram is a DDP datagram as a socket gets it
type Datagram struct {
	SourceNode   uint8
	SourceSocket uint8
	Socket       uint8
	Type         uint8
	Data         []uint8
}

// socketHandler takes the datagrams of a socket
type socketHandler func(d Datagram)

// Node is the node, a station on a LocalTalk network
type Node struct {
	network *localtalk.Network
	address uint8

	frames  chan []uint8
	calls   chan func()
	stop    chan struct{}
	stopped chan struct{}

	sockets map[uint8]socketHandler
	tickers []func(now time.Time)
}

/*
NewNode puts a node on a network and takes an address for it, which takes the
time of the probes, a fifth of a second when nobody has the first address it
tries. It does nothing more until Start.
*/
func NewNode(network *localtalk.Network) *Node {
	n := &Node{
		network: network,
		frames:  make(chan []uint8, incomingFrames),
		calls:   make(chan func(), 16),
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
		sockets: make(map[uint8]socketHandler),
	}
	network.Attach(n)
	n.acquireAddress()
	return n
}

// Address is the node's address on the network
func (n *Node) Address() uint8 {
	return n.address
}

/*
Receive takes a frame another station sent. It is called on the sender's
goroutine and only queues it: a frame that finds the queue full is dropped,
as on a busy wire.
*/
func (n *Node) Receive(frame []uint8) {
	select {
	case n.frames <- frame:
	default:
	}
}

/*
acquireAddress takes the first server address nobody answers for. Every probe
is a lapENQ for the address; a node that has it answers with a lapACK, and
the next one down is tried.
*/
func (n *Node) acquireAddress() {
	for candidate := firstServerNode; candidate >= lastServerNode; candidate-- {
		if n.probe(uint8(candidate)) {
			n.address = uint8(candidate)
			return
		}
	}
	// Every server address taken is a network nobody will build; take the
	// first one anyway rather than none
	n.address = firstServerNode
}

// probe tells whether nobody answered for an address
func (n *Node) probe(candidate uint8) bool {
	for i := 0; i < probes; i++ {
		n.network.Send(n, []uint8{candidate, candidate, lapEnq})

		deadline := time.After(probeInterval)
	waiting:
		for {
			select {
			case frame := <-n.frames:
				if len(frame) >= 3 && frame[2] == lapAck && frame[0] == candidate {
					return false
				}
			case <-deadline:
				break waiting
			}
		}
	}
	return true
}

// Start runs the node on a goroutine of its own
func (n *Node) Start() {
	go n.run()
}

// Stop takes the node off the network and waits for it to finish
func (n *Node) Stop() {
	n.network.Detach(n)
	close(n.stop)
	<-n.stopped
}

// do runs a function on the node's goroutine, which is how anything outside
// it reaches the node's state, the tests among them
func (n *Node) do(f func()) {
	done := make(chan struct{})
	n.calls <- func() {
		f()
		close(done)
	}
	<-done
}

func (n *Node) run() {
	defer close(n.stopped)
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case frame := <-n.frames:
			n.frame(frame)
		case f := <-n.calls:
			f()
		case now := <-ticker.C:
			for _, t := range n.tickers {
				t(now)
			}
		case <-n.stop:
			return
		}
	}
}

// listen gives a socket a handler, and onTick a timer, both on the node's
// goroutine
func (n *Node) listen(socket uint8, h socketHandler) {
	n.sockets[socket] = h
}

func (n *Node) onTick(t func(now time.Time)) {
	n.tickers = append(n.tickers, t)
}

// frame takes an LLAP frame off the network
func (n *Node) frame(frame []uint8) {
	if len(frame) < 3 {
		return
	}
	destination, source, kind := frame[0], frame[1], frame[2]

	if kind == lapEnq {
		// Somebody wants our address; it is ours
		if destination == n.address {
			n.network.Send(n, []uint8{source, n.address, lapAck})
		}
		return
	}
	if destination != n.address && destination != lapBroadcast {
		return
	}

	d, ok := parseDDP(kind, source, frame[3:])
	if !ok {
		return
	}
	if h, listening := n.sockets[d.Socket]; listening {
		h(d)
	}
}

/*
parseDDP takes a datagram out of the data of an LLAP frame. A short header is
the length, the two sockets and the type; a long one adds a checksum and the
network and node numbers, which on a single LocalTalk are of no use here.
*/
func parseDDP(kind uint8, source uint8, data []uint8) (Datagram, bool) {
	var d Datagram
	switch kind {
	case lapShortDDP:
		if len(data) < shortHeaderLength {
			return d, false
		}
		length := int(binary.BigEndian.Uint16(data) & 0x3ff)
		if length < shortHeaderLength || length > len(data) {
			return d, false
		}
		d.SourceNode = source
		d.Socket = data[2]
		d.SourceSocket = data[3]
		d.Type = data[4]
		d.Data = data[shortHeaderLength:length]
	case lapLongDDP:
		if len(data) < longHeaderLength {
			return d, false
		}
		length := int(binary.BigEndian.Uint16(data) & 0x3ff)
		if length < longHeaderLength || length > len(data) {
			return d, false
		}
		d.SourceNode = data[9]
		d.Socket = data[10]
		d.SourceSocket = data[11]
		d.Type = data[12]
		d.Data = data[longHeaderLength:length]
	default:
		return d, false
	}
	return d, true
}

// send puts a datagram on the network with a short header, which is all a
// single LocalTalk needs
func (n *Node) send(node uint8, socket uint8, from uint8, ddpType uint8, data []uint8) {
	if len(data) > maxDatagramData {
		return
	}
	frame := make([]uint8, 0, 3+shortHeaderLength+len(data))
	frame = append(frame, node, n.address, lapShortDDP)
	frame = binary.BigEndian.AppendUint16(frame, uint16(shortHeaderLength+len(data)))
	frame = append(frame, socket, from, ddpType)
	frame = append(frame, data...)
	n.network.Send(n, frame)
}
