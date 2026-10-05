package appletalk

import (
	"encoding/binary"
	"time"
)

/*
ATP, the AppleTalk Transaction Protocol: a request and up to eight response
packets, retried until they arrive. An exactly once request, XO, is answered
once however many times it is retried: the responder keeps the response and
sends it again, until the requester releases it or its time runs out. That is
what lets ASP run commands that must not run twice, a write among them.

	0    control: function on the bits 7 and 6, request $40, response
	     $80, release $c0; exactly once $20; end of message $10; send
	     transaction status $08; and with XO the release timeout on the
	     bits 2 to 0, 30 seconds times a power of two
	1    the bitmap of the packets wanted, or the sequence of a response
	2    the transaction id
	4    four user bytes
	8    the data, up to 578 bytes
*/

const (
	atpRequest  = 0x40
	atpResponse = 0x80
	atpRelease  = 0xc0

	atpFunctionMask = 0xc0
	atpXO           = 0x20
	atpEOM          = 0x10
	atpTimeoutMask  = 0x07

	atpHeaderLength = 8

	// MaxATPData is the most data a packet carries, and maxResponses the
	// most packets a response is made of
	MaxATPData   = 578
	maxResponses = 8

	// releaseTimeout is how long a response is kept with the shortest
	// release timeout, which the longer ones double
	releaseTimeout = 30 * time.Second
)

// UserBytes are the four bytes of an ATP header that belong to the protocol
// above
type UserBytes [4]uint8

// atpPacket is a packet of a response
type atpPacket struct {
	user UserBytes
	data []uint8
}

// atpRequestIn is a request as a responder gets it
type atpRequestIn struct {
	node   uint8
	socket uint8
	tid    uint16
	xo     bool
	user   UserBytes
	data   []uint8
}

// responder answers the requests of a socket. respond can be called later,
// for a request whose answer has to wait on something else.
type responder func(r atpRequestIn, respond func([]atpPacket))

// responseKey is a transaction as the responder keeps it
type responseKey struct {
	node   uint8
	socket uint8
	tid    uint16
}

// keptResponse is the response to an exactly once request, kept until
// released; packets is nil while the answer is still being worked out
type keptResponse struct {
	packets []atpPacket
	expires time.Time
}

// pendingRequest is a request this node made, waiting for its response
type pendingRequest struct {
	node, socket, from uint8
	header             [atpHeaderLength]uint8
	data               []uint8
	wanted             uint8
	got                [maxResponses]*atpPacket
	retries            int
	interval           time.Duration
	next               time.Time
	done               func([]atpPacket, bool)
}

// atpSocket is a socket that speaks ATP both ways
type atpSocket struct {
	node      *Node
	socket    uint8
	responder responder
	kept      map[responseKey]*keptResponse
	pending   map[uint16]*pendingRequest
	nextTID   uint16
}

// openATP opens a socket for ATP, answering requests with a responder, which
// can be nil for a socket that only makes requests
func (n *Node) openATP(socket uint8, r responder) *atpSocket {
	s := &atpSocket{
		node:      n,
		socket:    socket,
		responder: r,
		kept:      make(map[responseKey]*keptResponse),
		pending:   make(map[uint16]*pendingRequest),
	}
	n.listen(socket, s.datagram)
	n.onTick(s.tick)
	return s
}

func (s *atpSocket) datagram(d Datagram) {
	if d.Type != ddpTypeATP || len(d.Data) < atpHeaderLength {
		return
	}
	control := d.Data[0]
	tid := binary.BigEndian.Uint16(d.Data[2:])
	var user UserBytes
	copy(user[:], d.Data[4:8])
	data := d.Data[atpHeaderLength:]

	switch control & atpFunctionMask {
	case atpRequest:
		s.request(d, control, d.Data[1], tid, user, data)
	case atpRelease:
		delete(s.kept, responseKey{d.SourceNode, d.SourceSocket, tid})
	case atpResponse:
		s.response(d, control, d.Data[1], tid, user, data)
	}
}

