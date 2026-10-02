// Package localtalk carries LocalTalk frames between the stations of a
// network, in the process and over UDP, knowing nothing of the emulator.
package localtalk

import "sync"

/*
A LocalTalk network, as LLAP frames: the destination node, the source node,
the type, and the data. The flags, the CRC and the RTS/CTS handshake are the
business of each station's own link, which answers them at home; the network
carries the frames that matter, data and the ENQ and ACK of the address
acquisition, from each station to all the others, the way a wire does.

A station can be the printer port of an emulated Macintosh, a transport that
joins the network to others over UDP, or a node of izmac's own. Frames can be
sent from any goroutine, and a station is handed the frames of the others on
the goroutine of whoever sent them, so a station takes them quickly and
passes them on to its own.
*/

// Station is something on the network that frames are delivered to
type Station interface {
	Receive(frame []uint8)
}

// Network joins stations: a frame sent by one reaches every other
type Network struct {
	mutex    sync.Mutex
	stations []Station
}

// NewNetwork makes a network with nothing on it
func NewNetwork() *Network {
	return &Network{}
}

// Attach puts a station on the network
func (n *Network) Attach(station Station) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.stations = append(n.stations, station)
}

// Detach takes a station off the network
func (n *Network) Detach(station Station) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	for i, s := range n.stations {
		if s == station {
			n.stations = append(n.stations[:i], n.stations[i+1:]...)
			return
		}
	}
}

/*
Send puts a frame on the network from one of its stations. Every other station
gets a copy of its own, so that none can change what another one sees.
*/
func (n *Network) Send(from Station, frame []uint8) {
	n.mutex.Lock()
	others := make([]Station, 0, len(n.stations))
	for _, s := range n.stations {
		if s != from {
			others = append(others, s)
		}
	}
	n.mutex.Unlock()

	for _, s := range others {
		s.Receive(append([]uint8(nil), frame...))
	}
}
