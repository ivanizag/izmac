package localtalk

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/sfiera/multitalk/pkg/llap"
	"github.com/sfiera/multitalk/pkg/ltou"
)

/*
LocalTalk over UDP, the way Mini vMac 37 and the LocalTalk tools that came
after it speak it: every frame a datagram to the multicast group
239.192.76.84, port 1954, made of a four byte sender id and the LLAP frame.
Everything on the local network listening to the group is on the same
LocalTalk: other izmacs, Mini vMacs, and through a bridge such as MultiTalk or
TashRouter, EtherTalk and real LocalTalk.

A multicast datagram comes back to the sender as well, so every transport has
an id of its own, and drops what carries it. Mini vMac uses its process id
for that; the id here is random, so that two transports in one process, a
test's for one, do not take each other for themselves.

The packets are encoded by sfiera's MultiTalk, whose ltou and llap packages
are the format as the rest of the LocalTalk world reads it.

Sending never holds up whoever sends, which is the emulated machine: a frame
is queued, and sent from a goroutine of its own, with a deadline. A network
that swallows multicast, as some managed ones do, otherwise blocks the first
send that finds the socket full, and the machine with it. A frame that finds
the queue full or does not get out in time is dropped, as a frame on a busy
wire is, and the first one that fails is said once.
*/

const (
	// maxDatagram is more than any LLAP frame, which is 603 bytes at most
	maxDatagram = 1024

	// The frames waiting to be sent, and how long each may take
	sendQueue   = 64
	sendTimeout = 100 * time.Millisecond
)

// UDP is the transport, a station that stands for everything on the group
type UDP struct {
	network *Network
	conn    *net.UDPConn
	id      uint32

	outgoing chan []uint8
	send     func([]uint8) error
	warn     func(error)
	stop     chan struct{}
	stopped  chan struct{}

	closing sync.Once
}

// newUDP is a transport with the id given, sending with the function given,
// and its sender running
func newUDP(network *Network, id uint32, send func([]uint8) error, warn func(error)) *UDP {
	u := &UDP{
		network:  network,
		id:       id,
		outgoing: make(chan []uint8, sendQueue),
		send:     send,
		warn:     warn,
		stop:     make(chan struct{}),
		stopped:  make(chan struct{}),
	}
	go u.sending()
	return u
}

/*
JoinUDP puts a network on the LocalTalk of the local network. The interface is
the one to join the group on, or empty for the system's choice. Warn, if not
nil, is told of the first frame that could not be sent.
*/
func JoinUDP(network *Network, iface string, warn func(error)) (*UDP, error) {
	var ifi *net.Interface
	if iface != "" {
		var err error
		if ifi, err = net.InterfaceByName(iface); err != nil {
			return nil, fmt.Errorf("can not use the interface %v for LocalTalk: %w", iface, err)
		}
	}

	group := ltou.MulticastAddr
	conn, err := net.ListenMulticastUDP("udp4", ifi, group)
	if err != nil {
		return nil, fmt.Errorf("can not join LocalTalk over UDP on %v: %w", group, err)
	}

	var id [4]uint8
	if _, err := rand.Read(id[:]); err != nil {
		conn.Close()
		return nil, err
	}

	send := func(data []uint8) error {
		conn.SetWriteDeadline(time.Now().Add(sendTimeout))
		_, err := conn.WriteToUDP(data, group)
		return err
	}
	u := newUDP(network, binary.BigEndian.Uint32(id[:]), send, warn)
	u.conn = conn
	network.Attach(u)
	go u.listen()
	return u, nil
}

/*
Receive queues a frame of the network to go out to the group. A frame that can
not be encoded, or finds the queue full, is dropped.
*/
func (u *UDP) Receive(frame []uint8) {
	data, ok := encodeDatagram(frame, u.id)
	if !ok {
		return
	}
	select {
	case u.outgoing <- data:
	default:
	}
}

// sending sends what is queued, until the transport closes
func (u *UDP) sending() {
	defer close(u.stopped)
	warned := false
	for {
		select {
		case <-u.stop:
			return
		case data := <-u.outgoing:
			if err := u.send(data); err != nil && !warned && u.warn != nil {
				warned = true
				u.warn(err)
			}
		}
	}
}

// encodeDatagram makes the datagram of a frame, with the sender id in front
func encodeDatagram(frame []uint8, id uint32) ([]uint8, bool) {
	var packet ltou.Packet
	packet.Pid = id
	if err := llap.Unmarshal(frame, &packet.LLAP); err != nil {
		return nil, false
	}
	data, err := ltou.Marshal(packet)
	return data, err == nil
}

// decodeDatagram takes the frame out of a datagram, unless it is one of our
// own coming back or not a frame at all
func decodeDatagram(data []uint8, own uint32) ([]uint8, bool) {
	var packet ltou.Packet
	if err := ltou.Unmarshal(data, &packet); err != nil || packet.Pid == own {
		return nil, false
	}
	frame, err := llap.Marshal(packet.LLAP)
	return frame, err == nil
}

// listen hands the frames of the others on the group to the network
func (u *UDP) listen() {
	buffer := make([]uint8, maxDatagram)
	for {
		n, _, err := u.conn.ReadFromUDP(buffer)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}

		if frame, ok := decodeDatagram(buffer[:n], u.id); ok {
			u.network.Send(u, frame)
		}
	}
}

// Close leaves the group and takes the transport off the network
func (u *UDP) Close() error {
	var err error
	u.closing.Do(func() {
		u.network.Detach(u)
		close(u.stop)
		if u.conn != nil {
			err = u.conn.Close()
		}
		<-u.stopped
	})
	return err
}
