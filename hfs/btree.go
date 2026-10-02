package hfs

import (
	"encoding/binary"
	"sort"
)

/*
The two B-trees of the volume, built whole rather than by inserting into them.

Every node is 512 bytes: a descriptor, the records from the front, and from the
back a list of where each record starts, with one more entry for where the free
space starts.

	0  4  the next node at the same level
	4  4  the one before it
	8  1  the type: index $00, header $01, leaf $ff
	9  1  the level, one for the leaves
	10 2  how many records

The first node is the header, which says where the root is and carries a map
of the nodes in use. The leaves hold the records in key order and are linked
from first to last; the index nodes above them hold the first key of every
node below, padded to the longest a key can be, and its node number.

The catalog has a record for each file and folder, keyed by the folder it is
in and its name, and a thread for each folder, keyed by the folder itself and
an empty name, that leads from a folder id back to its name. The extents
B-tree would hold the extents of fragmented forks, and a volume built here has
none.
*/

const (
	nodeDescriptorSize = 14
	headerRecordSize   = 106
	reservedRecordSize = 128
	mapRecordSize      = 256

	// indexRecordSize is a padded key and a node number
	indexRecordSize = 1 + catalogKeyLength + 4

	folderRecordSize = 70
	fileRecordSize   = 102
	threadRecordSize = 46
)

// catalogRecord is a key, the folder and the name, and what goes with it
type catalogRecord struct {
	parentID uint32
	name     []uint8
	body     []uint8
}

// bytes is the record as it goes in a leaf, the key padded to an even length
func (r *catalogRecord) bytes() []uint8 {
	key := leafKey(r.parentID, r.name)
	if len(key)%2 != 0 {
		key = append(key, 0)
	}
	return append(key, r.body...)
}

// leafKey is a catalog key as long as its name needs
func leafKey(parentID uint32, name []uint8) []uint8 {
	key := make([]uint8, 7+len(name))
	key[0] = uint8(6 + len(name))
	binary.BigEndian.PutUint32(key[2:], parentID)
	key[6] = uint8(len(name))
	copy(key[7:], name)
	return key
}

// indexKey is a catalog key padded to the longest one, as index nodes keep it
func indexKey(parentID uint32, name []uint8) []uint8 {
	key := make([]uint8, 1+catalogKeyLength)
	copy(key, leafKey(parentID, name))
	key[0] = catalogKeyLength
	return key
}

/*
buildCatalog lays out the catalog B-tree, in nodes. The total is how many nodes
the file has room for, the ones past the tree being free; zero asks only how
many the tree takes.
*/
func (v *volume) buildCatalog(total uint32) [][]uint8 {
	records := v.catalogRecords()
	sort.Slice(records, func(i, j int) bool {
		a, b := records[i], records[j]
		if a.parentID != b.parentID {
			return a.parentID < b.parentID
		}
		return v.order.compare(a.name, b.name) < 0
	})

	// The leaves, filled one after the other
	var leaves [][][]uint8
	var firstOfLeaf []*catalogRecord
	var current [][]uint8
	room := 0
	for _, r := range records {
		b := r.bytes()
		if len(current) == 0 || room < len(b)+2 {
			current = nil
			room = nodeSize - nodeDescriptorSize - 2
			leaves = append(leaves, nil)
			firstOfLeaf = append(firstOfLeaf, r)
		}
		current = append(current, b)
		leaves[len(leaves)-1] = current
		room -= len(b) + 2
	}

	nodes := [][]uint8{nil} // the header, written last
	for i, recordsOfLeaf := range leaves {
		number := uint32(len(nodes))
		var flink, blink uint32
		if i > 0 {
			blink = number - 1
		}
		if i < len(leaves)-1 {
			flink = number + 1
		}
		nodes = append(nodes, newNode(nodeLeaf, 1, recordsOfLeaf, flink, blink))
	}

	// The index nodes, a level at a time until one node holds the level
	type below struct {
		number uint32
		key    []uint8
	}
	level := make([]below, len(leaves))
	for i, r := range firstOfLeaf {
		level[i] = below{number: uint32(1 + i), key: indexKey(r.parentID, r.name)}
	}

	depth := uint16(1)
	root := uint32(1)
	perIndex := (nodeSize - nodeDescriptorSize - 2) / (indexRecordSize + 2)
	for len(level) > 1 {
		depth++
		var next []below
		first := uint32(len(nodes))
		count := (len(level) + perIndex - 1) / perIndex
		for i := 0; i < count; i++ {
			group := level[i*perIndex : min(len(level), (i+1)*perIndex)]
			indexRecords := make([][]uint8, len(group))
			for j, b := range group {
				indexRecords[j] = binary.BigEndian.AppendUint32(append([]uint8(nil), b.key...), b.number)
			}

			number := first + uint32(i)
			var flink, blink uint32
			if i > 0 {
				blink = number - 1
			}
			if i < count-1 {
				flink = number + 1
			}
			nodes = append(nodes, newNode(nodeIndex, uint8(depth), indexRecords, flink, blink))
			next = append(next, below{number: number, key: group[0].key})
		}
		level = next
		root = first
	}

	used := uint32(len(nodes))
	if total < used {
		total = used
	}
	nodes[0] = headerNode(treeHeader{
		depth:     depth,
		root:      root,
		records:   uint32(len(records)),
		firstLeaf: 1,
		lastLeaf:  uint32(len(leaves)),
		keyLength: catalogKeyLength,
		nodes:     total,
		used:      used,
	})

	for uint32(len(nodes)) < total {
		nodes = append(nodes, make([]uint8, nodeSize))
	}
	return nodes
}

