package appletalk

import (
	"bytes"
	"encoding/binary"
)

/*
NBP, the Name Binding Protocol: how a Macintosh finds a server by name. The
Chooser's AppleShare looks up =:AFPServer@*, anything of type AFPServer in this
zone, by broadcasting a lookup to the names information socket, 2, of every
node; a node with a name that matches answers with where it is.

	0    function on the high nibble, tuple count on the low
	1    NBP id, which the reply carries back
	2    tuples: network (2), node, socket, enumerator, then the
	     object, the type and the zone as Pascal strings

The tuple of a lookup is where to send the reply. On a LocalTalk with no
router the network is zero and the zone is *, and the Macintosh sends a lookup
rather than a broadcast request, but a broadcast request is answered the same
way, as a router would pass it on as a lookup.
*/

const (
	nbpBroadcastRequest = 1
	nbpLookup           = 2
	nbpLookupReply      = 3

	// The wildcards: = for anything, and ≈, $c5 in Mac OS Roman, for
	// anything around what is given
	nbpWildcard        = "="
	nbpPartialWildcard = 0xc5
)

// Name is an NBP entity name: object, type and zone
type Name struct {
	Object string
	Type   string
	Zone   string
}

// registered is a name a node answers to, and the socket it answers with
type registered struct {
	name   [3][]uint8
	socket uint8
}

// nbp answers lookups for the names registered on the node
type nbp struct {
	node  *Node
	names []registered
}

func (n *Node) startNBP() *nbp {
	p := &nbp{node: n}
	n.listen(socketNBP, p.datagram)
	return p
}

// register gives the node a name, in Mac OS Roman
func (p *nbp) register(object []uint8, kind []uint8, socket uint8) {
	p.names = append(p.names, registered{
		name:   [3][]uint8{object, kind, []uint8("*")},
		socket: socket,
	})
}

func (p *nbp) datagram(d Datagram) {
	if d.Type != ddpTypeNBP || len(d.Data) < 2 {
		return
	}
	function, count, id := d.Data[0]>>4, d.Data[0]&0x0f, d.Data[1]
	if (function != nbpLookup && function != nbpBroadcastRequest) || count < 1 {
		return
	}

	tuple, ok := parseTuple(d.Data[2:])
	if !ok {
		return
	}

	for _, r := range p.names {
		if !nameMatches(tuple.name, r.name) {
			continue
		}
		reply := []uint8{nbpLookupReply<<4 | 1, id}
		reply = binary.BigEndian.AppendUint16(reply, 0) // our network
		reply = append(reply, p.node.address, r.socket, 0)
		for _, part := range r.name {
			reply = append(reply, uint8(len(part)))
			reply = append(reply, part...)
		}
		p.node.send(tuple.node, tuple.socket, socketNBP, ddpTypeNBP, reply)
	}
}

// tuple is an NBP tuple: where an entity is, and its name
type tuple struct {
	node   uint8
	socket uint8
	name   [3][]uint8
}

func parseTuple(data []uint8) (tuple, bool) {
	var t tuple
	if len(data) < 5 {
		return t, false
	}
	t.node, t.socket = data[2], data[3]
	rest := data[5:]
	for i := range t.name {
		if len(rest) < 1 || len(rest) < 1+int(rest[0]) {
			return t, false
		}
		t.name[i] = rest[1 : 1+int(rest[0])]
		rest = rest[1+int(rest[0]):]
	}
	return t, true
}

/*
nameMatches compares a looked up name with a registered one, case
insensitively as NBP does. = matches anything, in the object or the type, ≈
anything around the rest of the word, and * or an empty zone is this one.
*/
func nameMatches(wanted [3][]uint8, have [3][]uint8) bool {
	for i := 0; i < 2; i++ {
		if !partMatches(wanted[i], have[i]) {
			return false
		}
	}
	zone := wanted[2]
	return len(zone) == 0 || string(zone) == "*" || equalFold(zone, have[2])
}

func partMatches(wanted []uint8, have []uint8) bool {
	if string(wanted) == nbpWildcard {
		return true
	}
	if at := bytes.IndexByte(wanted, nbpPartialWildcard); at >= 0 {
		prefix, suffix := wanted[:at], wanted[at+1:]
		return len(have) >= len(prefix)+len(suffix) &&
			equalFold(have[:len(prefix)], prefix) &&
			equalFold(have[len(have)-len(suffix):], suffix)
	}
	return equalFold(wanted, have)
}

/*
equalFold compares two names in Mac OS Roman with the letters of ASCII folded,
which is what NBP names in practice are made of. It goes byte by byte: the
names are not UTF-8, and folding them as if they were would take every accented
letter for every other.
*/
func equalFold(a []uint8, b []uint8) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if foldASCII(a[i]) != foldASCII(b[i]) {
			return false
		}
	}
	return true
}

func foldASCII(c uint8) uint8 {
	if c >= 'a' && c <= 'z' {
		return c - 'a' + 'A'
	}
	return c
}