/*
request answers a request. A retried exactly once request finds its response
kept and gets the packets it asks for again; one whose answer is still being
worked out is ignored, as the answer goes out when it is ready.
*/
func (s *atpSocket) request(d Datagram, control uint8, bitmap uint8, tid uint16,
	user UserBytes, data []uint8) {

	if s.responder == nil {
		return
	}
	xo := control&atpXO != 0
	key := responseKey{d.SourceNode, d.SourceSocket, tid}

	if xo {
		if kept, ok := s.kept[key]; ok {
			if kept.packets != nil {
				s.sendResponse(key, kept.packets, bitmap)
			}
			return
		}
		timeout := releaseTimeout << (control & atpTimeoutMask)
		s.kept[key] = &keptResponse{expires: s.node.clock.Now().Add(timeout)}
	}

	r := atpRequestIn{node: d.SourceNode, socket: d.SourceSocket, tid: tid, xo: xo, user: user, data: data}
	s.responder(r, func(packets []atpPacket) {
		if kept, ok := s.kept[key]; ok && xo {
			kept.packets = packets
		}
		s.sendResponse(key, packets, bitmap)
	})
}

// sendResponse sends the packets of a response the bitmap asks for, the last
// one marked as the end
func (s *atpSocket) sendResponse(key responseKey, packets []atpPacket, bitmap uint8) {
	if len(packets) == 0 {
		packets = []atpPacket{{}}
	}
	for i, p := range packets {
		if i >= maxResponses || bitmap&(1<<i) == 0 {
			continue
		}
		control := uint8(atpResponse)
		if i == len(packets)-1 {
			control |= atpEOM
		}
		header := []uint8{control, uint8(i), uint8(key.tid >> 8), uint8(key.tid)}
		header = append(header, p.user[:]...)
		s.node.send(key.node, key.socket, s.socket, ddpTypeATP, append(header, p.data...))
	}
}

/*
call makes a request of another node, retried every interval as many times as
given, or forever with a negative count, and done is called with the packets
of the response or with false if it never came. A request with no done is
sent once and forgotten, which is what a tickle is.
*/
func (s *atpSocket) call(node uint8, socket uint8, user UserBytes, data []uint8, bitmap uint8,
	xo bool, retries int, interval time.Duration, done func([]atpPacket, bool)) {

	s.nextTID++
	tid := s.nextTID

	control := uint8(atpRequest)
	if xo {
		control |= atpXO
	}
	p := &pendingRequest{
		node: node, socket: socket, from: s.socket,
		data: data, wanted: bitmap,
		retries: retries, interval: interval,
		next: s.node.clock.Now().Add(interval), done: done,
	}
	p.header = [atpHeaderLength]uint8{control, bitmap, uint8(tid >> 8), uint8(tid)}
	copy(p.header[4:], user[:])

	if done != nil {
		s.pending[tid] = p
	}
	s.sendRequest(p)
}

func (s *atpSocket) sendRequest(p *pendingRequest) {
	header := p.header
	header[1] = p.wanted
	s.node.send(p.node, p.socket, p.from, ddpTypeATP, append(header[:], p.data...))
}

// response takes a packet of the response to a request this node made
func (s *atpSocket) response(d Datagram, control uint8, sequence uint8, tid uint16,
	user UserBytes, data []uint8) {

	p, ok := s.pending[tid]
	if !ok || d.SourceNode != p.node || sequence >= maxResponses {
		return
	}

	p.got[sequence] = &atpPacket{user: user, data: append([]uint8(nil), data...)}
	p.wanted &^= 1 << sequence
	if control&atpEOM != 0 {
		// Nothing after the end is coming
		p.wanted &= 1<<sequence - 1
	}
	if p.wanted != 0 {
		return
	}

	delete(s.pending, tid)

	var packets []atpPacket
	for _, g := range p.got {
		if g != nil {
			packets = append(packets, *g)
		}
	}
	p.done(packets, true)

	/*
		The release goes after whatever done sent. ASP answers a write once
		its WriteContinue is answered, and a Macintosh that gets the release
		of the WriteContinue first is still busy with it when the answer to
		the write comes, and loses that, to wait for a retry; the other way
		round it takes both.
	*/
	if p.header[0]&atpXO != 0 {
		release := [atpHeaderLength]uint8{atpRelease, 0, uint8(tid >> 8), uint8(tid)}
		s.node.send(p.node, p.socket, p.from, ddpTypeATP, release[:])
	}
}

// tick retries the requests still waiting and forgets the kept responses
// whose time has run out
func (s *atpSocket) tick(now time.Time) {
	for key, kept := range s.kept {
		if now.After(kept.expires) {
			delete(s.kept, key)
		}
	}

	for tid, p := range s.pending {
		if now.Before(p.next) {
			continue
		}
		if p.retries == 0 {
			delete(s.pending, tid)
			p.done(nil, false)
			continue
		}
		if p.retries > 0 {
			p.retries--
		}
		p.next = now.Add(p.interval)
		s.sendRequest(p)
	}
}