// catalogRecords makes the records of every file and folder on the volume
func (v *volume) catalogRecords() []*catalogRecord {
	var records []*catalogRecord

	var walk func(folder *folderEntry)
	walk = func(folder *folderEntry) {
		records = append(records,
			&catalogRecord{parentID: folder.parentID, name: folder.name, body: folderRecord(folder)},
			&catalogRecord{parentID: folder.id, body: threadRecord(folder)})
		for _, f := range folder.files {
			records = append(records,
				&catalogRecord{parentID: f.parentID, name: f.name,
					body: fileRecord(f, v.allocationSize)})
		}
		for _, child := range folder.folders {
			walk(child)
		}
	}
	walk(v.root)

	return records
}

/*
folderRecord is what the catalog keeps of a folder:

	 0  1  1, a folder
	 2  2  flags
	 4  2  how many files and folders it holds
	 6  4  its id
	10  4  created
	14  4  modified
	18  4  backed up
	22 16  Finder information: the window, flags, where the icon is, the view
	38 16  more Finder information
	54 16  reserved

The window is given a size that fits the screen of a Plus. The rest of the
Finder information is left for the Finder, which places a folder it has not
seen before itself.
*/
func folderRecord(folder *folderEntry) []uint8 {
	body := make([]uint8, folderRecordSize)
	body[0] = recordFolder
	binary.BigEndian.PutUint16(body[4:], uint16(len(folder.files)+len(folder.folders)))
	binary.BigEndian.PutUint32(body[6:], folder.id)
	modified := macTime(folder.modified)
	binary.BigEndian.PutUint32(body[10:], modified)
	binary.BigEndian.PutUint32(body[14:], modified)

	// The window: top, left, bottom, right
	binary.BigEndian.PutUint16(body[22:], 60)
	binary.BigEndian.PutUint16(body[24:], 40)
	binary.BigEndian.PutUint16(body[26:], 260)
	binary.BigEndian.PutUint16(body[28:], 440)
	return body
}

// threadRecord leads from a folder's id to its parent and its name
func threadRecord(folder *folderEntry) []uint8 {
	body := make([]uint8, threadRecordSize)
	body[0] = recordFolderThread
	binary.BigEndian.PutUint32(body[10:], folder.parentID)
	body[14] = uint8(len(folder.name))
	copy(body[15:], folder.name)
	return body
}

