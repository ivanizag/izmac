package scrap

import "testing"

/*
The line endings, which are what tells a paste that arrived as text from a
paste that arrived as one long line. The pair the host writes is one ending
and not two, and the machine writes neither of them.
*/
func TestTheLineEndingsAreTranslated(t *testing.T) {
	data := ToMac("one\ntwo\r\nthree")
	if string(data) != "one\rtwo\rthree" {
		t.Errorf("the line endings went to the machine as %q, wanted %q",
			data, "one\rtwo\rthree")
	}

	text := FromMac([]byte("one\rtwo\rthree"))
	if text != "one\ntwo\nthree" {
		t.Errorf("the line endings came back as %q, wanted %q",
			text, "one\ntwo\nthree")
	}
}

/*
The upper half of the characters is Mac OS Roman, which is where the Macintosh
put the accents ten years before Unicode existed. The e acute is $8e there and
nowhere near the $e9 of Latin-1 or the two bytes of UTF-8.
*/
func TestTheUpperHalfIsMacRoman(t *testing.T) {
	data := ToMac("café")
	if string(data) != "caf\x8e" {
		t.Errorf("the accent went to the machine as % x, wanted 63 61 66 8e", data)
	}

	if text := FromMac(data); text != "café" {
		t.Errorf("the accent came back as %q, wanted %q", text, "café")
	}
}

/*
A character the Macintosh has no byte for is replaced and the rest of the
paste goes through. Refusing the whole thing over one em dash would be worse,
and there is no third answer: the scrap holds one byte per character.
*/
func TestACharacterTheMacintoshDoesNotHaveIsReplaced(t *testing.T) {
	data := ToMac("a ☃ b")
	if string(data) != "a ? b" {
		t.Errorf("an unknown character went to the machine as %q, wanted %q",
			data, "a ? b")
	}
}

// Every byte means something in Mac OS Roman, so nothing read off the scrap
// can fail to be text
func TestEveryByteOfTheScrapDecodes(t *testing.T) {
	data := make([]byte, 256)
	for i := range data {
		data[i] = uint8(i)
	}

	// The carriage return becomes a line feed and the rest stand for
	// themselves, so the count is the same either way
	if text := []rune(FromMac(data)); len(text) != len(data) {
		t.Errorf("256 bytes of scrap decoded to %v characters", len(text))
	}
}
