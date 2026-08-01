# Command line options

[Back to the manual](manual.md)

izmac comes in two frontends. `macebiten` is the windowed one, the emulator
proper, and is what the releases ship as `izmac`. `headless` runs the machine
without a window and reports on the terminal; it is a debugging tool and is
not shipped, so you only have it if you build from the source.

```bash
izmac [options] [disk images...]

go run ./frontend/macebiten [options] [disk images...]
go run ./frontend/headless  [options] [disk images...]
```

**The options come first.** A name without a dash ends them, and everything
after it is taken as a disk image.

Any file named without an option is a disk image, and izmac works out from the
image itself whether it is a hard disk or a diskette. See
[Disks and diskettes](disks.md).

## The machine

Both frontends take all of these.

| Option | Default | Meaning |
|---|---|---|
| `-rom <file>` | `default.rom` | the Macintosh Plus ROM image, 128Kb. If the option is not given and the file is not there, it is downloaded once |
| `-hd <file>` | | a hard disk image for the SCSI bus. Repeat it for more than one, up to seven |
| `-floppy <file>` | | a 400K or 800K diskette image, plain or DiskCopy 4.2, to put in a drive. Repeat it for the external drive as well |
| `-ram <kb>` | `1024` | the memory size in Kb, `1024` or `4096`. Those are the two the real machine could be built with |
| `-speed <mhz>` | `plus` | `plus` for the real 7.8336 MHz, `full` for as fast as your machine goes, or a number of Mhz |
| `-pram <file>` | `pram.bin` | where the parameter RAM is kept between runs |
| `-clipboard` | on | share the clipboard with your system, both ways. `-clipboard=false` keeps them apart. See [Keyboard, mouse and the menu](controls.md) |
| `-wallclock` | off | read the clock from the host every time, instead of starting from it and counting the machine's own seconds |
| `-trace <list>` | | tracers to turn on, comma separated: `cpu`, `toolbox`, `sadmac`, `scsi`, `floppy` |
| `-profile` | off | write a CPU profile of the emulator itself |
| `-h` | | print the options and exit |

`-speed`, `-wallclock`, `-trace` and `-profile` are covered in
[Speed, the clock and the tracers](advanced.md).

## The headless frontend only

`headless` runs a fixed number of frames, prints where the machine ended up —
the registers, the instructions around the program counter, the cycles run —
and exits. It is how a change is checked without watching it.

| Option | Default | Meaning |
|---|---|---|
| `-frames <n>` | `60` | frames to run before reporting. A frame is a sixtieth of a second to the machine, however long it takes here |
| `-png <file>` | | write the screen as it ended up to a PNG file |
| `-disasm <hex>` | | disassemble from this address instead of running |
| `-disasmcount <n>` | `16` | how many instructions to disassemble |
| `-watch <hex>` | | report every write to a range of memory, with the instruction that made it |
| `-watchlen <n>` | `4` | the length of that range |

```bash
# Boot far enough to see the Finder, and keep the screen
go run ./frontend/headless -frames 600 -png screen.png system.img

# What is at the reset vector
go run ./frontend/headless -disasm 400000 -disasmcount 32

# Who is writing over the tick counter
go run ./frontend/headless -watch 16a -watchlen 4 -frames 120 system.img
```

## Examples

```bash
# The simplest thing that works
izmac mydisk.img

# Three disks on the SCSI bus, as ids 0, 1 and 2
izmac system.img work.img scratch.img

# A diskette in the internal drive and a hard disk on the bus
izmac -floppy system.dsk mydisk.img

# Your own ROM, four megabytes, and a boot at full speed
izmac -rom macplus.rom -ram 4096 -speed full mydisk.img

# Keep this machine's settings apart from the others
izmac -pram work-pram.bin work.img
```
