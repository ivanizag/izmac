# izmac

A Macintosh Plus emulator written in Go, built on
[iz68000](https://github.com/ivanizag/iz68000).


```bash
go run ./frontend/macebiten mydisk.img
go run ./frontend/macebiten system.img work.img scratch.img
```


## Installing

No installation required. [Download](https://github.com/ivanizag/izmac/releases)
the archive for Linux, Windows or macOS and run the single executable inside.

On macOS it can also be installed with homebrew:

```bash
brew install ivanizag/tap/izmac
```

## Running it

izmac needs a 128Kb Macintosh Plus ROM image, if none is provided it is downloaded from archive.org

```bash
# Windowed
go run ./frontend/macebiten mydisk.img

# Headless, dumps the screen on the terminal
go run ./frontend/headless -frames 120 system.img work.img scratch.img
```

| Option | Meaning |
|---|---|
| `-rom` | the ROM image, required |
| `-disk` | a disk image for the SCSI bus, repeat for more than one |
| `-floppy` | a diskette image for a drive, repeat for the external one |
| `-pram` | where the parameter RAM is kept, `pram.bin` by default |
| `-ram` | RAM size in Kb, 1024 or 4096 |
| `-speed` | `plus` for the real 7.8336Mhz, `full` for as fast as the host goes, or a number of Mhz |
| `-trace` | tracers to enable: `cpu`, `toolbox`, `sadmac`, `scsi`, `floppy` |

Files named without an option are disk images. What each one
is is worked out from the image itself: a Macintosh hard disk starts with a
driver descriptor map, a DiskCopy image says so in its header, and a diskette
is exactly 400K or 800K.

Up to seven disks go on the SCSI bus, taking the ids 0 upwards in the order
given. Note that the options have to come before the file names.

## Diskettes

Both drives of the Macintosh Plus are emulated, the internal one and the one
on the port at the back, and diskettes go in them in the order they are named.
A plain 400K or 800K image works, as does a DiskCopy 4.2 one, which is how a
Macintosh diskette is usually kept and which carries the tag bytes a plain
image has nowhere to put.

**400K and 800K are the only sizes there are.** The 1.44Mb diskette most
Macintosh software is distributed on today is recorded a different way and
arrived with the SuperDrive of later machines, so the drive of a Plus cannot
read one however the image is stored. izmac says so rather than trying. Look
for the 800K version of whatever it is, or put it on a hard disk image
instead.

```bash
# A diskette in the internal drive and a hard disk on the bus
go run ./frontend/macebiten -floppy system.dsk mydisk.img

# Or just name them, izmac works out which is which
go run ./frontend/macebiten mydisk.img games.dc42
```

Writing works, and so does formatting: the Macintosh can initialize a blank
image of the right size and use it from then on. A diskette is written back to
the file it came from when the drive stops, which is a few seconds after the
machine has finished with it, so the image on the host follows what the
Macintosh believes it has saved. An image the host will not let izmac write to
is still usable and appears as a locked diskette.

In the window, dropping an image on it puts the diskette in a free drive, and
the menu on F10 takes one out again. The machine can eject a disk by itself as
well, which is what dragging it to the trash does.

