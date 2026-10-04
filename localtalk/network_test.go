package localtalk

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"slices"
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

// heard says whether a frame has reached the station
func (r *recorder) heard(frame []uint8) bool {
	return slices.ContainsFunc(r.received(), func(got []uint8) bool {
		return bytes.Equal(got, frame)
	})
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
Two networks joined to LocalTalk over UDP, which is the same multicast group,
see each other's frames and not their own: two izmacs on one computer. Over
the local network that needs it to carry multicast, which a container, a
locked down CI machine or a managed firewall may not, and the test skips where
it can not be joined or nothing comes back. On the loopback interface it
needs nothing but the computer.

The group is not the test's alone. Every izmac on LocalTalk over UDP is on
it, and so are the machines of the end to end tests, which run at the same
time as these in a go test of every package: their frames reach both
networks too. So the frame sent ends in bytes of its own, and what is looked
for is that frame, not any.
*/
func meetOverUDP(t *testing.T, loopback bool) {
	one, two := NewNetwork(), NewNetwork()
	a, b := newRecorder(), newRecorder()
	one.Attach(a)
	two.Attach(b)

	ua, err := JoinUDP(one, loopback, nil)
	if err != nil {
		t.Skipf("multicast is not available here: %v", err)
	}
	defer ua.Close()
	ub, err := JoinUDP(two, loopback, nil)
	if err != nil {
		t.Skipf("multicast is not available here: %v", err)
	}
	defer ub.Close()

	frame := []uint8{0x20, 0x09, 0x01, 0x00, 0x05, 0x02, 0x02, 0x04}
	for range 8 {
		frame = append(frame, uint8(rand.N(256)))
	}
	one.Send(a, frame)

	deadline := time.After(2 * time.Second)
	for !b.heard(frame) {
		select {
		case <-b.got:
		case <-deadline:
			t.Skip("the frame did not come back from the multicast group, which this host may not let through")
		}
	}

	// And the frame did not come back to its own network as from outside
	time.Sleep(100 * time.Millisecond)
	if a.heard(frame) {
		t.Errorf("the sender's network heard its own frame back")
	}
}

func TestTwoNetworksMeetOverUDP(t *testing.T) {
	meetOverUDP(t, false)
}

func TestTwoNetworksMeetOverTheLoopback(t *testing.T) {
	meetOverUDP(t, true)
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

/*
A network that swallows multicast, as a managed one can, blocks a send once
the socket is full. The machine sending never waits for it: frames queue, the
ones that find the queue full are dropped, and the failure is said once.
*/
func TestASwallowingNetworkDoesNotHoldUpTheSender(t *testing.T) {
	var mutex sync.Mutex
	warnings := 0
	send := func([]uint8) error {
		time.Sleep(20 * time.Millisecond)
		return errors.New("i/o timeout")
	}
	u := newUDP(NewNetwork(), 1, send, func(error) {
		mutex.Lock()
		defer mutex.Unlock()
		warnings++
	})

	enq := []uint8{0x7f, 0x7f, 0x81}
	start := time.Now()
	for i := 0; i < 640; i++ {
		u.Receive(enq)
	}
	if took := time.Since(start); took > 100*time.Millisecond {
		t.Errorf("sending 640 frames took %v", took)
	}

	// Enough failures in a row to say so, at 20ms each
	time.Sleep(time.Duration(failuresToWarn+4) * 20 * time.Millisecond)
	u.Close()
	mutex.Lock()
	defer mutex.Unlock()
	if warnings != 1 {
		t.Errorf("the failure was said %v times, wanted once", warnings)
	}
}

// A send that fails now and then, on a busy computer, is not a network that
// lets nothing out, and is not said
func TestASlowSendNowAndThenIsNotWarnedOf(t *testing.T) {
	var mutex sync.Mutex
	sends, warnings := 0, 0
	send := func([]uint8) error {
		mutex.Lock()
		defer mutex.Unlock()
		sends++
		if sends%5 == 0 {
			return errors.New("i/o timeout")
		}
		return nil
	}
	u := newUDP(NewNetwork(), 1, send, func(error) {
		mutex.Lock()
		defer mutex.Unlock()
		warnings++
	})
	for i := 0; i < 50; i++ {
		u.Receive([]uint8{0x7f, 0x7f, 0x81})
		time.Sleep(time.Millisecond)
	}
	u.Close()

	mutex.Lock()
	defer mutex.Unlock()
	if warnings != 0 {
		t.Errorf("one send in five failing was warned of")
	}
}
