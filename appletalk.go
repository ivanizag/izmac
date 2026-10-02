package izmac

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ivanizag/izmac/afp"
	"github.com/ivanizag/izmac/appletalk"
	"github.com/ivanizag/izmac/component"
	"github.com/ivanizag/izmac/localtalk"
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
	// izmac puts there, appleTalkHost the LocalTalk over UDP of the
	// izmacs on this computer, and appleTalkUDP that of the local network
	appleTalkLocal = "local"
	appleTalkHost  = "host"
	appleTalkUDP   = "udp"

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
			p.scc.AnswerFrame(component.ChannelB, []uint8{source, destination, lapCts})
		}
		return
	case lapCts:
		return
	}

	if p.network != nil {
		p.network.send(frame)
	}
}

// Receive takes a frame another station sent, as a station on the network
func (p *localTalkPort) Receive(frame []uint8) {
	p.deliver(frame)
}

// networkLink is the network as the port sends to it, from the port
type networkLink struct {
	network *localtalk.Network
	port    *localTalkPort
}

func (l networkLink) send(frame []uint8) {
	l.network.Send(l.port, frame)
}

/*
joinNetwork puts the port on a network, and the network on LocalTalk over UDP
if the option says so: of this computer, or of the local network
*/
func (p *localTalkPort) joinNetwork(network *localtalk.Network, appleTalk string) (*localtalk.UDP, error) {
	network.Attach(p)
	p.network = networkLink{network: network, port: p}

	if appleTalk != appleTalkUDP && appleTalk != appleTalkHost {
		return nil, nil
	}
	return localtalk.JoinUDP(network, appleTalk == appleTalkHost, func(err error) {
		fmt.Printf("AppleTalk: frames are not getting out to LocalTalk over UDP, "+
			"the machine is alone on the network: %v\n", err)
	})
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

/*
poll puts the frames that arrived on the wire of the chip, from the run loop.
A lapACK is the answer to the lapENQ the machine sent, in the same dialog, and
comes at once, as the lapCTS does; everything else after the quiet of the
wire.
*/
func (p *localTalkPort) poll() {
	for {
		select {
		case frame := <-p.incoming:
			if len(frame) >= lapHeaderLength && frame[2] == lapAck {
				p.scc.AnswerFrame(component.ChannelB, frame)
			} else {
				p.scc.ReceiveFrame(component.ChannelB, frame)
			}
		default:
			return
		}
	}
}

/*
shareFolder puts a file server on the network serving a folder of the host,
named after the host, as the Chooser lists it, with the folder's name for the
volume's
*/
func shareFolder(network *localtalk.Network, folder string) *appletalk.Listener {
	name := serverName()
	volume := filepath.Base(filepath.Clean(folder))
	server := afp.NewServer(name, volume, folder)
	return appletalk.Listen(network, server.Name(), server)
}

// serverName is the host's name without its domain, or izmac when it has none
func serverName() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "izmac"
	}
	name, _, _ = strings.Cut(name, ".")
	return name
}
