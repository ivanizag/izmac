package appletalk

import (
	"encoding/binary"
	"time"

	"github.com/ivanizag/izmac/localtalk"
)

/*
ASP, the AppleTalk Session Protocol, which AFP runs over: sessions between a
workstation and a server, each command an exactly once ATP request, and the
answer its response. Which of the calls an ATP request is goes in its first
user byte.

	GetStatus  to the session listening socket; the response is the
	           server's status, the block the Chooser shows the server by
	OpenSess   to the listening socket, with the socket the workstation
	           listens on and the version, 1.0; the response gives the
	           server session socket and the session id
	Command    to the session socket, with the session id and a sequence;
	           the data is the AFP call and the response its result and
	           its reply
	Write      the same for a call that sends data, a write: the server
	           asks for the data with a WriteContinue to the workstation
	           before it answers
	Tickle     every 30 seconds both ways, to say the other end is there;
	           a session with nothing from the workstation for two minutes
	           is closed
	CloseSess  the end of a session
*/

const (
	aspCloseSession  = 1
	aspCommand       = 2
	aspGetStatus     = 3
	aspOpenSession   = 4
	aspTickle        = 5
	aspWrite         = 6
	aspWriteContinue = 7

	// aspVersion is the one ASP version there is, 1.0
	aspVersion = 0x0100

	// The ASP errors an OpenSess can answer with
	aspBadVersion      = -1066
	aspTooManySessions = -1068

	// The sockets of the server: where sessions are opened and where
	// they run. Dynamic sockets, from the upper half.
	socketListening = 0xfb
	socketSession   = 0xfc

	// QuantumSize is the most a command's reply or a write's data can be,
	// eight packets of ATP
	QuantumSize = maxResponses * MaxATPData

	tickleInterval = 30 * time.Second
	sessionTimeout = 2 * time.Minute

	// The retries of the WriteContinue: every two seconds, for a minute,
	// after which a workstation that has not answered has gone
	writeContinueInterval = 2 * time.Second
	writeContinueRetries  = 30

	maxSessions = 64
)

/*
Server is what runs in the sessions: AFP. Its calls come on the node's
goroutine, one at a time.
*/
type Server interface {
	// Status is the block GetStatus answers with
	Status() []uint8
	// OpenSession and CloseSession say a session started and ended
	OpenSession(session int)
	CloseSession(session int)
	// Command runs a call and returns its reply and its result code
	Command(session int, request []uint8) ([]uint8, int32)
	// Write runs a call that came with data
	Write(session int, request []uint8, data []uint8) ([]uint8, int32)
}

// session is a session with a workstation
type session struct {
	id         uint8
	node       uint8
	socket     uint8
	lastHeard  time.Time
	lastTickle time.Time
}

// Listener is an ASP server on a node
type Listener struct {
	node     *Node
	server   Server
	session  *atpSocket
	sessions map[uint8]*session
	nextID   uint8
}

/*
Listen puts a server on a network: a node of its own, with the name given,
of type AFPServer, answering to the Chooser. The name is in Mac OS Roman.
*/
func Listen(network *localtalk.Network, name []uint8, server Server) *Listener {
	n := NewNode(network)
	l := &Listener{
		node:     n,
		server:   server,
		sessions: make(map[uint8]*session),
	}

	names := n.startNBP()
	names.register(name, []uint8("AFPServer"), socketListening)
	n.openATP(socketListening, l.listening)
	l.session = n.openATP(socketSession, l.sessionRequest)
	n.onTick(l.tick)

	n.Start()
	return l
}

// Node is the listener's node on the network
func (l *Listener) Node() *Node {
	return l.node
}

// Sessions is how many sessions are open, for while the listener runs
func (l *Listener) Sessions() int {
	var n int
	l.node.do(func() { n = len(l.sessions) })
	return n
}

// Stop takes the server off the network
func (l *Listener) Stop() {
	l.node.Stop()
}

