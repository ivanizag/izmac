package afp

import (
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/unicode/norm"
)

/*
Names on the machine are Mac OS Roman, one byte a character, and on the host
UTF-8. A name from macOS comes with its accents as characters of their own
and is composed first, or every é would be an e followed by an accent.
*/

// longestName is the most a file or folder name can be on the machine
const longestName = 31

// macName is a name of the host as the machine has it: characters it cannot
// show become a question mark, and a colon, which separates the folders of a
// path there, a dash
func macName(name string, longest int) []uint8 {
	name = norm.NFC.String(name)
	out := make([]uint8, 0, len(name))
	for _, r := range name {
		if r == ':' {
			r = '-'
		}
		if r == utf8.RuneError || r < 0x20 {
			r = '?'
		}
		b, ok := charmap.Macintosh.EncodeRune(r)
		if !ok {
			b = '?'
		}
		out = append(out, b)
	}
	if len(out) > longest {
		out = out[:longest]
	}
	return out
}

// hostName is a name of the machine as the host has it
func hostName(name []uint8) string {
	runes := make([]rune, 0, len(name))
	for _, b := range name {
		runes = append(runes, charmap.Macintosh.DecodeByte(b))
	}
	return string(runes)
}
