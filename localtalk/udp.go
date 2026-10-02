package localtalk

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"

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
*/

// maxDatagram is more than any LLAP frame, which is 603 bytes at most
const maxDatagram = 1024

// UDP is the transport, a station that stands for everything on the group
type UDP struct {
	network *Network
	conn    *net.UDPConn
	group   *net.UDPAddr
	id      uint32

	closing sync.Once
}

/*
JoinUDP puts a network on the LocalTalk of the local network. The interface is
the one to join the group on, or empty for the system's choice.
*/
func JoinUDP(network *Network, iface string) (*UDP, error) {
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

	u := &UDP{
		network: network,
		conn:    conn,
		group:   group,
		id:      binary.BigEndian.Uint32(id[:]),
	}
	network.Attach(u)
	go u.listen()
	return u, nil
}

/*
Receive sends a frame of the network out to the group. A frame that can not be
encoded or sent is dropped, as a frame on a wire can be.
*/
func (u *UDP) Receive(frame []uint8) {
	if data, ok := encodeDatagram(frame, u.id); ok {
		u.conn.WriteToUDP(data, u.group)
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
		err = u.conn.Close()
	})
	return err
}
