package izmac

import (
	"github.com/ivanizag/izmac/component"
)

/*
AppleTalk on the printer port: what joins the SCC in SDLC mode to a LocalTalk
network.

The Macintosh runs its own AppleTalk, the .MPP and .ATP drivers of the ROM, and
only needs the chip to behave and something on the other end of the wire. That
something is a network of frames, LLAP frames: a destination node, a source
node, a type, and the data. The one part of LLAP that cannot cross a network
of any real latency is the handshake: a station sends a lapRTS before every
frame to a single node, and the node has about 200µs to answer with a lapCTS.
So the handshake never leaves the machine. The port answers a directed lapRTS
itself with the lapCTS the other node would have sent, and neither RTS nor CTS
goes on the network, which is what Mini vMac does and what LocalTalk over UDP
expects. Everything else, the data and the ENQ and ACK of the address
acquisition, goes on the network as it is.
*/

const (
	// appleTalkLocal is a LocalTalk network with nothing on it but what
	// izmac puts there
	appleTalkLocal = "local"

	// The LLAP control frames, by type
	lapEnq = 0x81
	lapAck = 0x82
	lapRts = 0x84
	lapCts = 0x85

	// lapBroadcast is the node every station listens to
	lapBroadcast = 0xff

	// lapHeaderLength is the destination, the source and the type
	lapHeaderLength = 3
)

/*
localTalkNetwork is what the frames of the machine go out to and come back
from. Frames arriving can come from other goroutines, so they are handed over
through a channel the run loop drains.
*/
type localTalkNetwork interface {
	send(frame []uint8)
}

// localTalkPort is the printer port in its LocalTalk role
type localTalkPort struct {
	scc      *component.SCC8530
	network  localTalkNetwork
	incoming chan []uint8
}

// localTalkIncoming is how many frames can wait for the run loop
const localTalkIncoming = 64

func newLocalTalkPort(scc *component.SCC8530) *localTalkPort {
	p := &localTalkPort{
		scc:      scc,
		incoming: make(chan []uint8, localTalkIncoming),
	}
	scc.AttachLink(component.ChannelB, p)
	return p
}

/*
SendFrame takes a frame the machine sent. A directed lapRTS is answered at once
with the lapCTS, and neither goes further; the rest goes to the network.
*/
func (p *localTalkPort) SendFrame(frame []uint8) {
	if len(frame) < lapHeaderLength {
		return
	}

	destination, source, kind := frame[0], frame[1], frame[2]
	switch kind {
	case lapRts:
		if destination != lapBroadcast {
			p.scc.ReceiveFrame(component.ChannelB, []uint8{source, destination, lapCts})
		}
		return
	case lapCts:
		return
	}

	if p.network != nil {
		p.network.send(frame)
	}
}

/*
deliver hands a frame from the network to the machine. It is safe to call from
any goroutine; a frame that finds the queue full is dropped, as a frame is on
a busy wire.
*/
func (p *localTalkPort) deliver(frame []uint8) {
	select {
	case p.incoming <- frame:
	default:
	}
}

// poll puts the frames that arrived on the wire of the chip, from the run loop
func (p *localTalkPort) poll() {
	for {
		select {
		case frame := <-p.incoming:
			p.scc.ReceiveFrame(component.ChannelB, frame)
		default:
			return
		}
	}
}
