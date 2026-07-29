# izmac implementation plan

A Macintosh Plus emulator in Go, built on [iz68000](https://github.com/ivanizag/iz68000),
following the structure and conventions of izapple2.

This merges the earlier `DESIGN.md`, written during iz68000 development, with the frontend
and packaging decisions taken since.

## Scope

- **Machine**: Macintosh Plus, 1 MB RAM (4 MB later), 128 KB ROM.
- **Storage**: SCSI first. The IWM is the long tail, not the way in.
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
| 0 | DIRTN | head step direction |
| 1 | CSTIN | **0 only when a disk is in the drive** |
| 2 | STEP | head stepping; the drive sets it back to 1 after ~12 ms |
| 3 | WRTPRT | 0 whenever the disk is locked |
| 4 | MOTORON | 0 turns the motor on, and only if a disk is in place |
| 5 | TKO | 0 only if the head is at track 0 |
| 7 | TACH | 60 pulses per rotation |
| 8, 9 | RDDATA0, RDDATA1 | data from the lower and upper heads |
| 12 | SIDES | 0 single sided, **1 double sided** |
| 15 | DRVIN | **0 if a drive is connected**, floats to 1 if not |

To read one: turn Q7 off, turn Q6 on, select the register, and the bit appears in the **high
bit** of `q7L`. Turn Q6 back off afterwards or the Disk Driver will not recognise the state.

Writing goes through LSTRB with CA1, CA0 and SEL selecting DIRTN, STEP, MOTORON or EJECT and
CA2 carrying the value; LSTRB must be held high at least 1 µs and under 1 ms, except for an
eject which needs half a second.

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

A diskette is reported and set aside rather than quietly dropped. The drives are not emulated
yet, and a file that vanishes without a word is worse than one refused.

## Package structure

Flat root package with subpackages only where izapple2 has them, filename prefixes carrying
the grouping — matching the `cardXxx.go` / `traceXxx.go` convention.

```
izmac/
  mac.go              type Mac, the machine and its accessors
  macRun.go           run loop, scanline tick, speed control
  command.go          command channel, mirrors izapple2/command.go
  configuration.go    command line flags
  memoryManager.go    address decode, overlay, iz68000.Memory impl
  rom.go              ROM loading and identification by checksum
  video.go            screen.VideoSource implementation
  via.go              the Mac's VIA wiring, over component/mos6522.go
  keyboard.go         key code table and transition queue
  mouse.go            quadrature generation
  scc.go              85C30, minimal: mouse DCD interrupts and serial stubs
  iwm.go              stub
  sound.go            sound buffer to audio sink
  scsi5380.go         NCR 5380 initiator side
  scsiTarget.go       direct-access target, the SCSI command set
  traceToolbox.go     A-line trap tracing by name
  traceSadMac.go      Sad Mac error code decoding
  traceLowMem.go      low memory globals watch
  component/          the chips, knowing nothing of the machine: the 6522
                      copied from izapple2 and extended, and the clock
  screen/             snapshot to *image.RGBA
  storage/            block device, shaped after izapple2/storage.BlockDisk
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

A diskette is reported and set aside rather than quietly dropped. The drives are not emulated
yet, and a file that vanishes without a word is worse than one refused.

## Package structure

Flat root package with subpackages only where izapple2 has them, filename prefixes carrying
the grouping — matching the `cardXxx.go` / `traceXxx.go` convention.

```
izmac/
  mac.go              type Mac, the machine and its accessors
  macRun.go           run loop, scanline tick, speed control
  command.go          command channel, mirrors izapple2/command.go
  configuration.go    command line flags
  memoryManager.go    address decode, overlay, iz68000.Memory impl
  rom.go              ROM loading and identification by checksum
  video.go            screen.VideoSource implementation
  via.go              the Mac's VIA wiring, over component/mos6522.go
  keyboard.go         key code table and transition queue
  mouse.go            quadrature generation
  scc.go              85C30, minimal: mouse DCD interrupts and serial stubs
  iwm.go              stub
  sound.go            sound buffer to audio sink
  scsi5380.go         NCR 5380 initiator side
  scsiTarget.go       direct-access target, the SCSI command set
  traceToolbox.go     A-line trap tracing by name
  traceSadMac.go      Sad Mac error code decoding
  traceLowMem.go      low memory globals watch
  component/          the chips, knowing nothing of the machine: the 6522
                      copied from izapple2 and extended, and the clock
  screen/             snapshot to *image.RGBA
  storage/            block device, shaped after izapple2/storage.BlockDisk
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

**It boots.** The ROM runs its power on tests, sizes the memory, initialises QuickDraw and
the clock, detects the SCSI bus, scans it, finds the disk, reads the driver descriptor map,
loads the driver from the blocks it points at, mounts the volume and **reaches the Finder**:
menu bar, mouse cursor, the volume icon and the trash. `e2e_finder_test.go` asserts it and
takes about a second.

Nine bugs stood between the first instruction and that desktop. All but one were mine, and
every one was found by watching the machine rather than reasoning about it.

1. **`$030a` is `DrvQHdr + 2`**, the head of the drive queue, and the ROM hangs on purpose
   when it is empty. Asserting the IWM drive installed line put a drive in it.
2. **A word that decodes to nothing is an illegal instruction, not a reason to stop.** The
   ROM identifies the processor by executing a MOVEC and catching the exception. iz68000
   panicked; it now raises vector 4.
3. **`$4007d4` was never a hang.** The halt detector counted instructions, so a wait for the
   tick counter looked stuck after a fifth of a frame. It counts cycles now.
4. **The ROM decides whether the machine has SCSI by comparing `$420000` with `$440000`.**
   Mirroring the ROM over the whole quarter made those equal, so it concluded there was no
   SCSI and never scanned the bus. The window is 256Kb, the size the sockets take.
5. **The clock had to work first.** Without an answer from it the ROM never learns the boot
   device.
6. **The arbitration bits of the initiator command register are status on a read**, not the
   values written, so the driver asked for the bus and never won it.
7. **Selection happens after the select line goes up, not on its edge.** Sampling the data
   bus at the edge sees only the initiator's own id.
8. **Request and acknowledge interlock.** A request that never falls leaves the driver
   waiting and its repeated acknowledges resend the same byte, which is what a descriptor
   block reading `08 08 08 08 08 08` looks like.
9. **The driver reads fewer bytes than it asked for and drains the rest.** Probing a disk it
   asks for a whole block and takes the first 256 bytes, all of the descriptor map it wants,
   then bit buckets the remainder a byte at a time through the data register and the
   acknowledge. A target that does not move on those acknowledges never reaches the status
   phase and the driver resets the bus.

The last four came out of the ROM disassembly at `../macdocs/mac_rom`, which is worth reaching
for early: `plus/toolbox/scsi_mgr.s` documents the transfer engine and the drain, and
`plus/os/file_mgr_cache.s` shows the probe asking for 256 bytes of a block in as many words.
`plus/boot/vectors.s` states the `$420000` against `$440000` test outright.

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

A diskette is reported and set aside rather than quietly dropped. The drives are not emulated
yet, and a file that vanishes without a word is worse than one refused.

## Package structure

Flat root package with subpackages only where izapple2 has them, filename prefixes carrying
the grouping — matching the `cardXxx.go` / `traceXxx.go` convention.

```
izmac/
  mac.go              type Mac, the machine and its accessors
  macRun.go           run loop, scanline tick, speed control
  command.go          command channel, mirrors izapple2/command.go
  configuration.go    command line flags
  memoryManager.go    address decode, overlay, iz68000.Memory impl
  rom.go              ROM loading and identification by checksum
  video.go            screen.VideoSource implementation
  via.go              the Mac's VIA wiring, over component/mos6522.go
  keyboard.go         key code table and transition queue
  mouse.go            quadrature generation
  scc.go              85C30, minimal: mouse DCD interrupts and serial stubs
  iwm.go              stub
  sound.go            sound buffer to audio sink
  scsi5380.go         NCR 5380 initiator side
  scsiTarget.go       direct-access target, the SCSI command set
  traceToolbox.go     A-line trap tracing by name
  traceSadMac.go      Sad Mac error code decoding
  traceLowMem.go      low memory globals watch
  component/          the chips, knowing nothing of the machine: the 6522
                      copied from izapple2 and extended, and the clock
  screen/             snapshot to *image.RGBA
  storage/            block device, shaped after izapple2/storage.BlockDisk
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

**Where it is now**: the ROM boots, draws the desktop with the blinking question mark floppy,
initialises the clock, detects the SCSI bus, scans it, selects the disk and **issues a correct
`READ(6)` of block 0**, the Driver Descriptor Map. The transfer of the data back does not
complete, so the driver resets the bus and tries again. That is the next thread.

Getting there took five bugs, four of them mine and every one found by watching the machine
rather than by reasoning about it.

1. **`$030a` is `DrvQHdr + 2`**, the head of the drive queue, and the ROM hangs on purpose
   when it is empty. Asserting the drive installed line of the IWM put a drive in it.
2. **`$4007d4` was never a hang.** The halt detector measured its threshold in instructions,
   so a wait for the tick counter — a loop that changes no register and lasts up to a whole
   frame — looked stuck after a fifth of a frame. It now measures in cycles and needs ten
   frames. The vertical blanking had been working perfectly all along, 441 raises in 441
   frames.
3. **The ROM decides whether the machine has SCSI by comparing a long at `$420000` with one
   at `$440000`.** Mirroring the ROM over the whole 4Mb quarter made those equal, so the ROM
   concluded there was no SCSI, skipped the bus scan at `$407d40` and never looked. The
   sockets of the Plus take 256Kb, so the window is `$400000`–`$43FFFF` with the 128Kb image
   repeated twice, and past it nothing answers.
4. **The clock had to work before any of that.** The ROM bit bangs it during startup and
   without an answer never learns the boot device. It now writes valid parameter RAM, `$a8`
   signature and defaults, on the first run.
5. **Selection happens after the select line goes up, not on its edge.** The driver
   arbitrates with its own id, asserts select, and only then puts the target on the data bus
   and asserts it. Sampling the data bus on the select edge sees only the initiator and
   selects nothing.
6. **The request and acknowledge lines interlock.** A target that holds request asserted for
   ever leaves the driver waiting for it to fall, and every acknowledge it keeps sending
   meanwhile hands over the same stale byte again — which is exactly what a descriptor block
   reading `08 08 08 08 08 08` looks like.

**The next thread** is the data in phase. After the `READ(6)` the driver masks interrupts,
writes `$80` to the initiator command register — a bus reset — and starts over, so something
about handing the 512 bytes back is wrong. The likeliest candidate is the bus and status
register: the end of DMA transfer bit is never set and the DMA request bit is asserted
whenever the bus is not free, neither of which is right.

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

A diskette is reported and set aside rather than quietly dropped. The drives are not emulated
yet, and a file that vanishes without a word is worse than one refused.

## Package structure

Flat root package with subpackages only where izapple2 has them, filename prefixes carrying
the grouping — matching the `cardXxx.go` / `traceXxx.go` convention.

```
izmac/
  mac.go              type Mac, the machine and its accessors
  macRun.go           run loop, scanline tick, speed control
  command.go          command channel, mirrors izapple2/command.go
  configuration.go    command line flags
  memoryManager.go    address decode, overlay, iz68000.Memory impl
  rom.go              ROM loading and identification by checksum
  video.go            screen.VideoSource implementation
  via.go              the Mac's VIA wiring, over component/mos6522.go
  keyboard.go         key code table and transition queue
  mouse.go            quadrature generation
  scc.go              85C30, minimal: mouse DCD interrupts and serial stubs
  iwm.go              stub
  sound.go            sound buffer to audio sink
  scsi5380.go         NCR 5380 initiator side
  scsiTarget.go       direct-access target, the SCSI command set
  traceToolbox.go     A-line trap tracing by name
  traceSadMac.go      Sad Mac error code decoding
  traceLowMem.go      low memory globals watch
  component/          the chips, knowing nothing of the machine: the 6522
                      copied from izapple2 and extended, and the clock
  screen/             snapshot to *image.RGBA
  storage/            block device, shaped after izapple2/storage.BlockDisk
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

**Where it is now**: the ROM completes the power on sequence, **draws the grey desktop with
the blinking question mark floppy and the mouse cursor**, and sits in its disk search retry
loop at `$4007ba` looking for something to boot from. Nothing is wrong with the machine at
this point; it is doing what a Macintosh with no bootable disk does.

**The `$030a` hang and the `$4007d4` hang are both resolved.** `$030a` is `DrvQHdr + 2`, the
head of the drive queue, and the ROM hangs on purpose when it is empty; asserting the drive
installed line put a drive in it. `$4007d4` was never a hang at all — see below.

**The next blocker is the real time clock.** The ROM bit bangs the clock chip during startup —
109 transitions of the enable line in a 700 frame run — and gets nothing back, so it never
learns which device to boot from and **never probes the SCSI bus**. With a disk image attached
the 5380 sees three accesses in a whole run, all of them the bus reset during hardware init.
So the RTC and its parameter RAM, the M2 work, is what stands between here and booting from
SCSI. The protocol is in the reference notes above.

**A lesson about the tooling, which cost more time than any of the hardware.** The halt
detector reported `$4007d4` as a halt for a long time, and it was not one: the ROM was waiting
for the tick counter with

	CMP.L  ($16a).W,D0
	BEQ    *

a loop that changes no register and covers two addresses, and that legitimately runs for up to
a whole frame until the vertical blanking interrupt moves the counter. The detector measured
its threshold in *instructions*, so at roughly 26 cycles an iteration it declared a halt after
about a fifth of a frame — every time, before the tick could possibly arrive. Everything
downstream of that was chasing a bug that did not exist: the vertical blanking flag was being
raised exactly once per frame all along, 441 times in 441 frames, and the processor was taking
the interrupts.

The detector now measures in **cycles** and requires ten frames of nothing changing. A wait for
an interrupt never lasts that long and a real halt never ends, so the two are cleanly
separated. Both cases have tests.

The general lesson is worth keeping: **when a measurement says the machine is stuck, check that
the measurement is not the thing that is broken.** The same mistake had already happened once,
when the detector fired on the ROM checksum loop.

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

A diskette is reported and set aside rather than quietly dropped. The drives are not emulated
yet, and a file that vanishes without a word is worse than one refused.

## Package structure

Flat root package with subpackages only where izapple2 has them, filename prefixes carrying
the grouping — matching the `cardXxx.go` / `traceXxx.go` convention.

```
izmac/
  mac.go              type Mac, the machine and its accessors
  macRun.go           run loop, scanline tick, speed control
  command.go          command channel, mirrors izapple2/command.go
  configuration.go    command line flags
  memoryManager.go    address decode, overlay, iz68000.Memory impl
  rom.go              ROM loading and identification by checksum
  video.go            screen.VideoSource implementation
  via.go              the Mac's VIA wiring, over component/mos6522.go
  keyboard.go         key code table and transition queue
  mouse.go            quadrature generation
  scc.go              85C30, minimal: mouse DCD interrupts and serial stubs
  iwm.go              stub
  sound.go            sound buffer to audio sink
  scsi5380.go         NCR 5380 initiator side
  scsiTarget.go       direct-access target, the SCSI command set
  traceToolbox.go     A-line trap tracing by name
  traceSadMac.go      Sad Mac error code decoding
  traceLowMem.go      low memory globals watch
  component/          the chips, knowing nothing of the machine: the 6522
                      copied from izapple2 and extended, and the clock
  screen/             snapshot to *image.RGBA
  storage/            block device, shaped after izapple2/storage.BlockDisk
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

**Where it is now**: the ROM completes the power on sequence and **draws the grey desktop
with the blinking question mark floppy and the mouse cursor**. It then deadlocks on purpose
at `$4006e8`:

	MOVE.L  ($030a).W,D6
	BEQ     *

**`$030a` is `DrvQHdr + 2`, the head of the drive queue.** The code around it walks a linked
list through the long at offset 0 of each element and reads a drive number at offset 6. The
ROM hangs on purpose when the queue is empty: a Macintosh with no drive at all never finishes
booting.

Nothing writes `$030a` with an absolute address — a scan of the ROM finds five references and
all five read it — and a watch on the RAM shows it written only by the memory test patterns,
then by the fill, then cleared at `$401a3c`, and never filled. No drive ever registers,
because the IWM stub answers that nothing is connected.

Driving the drive sense lines low was tried and it does get a drive into the queue and the
boot past `$4006e8`, to `$402424`, which confirms the mechanism. It is not the fix: answering
"there is a drive and a disk in it" only moves the hang into the `.Sony` driver. The stub
therefore still says nothing is connected, which is at least true.

**SCSI does not solve this, which corrects an assumption of this plan.** With a disk image
attached and every register access logged, the ROM touches the 5380 exactly three times
before it hangs: assert reset, release reset, clear the mode register. That is the hardware
init resetting the bus. It never scans for a device, because the drive queue hang happens
first. On this ROM the queue is filled by the floppy driver and the SCSI boot scan comes
later, so **the floppy is upstream of SCSI and had to be dealt with first**.

**How it was dealt with.** Logging which status line selectors the ROM asks about during the
boot shows only four in use — 1, 10, 12 and 14 — with 14 asked about far more often than the
rest, which is the shape of a presence poll. Asserting 14 alone got a drive into the queue and
the boot past `$4006e8`. The selector is CA2, CA1, CA0 and SEL, where **SEL comes from VIA
port A bit 5** and not from the IWM; that wiring was missing and is in place now.

The book's table then confirmed the reasoning and corrected the number: 1 is CSTIN and 12 is
SIDES, exactly as the shape of the polling suggested, but **DRVIN is selector 15, not 14**.
The Plus polls 14 and never 15. The book documents the 400K drive of the 128K and 512K
machines while the Plus ships an 800K one, so these are most likely the same line on different
drives. izmac asserts both and leaves the rest negated, which says a drive is present with no
disk in it. Selector 10 has no entry in the table at all and stays negated.

**Where it stops now**: `$4007d4`, in

	CMP.L  ($16a).W,D0
	BEQ    *

`$016a` is `Ticks`, the counter the vertical blanking handler increments sixty times a
second, so the ROM is waiting for time to pass. The VIA does assert its interrupt — the first
one lands at cycle 130250, exactly one frame — but a watch on `$016a` shows nothing ever
increments it, so the handler is not running or is not the one expected. That is the next
thread. One ordering bug was found and fixed while looking: the interrupt lines were being
updated before the scan line tick that raises the vertical blanking rather than after.

A third lesson, about the tooling rather than the machine: a halt detector that watches for a
narrow band of addresses with the registers standing still catches the loop the ROM ends on,
but it *also* catches a poll of a device that never answers, because a poll that discards
what it reads changes no registers either. The headless frontend therefore reports a stop as
a stop and offers the Sad Mac reading as an interpretation, with the registers and a
disassembly next to it, rather than asserting a failure the ROM never reported.

### M0 — Scaffolding

`go.mod` (`github.com/ivanizag/izmac`, with a `replace` to `../iz68000` while it is
unreleased), `AGENTS.md` adapted from izapple2, `configuration.go` with `-rom`, `-disk`,
`-trace`, both frontends building.

The ROM is **not distributable** and must be supplied by the user. `rom.go` identifies it by
checksum — the first long word of a Mac ROM is its own checksum — and refuses anything that
is not a Plus ROM, with a clear message naming what it got. The Plus has three 128 KB
revisions, all reporting ROM version `0x0075`:

| Checksum | Version | Nickname | Notes |
|---|---|---|---|
| `0x4D1EEEE1` | v1 | Lonely Hearts | SCSI driver bug: will not boot if an external drive is off |
| `0x4D1EEAE1` | v2 | Lonely Heifers | Boot bug fixed; the majority of beige Pluses |
| `0x4D1F8172` | v3 | Loud Harmonicas | Handles drives returning Unit Attention on power-up or reset |

**Target v3, `0x4D1F8172`.** Beyond being the last revision, it is the one that tolerates a
target returning Unit Attention after power-up or reset — which is exactly what a freshly
constructed emulated target does. Choosing v3 removes a constraint from `scsiTarget.go` that
v1 and v2 would impose. Accept v1 and v2 with a warning rather than refusing them.

*Exit*: `go run ./frontend/macebiten` opens a window; `go test ./...` is green.

### M1 — Sad Mac

Memory map, ROM loading, overlay, reset, and A-line trap dispatch. **No video yet.** The
target is reaching the ROM's power-on test failure path, which is detectable from CPU state
alone — so this milestone needs no framebuffer and no VIA beyond the overlay bit.

The Sad Mac codes are a precise oracle, and they live in **CPU registers**, which is what makes
this milestone viable without video: **`D7` low word is the major code** (the class of test that
failed) with its high word holding Apple-internal flags, and **`D6` is the minor code** (the
specifics — a bitmask naming the failing RAM chip, or the exception number). The screen shows
them as `YYXXXX`, class then subcode.

| Class | Test |
|---|---|
| `01` | ROM checksum |
| `02` | RAM bus subtest — subcode is a chip bitmask |
| `03` | RAM byte-write — subcode is a chip bitmask |
| `04` | RAM modulo-3 pattern — subcode is a chip bitmask |
| `05` | RAM address uniqueness — subcode is a chip bitmask |
| `0F` | Exception before the OS loaded — subcode names the 68000 exception |

**Class `0F` is the one that will fire most during bring-up**, and it is the most useful: a bug
in izmac surfaces as an exception number telling us which vector we mishandled.

For detecting the Sad Mac without a ROM-version-specific address, watch for the CPU entering a
tight infinite loop — where the display routine ends — and dump `D7`/`D6` decoded. That is
version independent and needs no symbol table.

`traceSadMac.go` does this decoding. Build it here, with the Toolbox tracer alongside it.

*Exit*: headless run reaches the Sad Mac path and prints a decoded reason.

### M2 — Happy Mac and the blinking "?" floppy

VIA, the VBlank interrupt, the scanline tick, and the framebuffer. The big morale moment: a
Happy Mac, then the blinking question-mark disk. The blink *is* the timing test — reaching it
proves the memory map, the overlay transition, video, and the VBlank rate all at once.

Sound is nearly free once the scanline tick exists, so **take the boot beep opportunistically
here** even though full sound work is M5. It is a good early sign of life, and it validates
the tick independently of video.

*Exit*: both frontends show the Happy Mac and then the "?" disk blinking at roughly 1 Hz; a
headless framebuffer hash asserts it.

### M3 — A usable ROM: RTC, PRAM, keyboard, mouse

Extend the 6522 with the shift register and the CA/CB lines.

**RTC and PRAM**: a one-bit serial protocol over `rTCData` PB0, `rTCClk` PB1 and `rTCEnb` PB2.
Small but not optional — bad parameter RAM and the ROM will not find a boot device. izapple2's
`component/microPD1990ac.go` is a different chip but the same shape of protocol, a good
template. The chip lives in `component/` because it needs nothing from the rest of the
machine: the VIA hands it the three lines and takes the one it drives back.
**PRAM must persist to a file** or every run re-does the no-startup-disk path. It
defaults to `pram.bin` in the working directory, overridable with `-pram`.

The protocol, from pages III-37 and III-38. Pull `rTCEnb` low for the whole transaction —
raising it aborts. Bytes go high bit first. The command byte's top bit is 1 to read and 0 to
write, and **its low two bits are always 01**:

| Command | Register |
|---|---|
| `z0000001`, `z0000101`, `z0001001`, `z0001101` | the four seconds bytes, low order first |
| `00110001` | test register, write only |
| `00110101` | write protect register, write only |
| `z010aa01` | PRAM `$10`–`$13` |
| `z1aaaa01` | PRAM `$00`–`$0F` |

Twenty bytes of PRAM in total. Bit 7 of the write protect register blocks writes to
everything including PRAM. The seconds counter is four bytes incremented once a second; the
book warns to write it low byte first and to read it twice until two reads agree, because it
can tick mid-transfer. The one second interrupt is enabled by bit 0 of the VIA IER.

**The seconds answer to two groups of commands and not one.** The bit 4 is not decoded for
them, so `$8d` and `$9d` are the same register. That reads like a curiosity of the chip until
you see how the ROM uses it: `INITUTIL_CONT` reads the counter by stepping *down* from `$9d`
by four, which takes it through the four registers with the bit set and then the four with it
clear, and compares the halves to be sure it did not catch the counter mid-tick. A chip that
answers only one group returns four zeros for the other half, the comparison fails, the ROM
retries once and then gives up with a clock read error — and the machine sits at 1904 with
nothing to say why. The book's table lists only the four commands with the bit clear, so this
one comes from the disassembly and not from Inside Macintosh.

**Keyboard**: bidirectional bit-banged protocol over the VIA shift register, fiddly enough
that most emulators simplify it. Codes are **Mac raw key codes, not ASCII**, which is why the
frontend must deliver physical key up/down events. Commands and response encoding are in the
reference section above; the Inquiry timeout path is the one to get right.

**Mouse — a decision DESIGN.md flagged as worth taking deliberately rather than drifting
into.** The two options are writing the position straight into the low memory globals, or
generating real quadrature: X2/Y2 on VIA PB4/PB5, X1/Y1 on the SCC data carrier detect inputs
to raise interrupts, button on PB3. **Recommendation: real quadrature.** Two reasons — enough
SCC is needed anyway so the ROM's serial init does not hang, and the low-memory-globals route
is fragile against ROM version differences. The pacing worry dissolves against the scanline
tick: at most one quadrature transition per tick is ~22 kHz, an order of magnitude above what
a real mouse produces. The ROM infers direction from the phase relationship between X1 and X2,
so it must be a genuine quadrature sequence, not merely interrupts. Fall back to the low
memory hack only if this actively fights us.

*Exit*: PRAM survives a restart; keys reach the ROM; the cursor tracks and clicks.

### M4 — SCSI, and the Finder

The biggest and riskiest milestone. `scsi5380.go` implements the initiator registers, the
phase state machine and the pseudo-DMA path the ROM uses. `scsiTarget.go` is one
direct-access device backed by a disk image, answering TEST UNIT READY, INQUIRY, REQUEST
SENSE, READ CAPACITY, MODE SENSE and READ/WRITE (6) and (10).

Add a SCSI trace printing each CDB and phase transition — with a bus protocol, watching the
conversation is far faster than reasoning about it.

The image must be an HFS volume with a blessed System Folder; start from a known-good one
rather than trying to create it.

*Exit*: **the Finder desktop.** A headless boot test asserts a framebuffer hash within a cycle
budget, mirroring izapple2's `e2e_boot_test.go`.

### M5 — Sound, expansion, serial

Full sound: 370 samples per frame, volume from PA0–PA2, gated on PB7, into the frontend's
audio sink. Then 4 MB of RAM, a second SCSI drive, and serial with AppleTalk.

### M6 — The IWM and real floppies

The long tail: five speed zones, a different GCR nibble table from the Disk ][, tag bytes,
two sides on the 800K.

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
   datasheet. It works; the rest of the bits are unknown.
3. **Selector 10 of the drive status lines**, which the Plus polls and the book's 400K table
   does not list.

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