/*
fileRecord is what the catalog keeps of a file:

	 0  1  2, a file
	 2  1  flags
	 3  1  type, always zero
	 4 16  Finder information: type, creator, flags, where the icon is, the folder
	20  4  its id
	24  2  the first block of the data fork, unused
	26  4  the length of the data fork
	30  4  the space it takes
	34  2  the first block of the resource fork, unused
	36  4  the length of the resource fork
	40  4  the space it takes
	44  4  created
	48  4  modified
	52  4  backed up
	56 16  more Finder information
	72  2  clump size
	74 12  the extents of the data fork
	86 12  the extents of the resource fork
	98  4  reserved
*/
func fileRecord(f *fileEntry, allocationSize uint32) []uint8 {
	body := make([]uint8, fileRecordSize)
	body[0] = recordFile
	copy(body[4:], f.file.Type[:])
	copy(body[8:], f.file.Creator[:])
	binary.BigEndian.PutUint16(body[12:], f.file.Flags&^(hasBeenInited|isOnDesk))
	binary.BigEndian.PutUint32(body[20:], f.id)

	binary.BigEndian.PutUint32(body[26:], uint32(len(f.file.Data)))
	binary.BigEndian.PutUint32(body[30:], f.data.count*allocationSize)
	binary.BigEndian.PutUint32(body[36:], uint32(len(f.file.Resource)))
	binary.BigEndian.PutUint32(body[40:], f.resource.count*allocationSize)
	modified := macTime(f.file.Modified)
	binary.BigEndian.PutUint32(body[44:], modified)
	binary.BigEndian.PutUint32(body[48:], modified)

	// Before the forks have their places these are zeros, which take the
	// same room in the record
	putExtents(body[74:], f.data)
	putExtents(body[86:], f.resource)

	return body
}

// newNode lays out a node with its records
func newNode(kind uint8, height uint8, records [][]uint8, flink uint32, blink uint32) []uint8 {
	node := make([]uint8, nodeSize)
	binary.BigEndian.PutUint32(node[0:], flink)
	binary.BigEndian.PutUint32(node[4:], blink)
	node[8] = kind
	node[9] = height
	binary.BigEndian.PutUint16(node[10:], uint16(len(records)))

	at := nodeDescriptorSize
	for i, r := range records {
		binary.BigEndian.PutUint16(node[nodeSize-2*(i+1):], uint16(at))
		copy(node[at:], r)
		at += len(r)
	}
	binary.BigEndian.PutUint16(node[nodeSize-2*(len(records)+1):], uint16(at))

	return node
}

// treeHeader is what the header node says of a B-tree
type treeHeader struct {
	depth     uint16
	root      uint32
	records   uint32
	firstLeaf uint32
	lastLeaf  uint32
	keyLength uint16
	nodes     uint32
	used      uint32
}

/*
headerNode is the first node of a B-tree: the header record, a reserved one,
and the map of the nodes in use

	 0  2  depth
	 2  4  the root node
	 6  4  how many leaf records
	10  4  the first leaf
	14  4  the last leaf
	18  2  the node size
	20  2  the longest key
	22  4  how many nodes
	26  4  how many of them are free
*/
func headerNode(h treeHeader) []uint8 {
	header := make([]uint8, headerRecordSize)
	binary.BigEndian.PutUint16(header[0:], h.depth)
	binary.BigEndian.PutUint32(header[2:], h.root)
	binary.BigEndian.PutUint32(header[6:], h.records)
	binary.BigEndian.PutUint32(header[10:], h.firstLeaf)
	binary.BigEndian.PutUint32(header[14:], h.lastLeaf)
	binary.BigEndian.PutUint16(header[18:], nodeSize)
	binary.BigEndian.PutUint16(header[20:], h.keyLength)
	binary.BigEndian.PutUint32(header[22:], h.nodes)
	binary.BigEndian.PutUint32(header[26:], h.nodes-h.used)

	nodeMap := make([]uint8, mapRecordSize)
	for i := uint32(0); i < h.used; i++ {
		nodeMap[i/8] |= 0x80 >> (i % 8)
	}

	return newNode(nodeHeader, 0, [][]uint8{
		header, make([]uint8, reservedRecordSize), nodeMap,
	}, 0, 0)
}

// emptyTree is a B-tree with nothing in it, the header node and free ones
func (v *volume) emptyTree(e extent, keyLength uint16) []uint8 {
	nodes := e.count * v.allocationSize / nodeSize
	return headerNode(treeHeader{keyLength: keyLength, nodes: nodes, used: 1})
}
