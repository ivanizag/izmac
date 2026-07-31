package scrap

import "testing"

// poll runs the watcher over a machine that is holding still, which is what
// the frames after a copy look like
func poll(w *Watcher, mem Memory, frames int) (string, bool) {
	for i := 0; i < frames; i++ {
		if text, found := w.Poll(mem); found {
			return text, true
		}
	}
	return "", false
}

// settled is enough frames for the watcher to be sure the scrap has stopped
// moving, with one to spare
const settled = settleFrames + 1

/*
What is on the scrap when the emulator starts belongs to whoever was using the
host, and the machine has done nothing to earn the right to replace it.
*/
func TestTheFirstReadingIsNotReported(t *testing.T) {
	mem := testMemory{}
	putScrap(mem, buildBlock("TEXT", "left over from last time"))

	if text, found := poll(NewWatcher(), mem, settled); found {
		t.Errorf("the scrap was sent to the host on the first look, as %q", text)
	}
}

/*
A machine that has just been reset has nothing where the record of the Scrap
Manager belongs. That is a reading like any other, and taking it is what keeps
the first copy after the boot from being taken for it and swallowed.
*/
func TestTheFirstCopyAfterABootIsReported(t *testing.T) {
	mem := testMemory{}

	w := NewWatcher()
	poll(w, mem, settled)

	putScrap(mem, buildBlock("TEXT", "the first copy"))

	text, found := poll(w, mem, settled)
	if !found {
		t.Fatal("the first copy after a boot was not reported")
	}
	if text != "the first copy" {
		t.Errorf("the copy came out as %q, wanted %q", text, "the first copy")
	}
}

func TestACopyIsReported(t *testing.T) {
	mem := testMemory{}
	putScrap(mem, buildBlock("TEXT", "before"))

	w := NewWatcher()
	poll(w, mem, settled)

	// A copy on the machine: the count moves and the block is replaced
	putScrap(mem, buildBlock("TEXT", "after"))
	mem.pokeWord(countAddress, 2)

	text, found := poll(w, mem, settled)
	if !found {
		t.Fatal("a copy on the machine was not reported")
	}
	if text != "after" {
		t.Errorf("the copy came out as %q, wanted %q", text, "after")
	}

	// And it is reported once, not on every frame after it
	if text, found := poll(w, mem, settled); found {
		t.Errorf("the same copy was reported again, as %q", text)
	}
}

/*
ZeroScrap bumps the count and PutScrap fills the block afterwards, so the
scrap is empty for as long as it takes the application to make the second
call. Reading on the count alone would send that emptiness to the host and
lose the clipboard.
*/
func TestAScrapStillBeingWrittenIsNotReported(t *testing.T) {
	mem := testMemory{}
	putScrap(mem, buildBlock("TEXT", "before"))

	w := NewWatcher()
	poll(w, mem, settled)

	// ZeroScrap has run and PutScrap has not
	mem.pokeWord(countAddress, 2)
	mem.pokeLong(sizeAddress, 0)

	if text, found := poll(w, mem, settleFrames-1); found {
		t.Errorf("the scrap was read halfway through a copy, as %q", text)
	}

	// PutScrap now writes what was copied
	putScrap(mem, buildBlock("TEXT", "after"))
	mem.pokeWord(countAddress, 2)

	text, found := poll(w, mem, settled)
	if !found {
		t.Fatal("the copy was not reported once it had been written")
	}
	if text != "after" {
		t.Errorf("the copy came out as %q, wanted %q", text, "after")
	}
}

/*
A paste from the host bumps the count of the scrap exactly as a copy on the
machine does. Without the watcher being told, the text would be sent straight
back to the host it came from.
*/
func TestTextPastedFromTheHostIsNotSentBack(t *testing.T) {
	mem := testMemory{}
	w := NewWatcher()
	poll(w, mem, settled)

	w.Suppress("from the host")
	putScrap(mem, buildBlock("TEXT", "from the host"))
	mem.pokeWord(countAddress, 7)

	if text, found := poll(w, mem, settled); found {
		t.Errorf("the text pasted from the host came back as %q", text)
	}

	// What is copied after it is still reported
	putScrap(mem, buildBlock("TEXT", "copied on the machine"))
	mem.pokeWord(countAddress, 8)

	if _, found := poll(w, mem, settled); !found {
		t.Error("the copy after a paste was not reported")
	}
}
