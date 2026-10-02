package localtalk

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

// recorder is a station that keeps what reaches it
type recorder struct {
	mutex  sync.Mutex
	frames [][]uint8
	got    chan struct{}
}

func newRecorder() *recorder {
	return &recorder{got: make(chan struct{}, 100)}
}

func (r *recorder) Receive(frame []uint8) {
	r.mutex.Lock()
	r.frames = append(r.frames, frame)
	r.mutex.Unlock()
	r.got <- struct{}{}
}

func (r *recorder) received() [][]uint8 {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return append([][]uint8(nil), r.frames...)
}

func TestAFrameReachesEveryOtherStation(t *testing.T) {
	n := NewNetwork()
	a, b, c := newRecorder(), newRecorder(), newRecorder()
	n.Attach(a)
	n.Attach(b)
	n.Attach(c)

	frame := []uint8{0xff, 0x09, 0x01, 0x00, 0x05}
	n.Send(a, frame)

	if len(a.received()) != 0 {
		t.Errorf("the sender heard its own frame")
	}
	for _, r := range []*recorder{b, c} {
		if got := r.received(); len(got) != 1 || !bytes.Equal(got[0], frame) {
			t.Errorf("a station got %x, wanted the frame", got)
		}
	}

	// Each has a copy of its own
	b.received()[0][0] = 0
	if c.received()[0][0] != 0xff {
		t.Errorf("one station's change reached another's frame")
	}
}

func TestADetachedStationHearsNothing(t *testing.T) {
	n := NewNetwork()
	a, b := newRecorder(), newRecorder()
	n.Attach(a)
	n.Attach(b)
	n.Detach(b)

	n.Send(a, []uint8{0xff, 0x09, 0x01})
	if len(b.received()) != 0 {
		t.Errorf("a station off the network got a frame")
	}
}

/*
Two networks joined to the LocalTalk of the local network, which is the same
UDP multicast group, see each other's frames and not their own. Multicast is
not everywhere, a container or a locked down CI machine among the places, and
the test skips where it can not be joined or nothing comes back.
*/
func TestTwoNetworksMeetOverUDP(t *testing.T) {
	one, two := NewNetwork(), NewNetwork()
	a, b := newRecorder(), newRecorder()
	one.Attach(a)
	two.Attach(b)

	ua, err := JoinUDP(one, "")
	if err != nil {
		t.Skipf("multicast is not available here: %v", err)
	}
	defer ua.Close()
	ub, err := JoinUDP(two, "")
	if err != nil {
		t.Skipf("multicast is not available here: %v", err)
	}
	defer ub.Close()

	frame := []uint8{0x20, 0x09, 0x01, 0x00, 0x05, 0x02, 0x02, 0x04}
	one.Send(a, frame)

	select {
	case <-b.got:
	case <-time.After(2 * time.Second):
		t.Skip("nothing came back from the multicast group, which this host may not loop back")
	}

	if got := b.received(); !bytes.Equal(got[0], frame) {
		t.Errorf("the other network got %x, wanted %x", got[0], frame)
	}

	// And the frame did not come back to its own network as from outside
	time.Sleep(100 * time.Millisecond)
	if len(a.received()) != 0 {
		t.Errorf("the sender's network heard its own frame back: %x", a.received())
	}
}

/*
The datagram as Mini vMac sends it: the sender id, four bytes big endian, and
the LLAP frame after it as it is, the destination, the source, the type and
the data
*/
func TestTheDatagramIsTheIdAndTheFrame(t *testing.T) {
	frame := []uint8{0x20, 0x09, 0x01, 0x00, 0x05, 0x02, 0x02, 0x04}
	data, ok := encodeDatagram(frame, 0x01020304)
	if !ok {
		t.Fatalf("the frame could not be encoded")
	}

	wanted := append([]uint8{0x01, 0x02, 0x03, 0x04}, frame...)
	if !bytes.Equal(data, wanted) {
		t.Errorf("the datagram is %x, wanted %x", data, wanted)
	}

	// Another sender's comes in as the frame, our own does not
	if got, ok := decodeDatagram(data, 0x99); !ok || !bytes.Equal(got, frame) {
		t.Errorf("another sender's datagram gave %x", got)
	}
	if _, ok := decodeDatagram(data, 0x01020304); ok {
		t.Errorf("our own datagram came back in")
	}

	// A control frame has no data
	enq := []uint8{0x28, 0x28, 0x81}
	data, _ = encodeDatagram(enq, 7)
	if got, ok := decodeDatagram(data, 8); !ok || !bytes.Equal(got, enq) {
		t.Errorf("a lapENQ came through as %x", got)
	}
}
