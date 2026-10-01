package unwrap

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"golang.org/x/text/encoding/charmap"
)

/*
BinHex 4.0, the seven bit text a Macintosh file was mailed and posted as.

The file starts with whatever text the sender put in front, the line
"(This file must be converted with BinHex 4.0)", and then the data between two
colons, six bits to a character out of a 64 character alphabet, with line
breaks anywhere. Undone, that is run length encoded: $90 followed by a count
repeats the byte before it, and $90 followed by zero is a $90.

What comes out is a header, the data fork and the resource fork, each closed
by its CRC:

	1     the length of the name
	n     the name, in Mac OS Roman
	1     a version, zero
	4     the type
	4     the creator
	2     the Finder flags
	4     the length of the data fork
	4     the length of the resource fork
	2     the CRC of the header
*/

const (
	binhexMarker   = "(This file must be converted with BinHex"
	binhexAlphabet = "!\"#$%&'()*+,-012345689@ABCDEFGHIJKLMNPQRSTUVXYZ[`abcdefhijklmpqr"

	binhexRepeat   = 0x90
	binhexInvalid  = 0xff
	binhexMaxName  = 63
	binhexFixedLen = 1 + 4 + 4 + 2 + 4 + 4 // version to resource fork length
)

// binhexDecoder carries the alphabet turned round, a character to its six
// bits
type binhexDecoder struct {
	values [256]uint8
}

func newBinhexDecoder() *binhexDecoder {
	d := &binhexDecoder{}
	for i := range d.values {
		d.values[i] = binhexInvalid
	}
	for i := 0; i < len(binhexAlphabet); i++ {
		d.values[binhexAlphabet[i]] = uint8(i)
	}
	return d
}

/*
identify looks for the marker line. Everything before it has to be text, the
mail headers and the description people put there: a disk image of a BinHex
utility can carry the same words somewhere, and is not to be taken for one.
*/
func (d *binhexDecoder) identify(head []uint8) bool {
	at := bytes.Index(head, []uint8(binhexMarker))
	if at < 0 {
		return false
	}

	for _, b := range head[:at] {
		if b < 0x20 && b != '\r' && b != '\n' && b != '\t' {
			return false
		}
	}
	return true
}

// decode takes a BinHex file back to the data fork and the name it carried
func (d *binhexDecoder) decode(text []uint8) (File, error) {
	stream, err := d.sixBits(text)
	if err != nil {
		return File{}, err
	}
	data := expandRuns(stream)

	if len(data) < 1 {
		return File{}, fmt.Errorf("the BinHex data is empty")
	}
	nameLength := int(data[0])
	if nameLength > binhexMaxName {
		return File{}, fmt.Errorf("the BinHex header has a name of %v characters", nameLength)
	}

	headerLength := 1 + nameLength + binhexFixedLen
	if len(data) < headerLength+2 {
		return File{}, fmt.Errorf("the BinHex data ends inside its header")
	}

	header := data[:headerLength]
	if crc16(header) != binary.BigEndian.Uint16(data[headerLength:]) {
		return File{}, fmt.Errorf("the BinHex header fails its CRC")
	}

	name := macRoman(data[1 : 1+nameLength])
	lengths := header[1+nameLength+1+4+4+2:]
	dataLength := int(binary.BigEndian.Uint32(lengths[0:4]))

	from := headerLength + 2
	if dataLength < 0 || len(data) < from+dataLength+2 {
		return File{}, fmt.Errorf("%v ends inside its data fork", name)
	}

	fork := data[from : from+dataLength]
	if crc16(fork) != binary.BigEndian.Uint16(data[from+dataLength:]) {
		return File{}, fmt.Errorf("the data fork of %v fails its CRC", name)
	}

	return File{Name: name, Data: fork}, nil
}

// sixBits undoes the alphabet, from the colon after the marker to the one
// that closes the data
func (d *binhexDecoder) sixBits(text []uint8) ([]uint8, error) {
	at := bytes.Index(text, []uint8(binhexMarker))
	if at < 0 {
		return nil, fmt.Errorf("there is no BinHex marker")
	}
	start := bytes.IndexByte(text[at+len(binhexMarker):], ':')
	if start < 0 {
		return nil, fmt.Errorf("the BinHex data never starts")
	}

	out := make([]uint8, 0, len(text)*3/4)
	var bits uint32
	var count uint

	for _, c := range text[at+len(binhexMarker)+start+1:] {
		switch c {
		case ':':
			return out, nil
		case '\r', '\n', '\t', ' ':
			continue
		}

		value := d.values[c]
		if value == binhexInvalid {
			return nil, fmt.Errorf("the BinHex data has a %q in it", c)
		}

		bits = bits<<6 | uint32(value)
		count += 6
		if count >= 8 {
			count -= 8
			out = append(out, uint8(bits>>count))
		}
	}

	return nil, fmt.Errorf("the BinHex data is cut short")
}

// expandRuns undoes the run length encoding
func expandRuns(in []uint8) []uint8 {
	out := make([]uint8, 0, len(in))
	var last uint8

	for i := 0; i < len(in); i++ {
		if in[i] != binhexRepeat || i+1 >= len(in) {
			last = in[i]
			out = append(out, last)
			continue
		}

		i++
		count := in[i]
		if count == 0 {
			last = binhexRepeat
			out = append(out, last)
			continue
		}
		for range int(count) - 1 {
			out = append(out, last)
		}
	}

	return out
}

// macRoman turns a name from the machine into one the host can show
func macRoman(name []uint8) string {
	decoded, err := charmap.Macintosh.NewDecoder().Bytes(name)
	if err != nil {
		return string(name)
	}
	return string(decoded)
}
