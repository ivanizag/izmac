package scrap

/*
Watching the scrap for a copy made on the machine.

There is no notification to hook: the Scrap Manager bumps a counter and that is
all the outside gets. So the counter is read once a frame, which costs two
peeks and no cycles of the machine.

The counter alone is not enough to read on, though. ZeroScrap bumps it and
PutScrap writes the data afterwards, two calls apart, so a read taken the
instant the counter moves finds the scrap empty. What is watched instead is the
counter, the size and the handle together, and the scrap is read once all three
have held still for a few frames. Under System 7 an application can be switched
out between the two calls, which turns that gap from microseconds into as long
as the user takes, so this is not the belt and braces it looks.
*/

// settleFrames is how long the record has to hold still before the scrap is
// read, about a fifteenth of a second
const settleFrames = 4

// Watcher reports the text copied on the machine. One frame at a time is fed
// to it and it answers when there is something new.
type Watcher struct {
	seen   signature
	settle int

	// started tells that the first reading has been taken. What is on the
	// scrap when the emulator starts, or when the watcher is turned on, is
	// remembered rather than reported: the clipboard of the host belongs to
	// whoever filled it and is not there to be overwritten by a machine
	// that has only just booted.
	started bool

	// last is the text the scrap was holding when it was last read, which is
	// what a change is measured against
	last string
}

// signature is what is watched for a change. The count moves on every copy,
// and the size and the handle catch a scrap still being filled in.
type signature struct {
	count  uint16
	size   uint32
	handle uint32
}

/*
NewWatcher returns a watcher that has not looked at the machine yet. It starts
with the settling already running, so that the first reading is taken whether
the record changes or not: a machine that has just been reset has nothing but
zeroes where the record belongs, and a watcher waiting for a change would take
its first reading from the first copy and swallow it.
*/
func NewWatcher() *Watcher {
	return &Watcher{settle: settleFrames}
}

/*
Poll looks at the scrap once, and returns the text of a copy made on the
machine since the last one it reported.
*/
func (w *Watcher) Poll(mem Memory) (string, bool) {
	stuff := Read(mem)
	now := signature{count: stuff.Count, size: stuff.Size, handle: stuff.Handle}

	if now != w.seen {
		// Something is happening to the scrap, wait for it to finish
		w.seen = now
		w.settle = settleFrames
		return "", false
	}

	if w.settle == 0 {
		return "", false
	}
	w.settle--
	if w.settle > 0 {
		return "", false
	}

	// The record has held still, so the scrap can be read
	text, found := Text(mem)

	if !w.started {
		w.started = true
		w.last = text
		return "", false
	}

	if !found || text == w.last {
		return "", false
	}

	w.last = text
	return text, true
}

/*
Suppress tells the watcher that this text is on the scrap of the machine
because it was put there from the host. Without it the next poll would find a
scrap that had changed, and send back to the host the very text that came from
it.
*/
func (w *Watcher) Suppress(text string) {
	w.started = true
	w.last = text
}
