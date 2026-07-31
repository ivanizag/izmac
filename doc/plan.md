# izmac implementation plan

A Macintosh Plus emulator in Go, built on [iz68000](https://github.com/ivanizag/iz68000),
following the structure and conventions of izapple2.

This merges the earlier `DESIGN.md`, written during iz68000 development, with the frontend
and packaging decisions taken since.

## Scope

- **Machine**: Macintosh Plus, 1 MB RAM (4 MB later), 128 KB ROM.
- **Storage**: SCSI first, then the IWM. Both are done.
- **Timing**: instruction level, not cycle accurate. The memory interface stays shaped so
  it could be tightened later.
- **Pure Go**: no cgo in the core, so the web frontend stays possible.
- **Frontends**: `macebiten` (primary) and `headless` (tests and bring-up), both from day one.
  Ebiten keeps the pure-Go WASM story that motivated writing our own 68000 core; cgo stays
  confined to an optional SDL frontend if one is ever wanted.

## Reference hardware notes

Verified against **Inside Macintosh volume III, chapter 2, "The Macintosh Hardware", pages
III-17 to III-46**, which is the authority for everything below unless said otherwise, plus
the *Guide to the Macintosh Family Hardware* and the reverse-engineered Plus PAL equations.
The chapter documents the 128K and 512K; the Plus differs in RAM size and in having SCSI, but
the video, sound, VIA, disk and clock interfaces are the same.

### Clocks and frame timing

| Quantity | Value |
|---|---|
| CPU clock | 7.8336 MHz |
| Pixel clock | 15.6672 MHz |
| Line rate | 22254.5 Hz (704 pixel clocks per line) |
| CPU cycles per line | **352** |
| Lines per frame | **370** (342 visible + 28 vertical blanking) |
| Frame rate | 60.15 Hz |
| Sound sample rate | 22254.5 Hz (one sample per line) |

The whole machine's timing falls out of one counter: **every 352 CPU cycles is one scanline
tick**. That tick consumes one sound sample and toggles HBlank; line 342 asserts VBlank on
VIA CA1. No cycle accuracy needed, and sound comes out at exactly the right rate for free.
This is the single most important simplification in the design, and it means the boot beep
costs almost nothing once the tick exists.

### Memory map

The 16 MB address space is divided into **four equal 4 MB quarters**: RAM, ROM + SCSI, SCC,
and IWM + VIA. Within each quarter only a few address lines are decoded, so a device's
registers **wrap around and reappear at many addresses**. Decode loosely — the ROM relies on
it. A `switch` on the top two bits selects the quarter, and the devices decode a handful of
bits below that.

| Range | Contents |
|---|---|
| `0x000000`–`0x3FFFFF` | RAM (mirrored), or ROM while the overlay is set |
| `0x400000`–`0x7FFFFF` | ROM (128 KB at `0x400000`, mirrored) and SCSI |
| `0x580000` | SCSI, NCR 5380 |
| `0x600000`–`0x7FFFFF` | RAM while the overlay is set |
| `0x800000`–`0xBFFFFF` | SCC: read `0x800000`–`0x9FFFFF`, write `0xA00000`–`0xBFFFFF` |
| `0xC00000`–`0xFFFFFF` | IWM and VIA |
| `0xC00000`–`0xDFFFFF` | IWM (base `0xDFE1FF`) |
| `0xE00000`–`0xFFFFFF` | VIA (base `0xEFE1FE`, registers 512 bytes apart) |

**The overlay is the one thing to get right first.** VIA port A bit 4, set at reset, maps ROM
over `0x000000`–`0x5FFFFF` — which is where the 68000 fetches the reset SP and PC, exactly
what `iz68000.State.Reset()` does — with RAM up at `0x600000`–`0x7FFFFF`. The boot code jumps
to the real ROM at `0x400000` and clears the bit, and the map becomes the normal one. Get this
wrong and not a single instruction executes.

**The VIA base `$EFE1FE` and the IWM base `$DFE1FF`** are confirmed twice over: the ROM loads
them into A5 and A0 and works from them, and the book gives them as `vBase` and `dBase`.
Registers are 512 bytes apart, indexed by `(address >> 9) & 0xF`. The VIA sits on the **upper**
byte of the data bus and is reached with even byte accesses, the IWM on the **lower** byte with
odd ones; izmac decodes loosely and ignores the distinction.

The VIA register order is the standard 6522 one: B, dirB, dirA, T1C, T1CH, T1L, T1LH, T2C,
T2CH, SR, ACR, PCR, IFR, IER, A. The SCC bases are `$9FFFF8` for reads and `$BFFFF9` for
writes, with offsets aCtl 2, aData 6, bCtl 0, bData 4.

**Addresses must be masked to 24 bits.** The Memory Manager keeps the locked and purgeable
flags in the high byte of master pointers, so a pointer is only valid once that byte is
dropped. iz68000 masks on its side; any address arithmetic in izmac — SCSI transfer
addresses, video base computation, tracing — has to do the same.

### Video and sound buffers

Both live at the top of RAM and are selected by VIA port A bits. Video is 512×342 at one bit
per pixel, 21888 bytes, plain bitmap, no CRTC — **black is one**. Rendering is a blit once per
frame, as cheap as it sounds. For 1 MB (`ramTop = 0x100000`):

| Buffer | Offset from RAM top | 1 MB | 4 MB | Selected by |
|---|---|---|---|---|
| Video main | `-0x5900` | `0x0FA700` | `0x3FA700` | VIA PA6 = 1 |
| Video alternate | `-0xD900` | `0x0F2700` | `0x3F2700` | VIA PA6 = 0 |
| Sound main | `-0x0300` | `0x0FFD00` | `0x3FFD00` | VIA PA3 = 1 |
| Sound alternate | `-0x5F00` | `0x0FA100` | `0x3FA100` | VIA PA3 = 0 |

Apple's published tables for the 1, 2, 2.5 and 4 MB configurations all reduce to these same
four offsets from the top of RAM, so keeping `ramTop` a variable makes 4 MB a flag rather than
a rewrite. The corresponding low memory globals are `ScrnBase`, `SoundBase` and `MemTop` —
useful anchors for the low memory watch.

### The VIA is the hub

A 6522 carrying most of the machine. Everything on it raises **interrupt level 1**:

| Port A | | Port B | |
|---|---|---|---|
| PA0–PA2 | Sound volume | PB0 | RTC data |
| PA3 | Sound buffer select | PB1 | RTC clock |
| PA4 | **Overlay** | PB2 | RTC enable |
| PA5 | Disk head select | PB3 | Mouse switch |
| PA6 | Video page select | PB4 | Mouse X2 |
| PA7 | SCC wait/request | PB5 | Mouse Y2 |
| | | PB6 | HBlank |
| | | PB7 | Sound disable |

All of the above is confirmed by the book, including the polarities: `vPage2` 0 selects the
alternate screen, `vSndPg2` 0 the alternate sound buffer, `vSndEnb` 0 enables sound, `vSW` 0
means the mouse button is down, `vH4` 1 means horizontal blanking. The reset values are
`vAOut $7F` and `vAInit $7B` for port A, `vBOut $87` and `vBInit $07` for port B.

The interrupt flag register is named per device rather than per 6522 pin, and the two views
line up exactly:

| Bit | Device | 6522 source |
|---|---|---|
| 0 | One second interrupt | CA2 |
| 1 | **Vertical blanking** | CA1 |
| 2 | Keyboard data ready | shift register |
| 3 | Keyboard data bit | CB2 |
| 4 | Keyboard clock | CB1 |
| 5 | Timer 2 | T2 |
| 6 | Timer 1 | T1 |
| 7 | Any enabled VIA interrupt | IRQ |

The peripheral control register splits the same way: bit 0 is the vertical blanking edge,
bits 1–3 the one second interrupt, bit 4 the keyboard clock, bits 5–7 the keyboard data.

**The interrupt levels are not what this plan said.** The three sources drive one IPL line
each — the VIA drives IPL0, the SCC IPL1 and the programmer's switch IPL2 — so they combine
arithmetically rather than the switch simply being level 7:

| Vector | Level | Source |
|---|---|---|
| `$64` | 1 | VIA |
| `$68` | 2 | SCC |
| `$6C` | 3 | VIA + SCC |
| `$70` | 4 | Interrupt switch |
| `$74` | 5 | switch + VIA |
| `$78` | 6 | switch + SCC |
| `$7C` | 7 | all three |

So the programmer's switch alone is **level 4**, not level 7. Level 1 for the VIA, which is
what izmac drives, is right.

### SCSI register addressing

Addresses take the form `0x580drn`: **`r`** is the 5380 register number, **`n`** is 0 for reads
and 1 for writes, **`d`** is the DACK signal for the pseudo-DMA path. So registers are 16 bytes
apart, reads and writes are at even and odd addresses of the same register, and the DACK
variants sit `0x200` higher.

| Register | Read | Write |
|---|---|---|
| Current SCSI Data | `0x580000` | — |
| Output Data | — | `0x580001` |
| Output Data (DACK) | — | `0x580201` |
| Initiator Command | `0x580010` | `0x580011` |
| Mode | `0x580020` | `0x580021` |
| Target Command | `0x580030` | `0x580031` |
| Select Enable | — | `0x580041` |
| Bus and Status | `0x580050` | — |
| DMA Transmit Start | — | `0x580051` |
| Input Data | `0x580060` | — |
| Input Data (DACK) | `0x580260` | — |
| Target DMA Receive | — | `0x580061` |
| Reset Parity/Interrupt | `0x580070` | — |
| Initiator DMA Receive | — | `0x580071` |

### The disk status and control registers

The sixteen one-bit status registers of the drive, selected by **CA2, CA1, CA0 and SEL** in
that order from the high bit. CA0 to CA2 and LSTRB come from the IWM, **SEL comes from VIA
port A bit 5**. Page III-35.

| Selector | Register | Meaning |
|---|---|---|
| 0 | DIRTN | head step direction, **1 steps back towards the track 0** |
| 1 | CSTIN | **0 only when a disk is in the drive** |
| 2 | STEP | head stepping; the drive sets it back to 1 after ~12 ms |
| 3 | WRTPRT | 0 whenever the disk is locked |
| 4 | MOTORON | 0 turns the motor on, and only if a disk is in place |
| 5 | TKO | 0 only if the head is at track 0 |
| 7 | TACH | 60 pulses per rotation |
| 8, 9 | RDDATA0, RDDATA1 | data from the lower and upper heads |
| 12 | SIDES | 0 single sided, **1 double sided** |
| 13 | /READY | **0 when the drive is up to speed**, not in the book |
| 14 | DRVIN | **0 if a drive is connected**, floats to 1 if not |
| 15 | NEWINTF | **1 on the 800K drive of the Plus**, not in the book |

**The book's table is the 400K drive and the Plus has the 800K one.** `SonyEqu.a` of the ROM
sources gives the same registers in its own order, CA1, CA0, SEL and CA2, and converting them
lands on the book's table with three differences that matter:

- **DRVIN is 14, not 15.** `DrvExstAdr` is 13 in the driver's order, which is selector 14 here.
  Answering only 15 leaves the ROM hanging on an empty drive queue.
- **15 is a different line altogether**, `NewIntfAdr`, and it is **not active low**. The driver
  reads it once at startup with `MOVEQ #$F,D0 ... SMI.B NewIntf` and takes a one to mean the
  drive regulates its own speed.
- **13 is /READY**, `ReadyAdr`, which `Sony_Seek` polls a thousand times with a millisecond
  between before giving up.

**Reporting the new interface is what makes the speed problem disappear.** With it low, the
driver believes it has a 400K drive and `Sony_MakeSpdTbl` calibrates the motor: it sets a pulse
width through the low bytes of the sound buffer, times fifteen TACH edges four times over at
two different settings, and interpolates a table for the five bands. With it high the whole
routine is `MOVE #$1F40,D0; BRA WakeUp`, an 800 ms wait, and `Sony_SetSpeed` forces the pulse
width to zero. The tachometer still has to move, but nothing measures it.

To read a register: turn Q7 off, turn Q6 on, select the register, and the bit appears in the
**high bit** of `q7L`. Turn Q6 back off afterwards or the Disk Driver will not recognise the
state.

Writing goes through LSTRB with CA1, CA0 and SEL selecting DIRTN, STEP, MOTORON or EJECT — the
registers 0, 2, 4 and 6 of those three bits — and CA2 carrying the value. All of them are
active low except the eject, which happens on a one. LSTRB must be held high at least 1 µs and
under 1 ms, except for an eject which needs half a second.

**The head is not latched.** `Sony_AdrDisk` moves the four lines one at a time, so on the way
from one register to another it passes through others, and two of those are the read data pair
at 8 and 9. Taking the head from the lines as they move picks up a state the driver was only
passing through; it has to be taken when a byte is actually read or written.

### The diskette itself

Group coded recording, six bits carried in eight, verified against `DT_Sony_NiblTbl` and the
read and write engines of `plus/resources/res_drvr_sony.s`.

| | |
|---|---|
| Bit rate | 489.6 kbit/s, so 61200 bytes a second |
| **CPU cycles per disk byte** | **exactly 128** |
| Tracks | 80 a side, 1 or 2 sides |
| Sectors per track | 12, 11, 10, 9, 8 in five bands of sixteen tracks |
| Rotation | 394, 429, 472, 525, 590 rpm over the same bands |
| Sector | 12 tag bytes and 512 of data |

The drive turns slower the further out the head is so that a bit is the same length of track
everywhere, which is why an outer track holds fewer sectors. **None of those speeds has to
appear in the emulation.** A bit lasts the same time in every band, so a byte does, so a track
of twelve sectors simply takes half as long again to go round as one of eight: building every
track out of a fixed **778 bytes per sector** gives the right rotation for all five bands for
free. 12 × 778 × 128 cycles is 394 rpm without anyone saying so.

A sector on the track is a run of self sync, an address field of `D5 AA 96` and five values
closed by `DE AA FF`, a short run of sync, and a data field of `D5 AA AD`, the sector number,
699 nibbles, four of checksum and `DE AA FF`. The address field carries the track split over
two values with the side on the bit 5 of the second, and the low value is the exclusive or of
the other four.

**The data field is scrambled as it is written.** Three running bytes are carried through the
sector and every byte is exclusive ored with one of them before being encoded; the three are
the checksum the driver compares at the end, so one pass does both. The carry into the first
addition of a group is the bit rotated out of the third running byte, and the carry out of each
addition feeds the next. `SONY_RDDATA_CONT_3` is the authority and the sequence is short enough
to copy: `ADD.B D7,D3`, `ROL.B #1,D7`, `EOR.B D7,D2`, `ADDX.B D2,D5`, and so on round the three.

**Self sync is a bit level thing and izmac has no bits.** The real pattern slips a zero bit in
so a shift register finds the byte boundary again. Handing whole bytes over means a run of `$FF`
does the same job: the driver waits for bytes with the top bit set and then looks for a mark,
and `$FF` is a byte and is not a mark. Neither `$D5` nor `$AA` is a nibble, so a mark cannot
turn up inside data and the scan cannot be fooled. `$DE` **is** a nibble, which is why the
closing three bytes are never scanned for, only checked at a known offset.

### Keyboard protocol

Bidirectional serial over the VIA shift register, MSB first, the keyboard owning the clock.
The Mac initiates every exchange. Keyboard-to-host is eight 330 µs clock cycles, host-to-
keyboard eight 400 µs cycles — a timescale so far above the scanline tick that it can be
modelled as instantaneous with a plausible delay, which is the simplification most emulators
make.

| Command | Value | Response |
|---|---|---|
| Inquiry | `$10` | Key transition, or Null `$7B` |
| Instant | `$14` | Key transition, or Null `$7B` |
| Model Number | `$16` | bit 0 = 1, bits 1–3 model 1–8, bits 4–6 next device, bit 7 = another device connected |
| Test | `$36` | ACK `$7D` or NAK `$77` |

**Three of those four values were wrong in an earlier draft of this plan**, taken from a web
source: Instant was `$12`, Model `$14` and Test `$16`. The values above are the book's.

A key transition response is one byte: **bit 7** is the state, 1 for key up and 0 for key
down, **bits 6–1** the code, **bit 0** always 1. The driver strips bit 7 and shifts right one
place, so the code the software sees is `(response & 0x7f) >> 1`.

The sequence matters as much as the values. The Macintosh sends **Model Number first** and
retries it every half second until the keyboard answers. Then it sends **Inquiry every quarter
second**; the keyboard answers with a transition, or with **Null `$7B` after a quarter second**
if nothing happened. If nothing answers within half a second the Macintosh assumes the keyboard
is gone and starts over with Model Number. The Null path is the one to get right — a keyboard
that simply says nothing when idle will send the ROM back to square one forever.

The full table of key-down transition codes for the US and international keyboards and the
keypad is Figure 9, page III-32.

## Sound

There is no sound chip. 370 words at the top of the RAM, one for each scan line of a frame,
are read by the circuit as the beam goes down the screen, which is 22254 samples a second and
costs nothing here because the scan line tick already exists. **Only the high byte of each
word is the sound**; the low one is the speed of the disk motor, which shares the buffer.

The values are unsigned around a middle of 128, so silence is a buffer full of `$80`. A buffer
of zeros is the loudest the machine can go one way, which is the opposite of what a stream of
zeros usually means.

The volume is three bits of the VIA port A, the buffer is chosen by its bit 3, and the bit 7
of the port B enables the sound when it is **zero**. The timer 1 can also invert that bit by
itself when the top bit of the auxiliary control register is set, which is how a tone is made
out of a buffer holding one repeated value without the processor doing anything.

A frontend takes the samples through an `AudioSink` and resamples: `audio.Stream` holds a
queue and interpolates to whatever rate the host device wants. Neither end waits for the
other — running dry holds the last value, which is quieter than a gap, and running over drops
the oldest, so the sound cannot fall progressively behind the picture at full speed.

## Attaching disks

Up to seven disks go on the bus, taking the ids 0 upwards in the order given, since the
Macintosh keeps the id 7 for itself. Files named on the command line without an option are
disk images, as izapple2 takes them, and what each one is comes from the image rather than
from its name: a disk that has been through Apple's formatter starts with a driver descriptor
map, the letters `ER`, and no diskette carries one; failing that, an image of exactly 400K or
800K is a diskette because those are the only sizes the drives of this machine make, and
anything else is a hard disk.

A diskette goes in a drive, the internal one first and then the external. A DiskCopy 4.2 image
says what it is in its own header and is recognised before the size is looked at, since the
file starts with the name of the volume and that could say anything.

A diskette is held whole in memory, 800Kb of sectors and 19Kb of tags at the very most, and the
file is rewritten complete when something changes. That is what makes DiskCopy bearable: its
checksums cover the whole file and would otherwise be recomputed against it on every sector.
The write back happens when the motor stops, which the driver does a few seconds after it has
finished, so the image on the host follows what the Macintosh believes it has saved.

## Package structure

Flat root package with subpackages only where izapple2 has them, filename prefixes carrying
the grouping — matching the `cardXxx.go` / `traceXxx.go` convention.

```
izmac/
  mac.go              type Mac, the machine and its accessors
  macRun.go           run loop, scanline tick, speed control
  command.go          command channel, mirrors izapple2/command.go
  configuration.go    command line flags
  memoryManager.go    address decode, overlay, iz68000.Memory impl, the chips
                      on the map and the low memory globals watch
  rom.go              the revision izmac targets and where to fetch one
  video.go            the frame buffer and drawing it to an image
  via.go              the Mac's VIA wiring, over component/mos6522.go
  keyboard.go         key code table and transition queue
  mouse.go            quadrature generation
  iwm.go              the floppy controller: the soft switches, the status,
                      the data register and the handshake
  iwmDrive.go         a drive: the motor, the head, the lines it reports and
                      the track going past
  sound.go            sound buffer to audio sink
  traceToolbox.go     A-line trap tracing by name
  traceSadMac.go      Sad Mac error code decoding
  component/          the chips, knowing nothing of the machine: the 6522
                      copied from izapple2 and extended, and the clock
  scsi/               the bus, the 5380 and the disks on it
  storage/            the files read off the host: images, their kind, the ROM,
                      the group coded recording and the diskette layout
  frontend/macebiten/
  frontend/headless/
  doc/
```

**On the 6522**: izapple2's `component/mos6522.go` is tested and close, but its own header
says the shift register, the CA/CB control lines and handshaking are not implemented — all
three of which the Mac needs. Recommendation: **copy it into izmac and extend it there**.
Extracting `component` into a shared module is cleaner long term but churns izapple2 for no
immediate gain.

## Things that will bite

- **The overlay at reset**, as above.
- **A-line traps are the entire Toolbox.** Every `$Axxx` opcode vectors through 10, and the
  handler needs the address of the trap itself to read the opcode — which iz68000 already
  stacks correctly. This is a *correctness requirement on the critical path*, not a
  refinement: it must work in M1, before video exists. The tracer sits on top of it.
- **Address errors are load bearing.** The ROM and MacsBug depend on odd word accesses
  faulting. iz68000 raises them from its own `getWord`/`setWord` layer, built out of
  byte-granular `Peek`/`Poke`. **Corollary: do not add word or long access to the `Memory`
  interface as an optimisation** — it would move the odd-address check into izmac and is the
  most likely way to silently break this.
- **MOVE from SR is not privileged on the 68000**, it became so on the 68010. Macintosh code
  relies on the 68000 behaviour and iz68000 has this right. Do not "fix" it.
- **Sloppy address decoding**: mirror everything, do not decode exactly.
- **Known iz68000 gaps**, both unobservable by a working Macintosh, so do not chase them: the
  flags left undefined by the manual on the decimal instructions, and the exact cycle count of
  an instruction aborted by an address error.

## Milestones

Each has an exit criterion the headless frontend can assert, so progress is testable rather
than eyeballed.

**Progress**: M0 is done. M1 runs the real ROM: it gets through the power on tests and 74
million cycles before stopping, with no Sad Mac drawn. Two findings from that run:

1. **The VIA and IWM base addresses are confirmed** by the ROM's own code, as above.
2. **The ROM talks to the IWM before anything else works.** It stopped in a loop at
   `$400104`–`$400126` running the chip presence handshake — disable the drive, write `$1f`
   to the mode register, read it back through the status register — which a stub answering
   `$ff` to everything can never satisfy. "The IWM is the long tail, not the way in" is still
   true of *floppy emulation*, but a stub answering plausibly was needed here and not in M6.
   `iwm.go` now implements the sixteen soft switches, the mode register and the status, and
   the handshake passes.

3. **The vertical blanking on CA1 was the next blocker**, at `$40032c`. The 6522 now has the
   CA/CB lines as inputs, with one detail that matters: reaching the port A through register
   1 clears the control line flags and through register 15 does not, which is exactly why the
   Macintosh uses register 15. With VBlank driven from the scan line tick the ROM sizes the
   RAM correctly and runs its memory tests.
4. **A word that decodes to nothing is an illegal instruction, not a reason to stop.** The
   ROM finds out which processor it is running on by executing a MOVEC, `$4e7b`, and catching
   the exception. iz68000 panicked on unknown opcodes; it now raises vector 4.

**It boots, and it can be used.** The ROM runs its power on tests, sizes the memory,
initialises QuickDraw and the clock, finds the SCSI bus, loads the driver off the disk,
mounts the volume and **reaches the Finder**. The keyboard and the mouse work, so the desktop
answers. Three tests assert it end to end and cost a second each: the Finder appearing, the
pointer moving where the mouse is pushed, and a key press reaching KeyMap.

Twelve bugs stood between the first instruction and a usable machine. Eleven were mine, and
every one was found by watching the machine rather than reasoning about it.

1. **`$030a` is `DrvQHdr + 2`**, the head of the drive queue, and the ROM hangs on purpose
   when it is empty.
2. **A word that decodes to nothing is an illegal instruction, not a reason to stop.** iz68000
   panicked on the MOVEC the ROM uses to identify the processor.
3. **`$4007d4` was never a hang.** The halt detector counted instructions, so a wait for the
   tick counter looked stuck after a fifth of a frame.
4. **The ROM decides whether the machine has SCSI by comparing `$420000` with `$440000`.**
   Mirroring the ROM over the whole quarter made those equal. The window is 256Kb.
5. **The clock had to work first**, or the ROM never learns the boot device.
6. **The arbitration bits of the initiator command register are status on a read.**
7. **Selection happens after the select line goes up, not on its edge.**
8. **Request and acknowledge interlock.** A request that never falls makes the driver resend
   the same byte.
9. **The driver reads fewer bytes than it asked for and drains the rest** through the data
   register and the acknowledge.
10. **The SCC pointer is three bits wide.** A write of `$0f` is the point high command plus
    register 7, which selects register 15, and the mouse interrupt handler reads register 15
    back to tell a carrier detect change from anything else. Masking the pointer to three bits
    put the enable in register 7 and answered zero from register 15.
11. **Reset external status is per channel.** Clearing the whole chip loses every transition
    of the axis whose turn it was not, which shows as a pointer that crawls.
12. **A byte over the keyboard wire is two events, not one.** The chip reports that the eight
    bits written to it have gone out, which is what tells the ROM to turn the shift register
    around and listen, and only then does the answer arrive. Delivering only the answer leaves
    the ROM still waiting to finish sending, and it asks again for ever.

One of these was in an API rather than in the hardware: `RunFrames` reset the machine on every
call, so a test that booted and then did something was really booting twice. It now carries on
unless `Reset()` is asked for.

**Three more came out of using the mouse rather than testing it**, which is worth remembering:
the end to end test asserted the pointer moved the right way and passed, while the mouse was
still unusable in the hand.

13. **The quadrature has to stand still until the port is read, not until the interrupt is
    cleared.** The dispatch at `EXTSTATUS_RESET` resets the external status of the SCC and
    only then jumps to the mouse handler that reads the VIA, so releasing the axis on the
    reset lets a scan line tick land in the gap and move the level out from under the
    handler. Every interrupt still arrives and is answered — 60 of 61 in a measured run —
    and about half are simply read with the wrong level, which is a mouse that goes nowhere
    in particular. The axis now holds from the edge until the port is read.
14. **The carrier detect inputs are active low.** The edge the ROM calls positive is the
    falling edge of the signal the mouse sends. Holding the phase still exposed this, because
    until then the sampling error was cancelling the inversion often enough to look right.
15. **A pixel is one transition of the interrupt line, not one phase.** The line only changes
    on two of the four phases, so a phase per pixel delivers half the movement asked for and
    the pointer feels heavy.

The lesson is the one this project keeps teaching in different clothes: a test that asserts a
direction is not a test that the thing works. The tests now pin the transition count per pixel
and that held movement is postponed rather than dropped.

The ROM disassembly at `../macdocs/mac_rom` gave four of these directly and confirmed a fifth.
`plus/toolbox/scsi_mgr.s` documents the transfer engine and the drain, `plus/os/file_mgr_cache.s`
shows the probe asking for 256 bytes of a block, `plus/hw/interrupts.s` shows the mouse handler
reading register 15, and `plus/boot/vectors.s` states the `$420000` test outright.

**The diskette drives work, both of them, in both directions.** The machine reads, writes and
formats: it initializes a blank image of the right size into a Macintosh volume and mounts it,
which is the end to end test. Almost all of it was read off `include/sonyequ.inc` and
`plus/resources/res_drvr_sony.s` rather than worked out, and the two places it was worked out
instead are two of the three things that went wrong.

1. **The new interface line, selector 15, is what decides everything else.** Reported low, the
   driver believes it has a 400K drive and goes off to measure the rotation on the tachometer
   and correct it with a pulse width through the sound buffer. Reported high it waits 800 ms and
   gets on with it. One bit is the difference between emulating a speed servo and not.
2. **The second head is at selector 9, not 10.** An arithmetic slip converting `SonyEqu.a`'s
   order, and the book had it right all along. It showed up as the far side of a track reading
   as the near one, which is the kind of thing only a test that compares against the image
   catches: every sector read fine, they were simply the wrong sectors.
3. **The head must not be latched when the phase lines move.** `Sony_AdrDisk` sets the four
   lines one at a time and passes through the read data pair on the way to somewhere else.

The number worth remembering is that **a disk byte is exactly 128 processor cycles**, which
falls out of 489.6 kbit/s against 7.8336MHz and makes the whole thing a counter. The one worth
remembering after that is **778 bytes per sector**, which gets all five rotation speeds right
without any of them being written down.

## Worth building early

Bring-up tooling, all of it cheap and all of it paying for itself many times over:

- **The Toolbox trap tracer**, printing every `$Axxx` call by name. The difference between
  "Sad Mac, no idea why" and "it died in InitZone" is one afternoon of work.
- **The Sad Mac decoder** (M1).
- **A low memory globals watch**.
- **A MacsBug-style disassembly view**, which iz68000 already supports through
  `DisasmInstruction`.
- **The SCSI CDB trace** (M4).

## Open questions

The three that blocked M1 are resolved above: target ROM v3 `0x4D1F8172`, PRAM in the working
directory, Sad Mac codes in `D7`/`D6`. What remains:

Inside Macintosh volume III settled the VIA base and register order, the keyboard commands and
the Null timeout, the SCC bases, the disk status table, the RTC protocol and the interrupt
levels. What is left:

1. **Why `Ticks` at `$016a` does not advance.** The VIA raises its interrupt correctly, the
   flag is IFR bit 1 as the book says, the vector at `$64` is right, but the counter never
   moves and the ROM waits for it at `$4007d4`. This is the live bug.
2. **The IWM status register bit layout.** That bit 5 reports the drive enable and the low
   five bits read back the mode was worked out from the ROM's own code, not from a
   datasheet. It works; the rest of the bits are unknown. The same goes for the handshake
   register: the top bit paces a write and the one below it reports an underrun that izmac
   cannot have.
3. ~~**Selector 10 of the drive status lines**~~, which was an arithmetic slip: converting
   `SonyEqu.a`'s CA1, CA0, SEL, CA2 order correctly puts the second head at **9**, exactly where
   the book has it. What the Plus really polls and the book does not list are 13 and 15, above.

## References

- ***Inside Macintosh* volume III, chapter 2, "The Macintosh Hardware", III-17 to III-46** —
  the authority for the video, sound, SCC, mouse, keyboard, disk, clock and VIA interfaces,
  with every constant in the summary at III-43. Read it before guessing at any of them.
- *Guide to the Macintosh Family Hardware*, 2nd ed. — the Plus specifics, SCSI and the memory
  map
- *Inside Macintosh*, volumes I and II
- The Macintosh Plus schematics
- [Mini vMac](https://www.gryphel.com/c/minivmac/) — small, focused on exactly this machine,
  the closest thing to a reference implementation
- The `mac128` driver of MAME