// listening answers what comes to the listening socket
func (l *Listener) listening(r atpRequestIn, respond func([]atpPacket)) {
	switch r.user[0] {
	case aspGetStatus:
		respond(splitPackets(l.server.Status(), UserBytes{}))
	case aspOpenSession:
		respond([]atpPacket{{user: l.openSession(r)}})
	case aspTickle:
		if s, ok := l.sessions[r.user[1]]; ok && s.node == r.node {
			s.lastHeard = time.Now()
		}
		// A tickle is not answered: its retries are the tickles
	}
}

// openSession starts a session for a workstation and says where it runs
func (l *Listener) openSession(r atpRequestIn) UserBytes {
	if binary.BigEndian.Uint16(r.user[2:]) != aspVersion {
		return errorUserBytes(aspBadVersion)
	}
	if len(l.sessions) >= maxSessions {
		return errorUserBytes(aspTooManySessions)
	}

	for {
		l.nextID++
		if _, taken := l.sessions[l.nextID]; !taken && l.nextID != 0 {
			break
		}
	}
	now := time.Now()
	s := &session{id: l.nextID, node: r.node, socket: r.user[1], lastHeard: now, lastTickle: now}
	l.sessions[s.id] = s
	l.server.OpenSession(int(s.id))

	return UserBytes{socketSession, s.id, 0, 0}
}

// errorUserBytes is an OpenSess refused, with the error in the last two bytes
func errorUserBytes(code int16) UserBytes {
	var user UserBytes
	binary.BigEndian.PutUint16(user[2:], uint16(code))
	return user
}

// sessionRequest answers what comes to the session socket
func (l *Listener) sessionRequest(r atpRequestIn, respond func([]atpPacket)) {
	s, ok := l.sessions[r.user[1]]
	if !ok || s.node != r.node {
		return
	}
	s.lastHeard = time.Now()

	switch r.user[0] {
	case aspCommand:
		reply, result := l.server.Command(int(s.id), r.data)
		respond(splitPackets(reply, resultUserBytes(result)))
	case aspWrite:
		l.write(s, r, respond)
	case aspCloseSession:
		// Closed before the answer goes, so that a workstation that has
		// the answer finds the session gone
		l.closeSession(s)
		respond(nil)
	}
}

/*
write gets the data of a write from the workstation, with a WriteContinue
carrying the same sequence and the room there is for it, and answers the write
once the data is in
*/
func (l *Listener) write(s *session, r atpRequestIn, respond func([]atpPacket)) {
	user := UserBytes{aspWriteContinue, s.id, r.user[2], r.user[3]}
	room := binary.BigEndian.AppendUint16(nil, QuantumSize)

	l.session.call(s.node, s.socket, user, room, 0xff, true, writeContinueRetries, writeContinueInterval,
		func(packets []atpPacket, ok bool) {
			if !ok {
				return
			}
			var data []uint8
			for _, p := range packets {
				data = append(data, p.data...)
			}
			reply, result := l.server.Write(int(s.id), r.data, data)
			respond(splitPackets(reply, resultUserBytes(result)))
		})
}

func (l *Listener) closeSession(s *session) {
	delete(l.sessions, s.id)
	l.server.CloseSession(int(s.id))
}

// tick tickles the workstations and closes the sessions they have left
func (l *Listener) tick(now time.Time) {
	for _, s := range l.sessions {
		if now.Sub(s.lastHeard) > sessionTimeout {
			l.closeSession(s)
			continue
		}
		if now.Sub(s.lastTickle) >= tickleInterval {
			s.lastTickle = now
			l.session.call(s.node, s.socket, UserBytes{aspTickle, s.id}, nil, 0, false, 0, 0, nil)
		}
	}
}

// resultUserBytes is the result of a command, as its response carries it
func resultUserBytes(result int32) UserBytes {
	var user UserBytes
	binary.BigEndian.PutUint32(user[:], uint32(result))
	return user
}

// splitPackets cuts a reply into the packets of a response, each with the
// same user bytes, which is where ASP puts the result
func splitPackets(data []uint8, user UserBytes) []atpPacket {
	if len(data) > QuantumSize {
		data = data[:QuantumSize]
	}
	packets := []atpPacket{}
	for len(data) > MaxATPData {
		packets = append(packets, atpPacket{user: user, data: data[:MaxATPData]})
		data = data[MaxATPData:]
	}
	return append(packets, atpPacket{user: user, data: data})
}
