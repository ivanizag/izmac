# izmac

A Macintosh Plus emulator written in Go, built on
[iz68000](https://github.com/ivanizag/iz68000).


```bash
go run ./frontend/macebiten mydisk.img
go run ./frontend/macebiten system.img work.img scratch.img
```


The [manual](doc/manual.md) covers all of this in full: the disks, the
keyboard and the mouse, every option, and what to do when something does not
work.

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
| `-hd` | a hard disk image for the SCSI bus, repeat for more than one |
| `-floppy` | a diskette image for a drive, repeat for the external one |
| `-pram` | where the parameter RAM is kept, `pram.bin` by default |
| `-ram` | RAM size in Kb, 1024 or 4096 |
| `-speed` | `plus` for the real 7.8336Mhz, `full` for as fast as the host goes, or a number of Mhz |
| `-clipboard` | share the clipboard with the host, on by default. `-clipboard=false` keeps them apart |
| `-trace` | tracers to enable: `cpu`, `toolbox`, `sadmac`, `scsi`, `floppy` |

Files named without an option are disk images. What each one
is is worked out from the image itself: a Macintosh hard disk starts with a
driver descriptor map, a DiskCopy image says so in its header, and a diskette
is exactly 400K or 800K.

Up to seven disks go on the SCSI bus, taking the ids 0 upwards in the order
given. Note that the options have to come before the file names.

## Diskettes

Both drives are emulated, the internal one and the external, and diskettes go
in them in the order they are named. A plain 400K or 800K image works, as does
a DiskCopy 4.2 one. Those are the only sizes a Macintosh Plus can read.

```bash
# A diskette in the internal drive and a hard disk on the bus
go run ./frontend/macebiten -floppy system.dsk mydisk.img

# Or just name them, izmac works out which is which
go run ./frontend/macebiten mydisk.img games.dc42
```

Writing and formatting work, and a diskette is written back to the file it came
from when the drive stops. In the window, dropping an image on it puts the
diskette in a free drive and the menu on F10 takes one out.

## Copy and paste

The clipboard is shared with the host, both ways, on every System the machine
runs.

Copying inside the machine puts the text on the clipboard of the host as soon
as the application has finished copying it.

Going the other way, the clipboard of the host is handed to the machine
whenever the window is given the focus, so copying on the host and clicking
back on the window is usually all there is to it. **F11 forces the paste**, and
so does the item on the F10 menu, for when the focus never changed: a copy made
by a hotkey or a script, or a machine whose clipboard has been replaced since.

Either way the text arrives as the clipboard of the machine, so it is ⌘V in the
application that pastes it and not a burst of typing. Both the command key of
the host and the option key are the command key of the Macintosh, so ⌘V and
option-V do the same thing: the second is there for the combinations the host
keeps for itself.

Text only. A picture copied on the machine is a PICT, which the host has no
use for without a decoder, and it is left alone rather than emptying the
clipboard of the host.

Two things are worth knowing, and neither is izmac being careful:

- An application that keeps a clipboard of its own only publishes it when it
  is switched out, which under MultiFinder and System 7 means clicking on
  another window. Until it does, there is nothing to see from outside it.
- The clipboard is sometimes written to the Clipboard file instead of being
  kept in memory, and a copy that has gone to disk is not picked up.
