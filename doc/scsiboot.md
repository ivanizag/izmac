# How the Plus ROM boots off the SCSI bus

Everything here was read out of the 'Loud Harmonicas' ROM izmac targets,
checksum `0x4d1f8172`, by disassembling it with izmac itself:

```bash
go run ./frontend/headless -disasm 0x407d62 -disasmcount 60
```

The addresses are that ROM's. Another revision will have moved them, but the
shape of the thing is the same.

## The scan

`$407d40` is the entry. It walks the SCSI ids from 6 down to 0, skipping any
whose bit is already set in `($b2e).W`, and calls `$407d62` for each.

## One target

`$407d62` is where a disk is looked at, with the id in D5.

```
SUBA.L  #$100,A7        a 256 byte buffer on the stack
MOVEA.L A7,A2           A2 is where the read lands
MOVEQ   #0,D3           D3 is the block
MOVEQ   #1,D2           D2 is how many
MOVE.W  #$100,D4        D4 is the bytes per block for this read
BSR     $407dcc         read
CMPI.W  #$4552,(A7)     'ER', or give up
```

Then the driver descriptor map is read out of that buffer:

| offset | field | what it is |
|---|---|---|
| 0 | sbSig | `'ER'`, `0x4552` |
| 2 | sbBlkSize | block size, taken as the size for the driver read |
| 0x10 | sbDrvrCount | how many descriptors follow |
| 0x12 | first descriptor | eight bytes each |

A descriptor is `ddBlock` (long), `ddSize` (word, in blocks) and `ddType`
(word). The ROM walks them for the first with `ddType == 1` and gives up if
there is none.

## Loading the driver

```
MOVE.L  (A0),D3         ddBlock
MOVE.W  ($4,A0),D2      ddSize
MOVE.L  D4,D0
MULU.W  D2,D0           sbBlkSize * ddSize
_NewPtrSys              $a51e, so the system heap is up by now
MOVEA.L A0,A3           A3 is the driver
MOVEA.L A0,A2
BSR     $407dcc         read the driver into it
MOVEQ   #1,D3           then block 1
MOVEQ   #1,D2
MOVE.W  #$100,D4
MOVEA.L A7,A2
BSR     $407dcc
MOVEA.L A2,A0
JSR.L   (A3)            into the driver
```

So the driver is entered **at its first byte**, which has to be code and not
a `DRVR` header. On entry:

- **A3** the driver, where it was loaded
- **A0** a 256 byte buffer holding block 1 of the disk, on the ROM's stack
- **A2** the same buffer
- **D5** the SCSI id
- supervisor mode, and the driver is expected to `RTS`

Note the ROM never looks at the partition map itself. It reads block 1 and
hands it over, and finding the volume is the driver's problem.

## Reading a block

`$407dcc` is worth copying rather than reinventing. It takes D3 as the block,
D2 as the count, D4 as the bytes in one, A2 as the buffer, and builds the
command at `($9fa).W`:

```
MOVE.B  #$8,(A0)+       READ(6)
SWAP    D3
ANDI.B  #$1f,D3
MOVE.B  D3,(A0)+        the top five bits of the block
SWAP    D3
MOVE.W  D3,(A0)+        and the low sixteen
MOVE.B  D2,(A0)+        the count
CLR.B   (A0)+           control
```

The transfer itself goes through `_SCSIDispatch`, trap **`$a815`**, Pascal
style: room for the result word is made once, then each call pushes its
arguments and a selector and leaves the result in that same slot.

| selector | call |
|---|---|
| 1 | SCSIGet |
| 2 | SCSISelect, target id word |
| 3 | SCSICmd, command pointer and length |
| 5 | SCSIRead, TIB pointer |
| 4 | SCSIComplete, status pointer, message pointer, timeout long |

The TIB is two ten byte entries, a word opcode and two longs:

```
MOVE.W  #1,(A0)+        scInc
MOVE.L  A2,(A0)+        the buffer
MOVE.L  D4,(A0)+        the byte count
MOVE.W  #7,(A0)+        scStop
```

## Registering a drive

`$402558` is the tail of AddDrive: `MOVE.L D0,($6,A0)` puts the drive number
and the reference number into the queue element, and then it is enqueued on
the drive queue at `($308).W` through the routine at `$4119ca`, called with
the element in A0 and the header in A1.

The unit table is reached through `UTableBase` at `($11c).W` with
`UnitNtryCnt` at `($1d2).W`. `$4021ac`, `$402216` and `$40250c` are the
places to read for how the ROM walks it.

## What is not written down here yet

The `DRVR` side. A driver of our own still needs its header laid out the way
this ROM's Device Manager reads it, an entry in the unit table pointing at a
device control entry, and the return convention for Prime, none of which have
been read out of the ROM yet.
