package scrap

/*
The layout of the block the scrap handle points at. Each entry is a four
character type, a length as a long, and the bytes, padded to an even boundary
because the 68000 can not read a word at an odd address.

	'TEXT'  00000005  "Hello" 00
	'styl'  0000000e  ...

A truncated entry ends the walk rather than failing it. The scrap is read
while the machine is running and not asked for, so a block caught halfway
through being written gives whatever entries were complete and no more.
*/

const entryHeaderSize = 8

// Entries returns the type of every entry of a scrap block, in the order they
// are in it. It is what a tracer or a test uses to say what is on the
// clipboard without caring about the contents.
func Entries(data []byte) []string {
	types := make([]string, 0, 2)

	walk(data, func(entryType string, contents []byte) bool {
		types = append(types, entryType)
		return true
	})

	return types
}

// Entry returns the contents of an entry of the given type, and whether it was
// there
func Entry(data []byte, wanted string) ([]byte, bool) {
	var found []byte

	walk(data, func(entryType string, contents []byte) bool {
		if entryType != wanted {
			return true
		}
		found = contents
		return false
	})

	return found, found != nil
}

// walk calls back with every whole entry of a block until it runs out of them
// or the callback asks it to stop
func walk(data []byte, visit func(entryType string, contents []byte) bool) {
	for at := 0; at+entryHeaderSize <= len(data); {
		entryType := string(data[at : at+4])
		length := int(uint32(data[at+4])<<24 | uint32(data[at+5])<<16 |
			uint32(data[at+6])<<8 | uint32(data[at+7]))

		from := at + entryHeaderSize
		if length < 0 || from+length > len(data) {
			// The entry does not fit in what is there, so this is as far as
			// the block can be trusted
			return
		}

		if !visit(entryType, data[from:from+length]) {
			return
		}

		at = from + length
		if length%2 != 0 {
			at++
		}
	}
}
