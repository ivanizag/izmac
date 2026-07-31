package scrap

import (
	"strings"

	"golang.org/x/text/encoding/charmap"
)

/*
The text of the Macintosh is not the text of the host, in two ways.

The characters are Mac OS Roman, one byte each: the lower half is ASCII and
the upper half is where the accents, the currencies and the mathematical signs
of the Macintosh live, in an order of its own that predates Unicode by a
decade. golang.org/x/text has the table.

The line ending is a carriage return, where the host uses a line feed or the
two of them. A paste that does not translate them arrives as one long line in
MacWrite, or as a paragraph per character.

A character the Macintosh has no byte for is replaced rather than refused. The
alternative is a paste that fails because of one em dash, which is worse than
a paste with a question mark in it.
*/

// replacement stands in for a character the Macintosh does not have
const replacement = '?'

// FromMac turns the bytes of a scrap entry into a Go string
func FromMac(data []byte) string {
	var sb strings.Builder
	sb.Grow(len(data))

	for _, b := range data {
		if b == '\r' {
			sb.WriteByte('\n')
			continue
		}
		sb.WriteRune(charmap.Macintosh.DecodeByte(b))
	}

	return sb.String()
}

// ToMac turns a Go string into the bytes a scrap entry is made of
func ToMac(text string) []byte {
	// The pair first, so that a carriage return and line feed together make
	// one line ending and not two
	text = strings.ReplaceAll(text, "\r\n", "\n")

	data := make([]byte, 0, len(text))
	for _, r := range text {
		if r == '\n' || r == '\r' {
			data = append(data, '\r')
			continue
		}

		b, known := charmap.Macintosh.EncodeRune(r)
		if !known {
			b = replacement
		}
		data = append(data, b)
	}

	return data
}
