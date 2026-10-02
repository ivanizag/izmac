package afp

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"runtime"
	"strings"
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

/*
catalogName is the name a file or folder of the host has on the machine, and
whether it is the host's name as it is. A slash, which separates the folders
of a path on the host, is a colon on the machine, the way macOS shows it, and
the other way round. A name that is longer than the machine takes, or has
characters the machine cannot show, is not as it is: the caller makes it
unique.
*/
func catalogName(host string) ([]uint8, bool) {
	host = norm.NFC.String(host)
	exact := true
	out := make([]uint8, 0, len(host))
	for _, r := range host {
		switch {
		case r == ':':
			r = '/'
		case r == utf8.RuneError || r < 0x20 && r != '\r':
			r = '?'
			exact = false
		}
		b, ok := charmap.Macintosh.EncodeRune(r)
		if !ok {
			b = '?'
			exact = false
		}
		out = append(out, b)
	}
	if len(out) > longestName {
		exact = false
	}
	return out, exact
}

/*
mangledName is a name the machine can have for one of the host that it cannot
have as it is: as much of it as fits, a number made from the host's name to
tell it from others like it, and the extension, if it is short, so that what
the file is still shows
*/
func mangledName(name []uint8, host string) []uint8 {
	tag := []uint8(fmt.Sprintf("#%04X", crc32.ChecksumIEEE([]uint8(host))&0xffff))

	var extension []uint8
	if dot := bytes.LastIndexByte(name, '.'); dot > 0 && len(name)-dot <= 5 {
		extension = name[dot:]
		name = name[:dot]
	}
	room := longestName - len(tag) - len(extension)
	if len(name) > room {
		name = name[:room]
	}
	out := append(append([]uint8{}, name...), tag...)
	return append(out, extension...)
}

/*
hostFileName is the name a file or folder the machine makes gets on the host:
the slash of the machine a colon. Windows takes neither, and a few other
characters besides, and gets those as underscores.
*/
func hostFileName(name []uint8) string {
	out := []rune(hostName(name))
	for i, r := range out {
		switch {
		case r == '/':
			out[i] = ':'
		case r < 0x20:
			out[i] = '_'
		}
		if runtime.GOOS == "windows" && strings.ContainsRune(`<>:"/\|?*`, out[i]) {
			out[i] = '_'
		}
	}
	return string(out)
}

// sameName compares two names as the machine does, without regard to case
func sameName(a []uint8, b []uint8) bool {
	return strings.EqualFold(hostName(a), hostName(b))
}

// nameOrder is what names are sorted by, so that a folder is listed in the
// same order every time
func nameOrder(name []uint8) string {
	return strings.ToLower(hostName(name))
}
