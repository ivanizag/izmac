# The izmac manual

izmac is an emulator of the Macintosh Plus, the machine Apple sold from 1986:
a 68000 running at 7.8336 MHz, one megabyte of memory, a 512 by 342 black and
white screen, two 800K diskette drives and a SCSI bus. izmac runs the real
ROM and the real software, and what you get on the screen is what the machine
put there.

This manual is in six parts. If you have never used a Macintosh of that age,
read the first two and stop; the rest is there when you want it.

| | |
|---|---|
| **This page** | installing izmac, the first run, and what the machine is |
| [Disks and diskettes](disks.md) | where the software comes from, and how to put it in |
| [Keyboard, mouse and the menu](controls.md) | driving the machine and driving the emulator |
| [Command line options](options.md) | every option of both frontends |
| [Speed, the clock and the tracers](advanced.md) | the emulator's own knobs, and the debugging tools |
| [When something goes wrong](troubleshooting.md) | the failures worth recognising |

## Installing

There is nothing to install. [Download](https://github.com/ivanizag/izmac/releases)
the archive for Linux, Windows or macOS and run the single executable inside.
On macOS it is also on homebrew:

```bash
brew install ivanizag/tap/izmac
```

To run it from the source instead you need Go, and then:

```bash
go run ./frontend/macebiten
```

Everywhere below, `izmac` means the executable from the archive. If you are
running from the source, put `go run ./frontend/macebiten` in its place.

## The ROM

A Macintosh without its ROM does nothing at all: the ROM is the machine's
firmware, and on this one it holds most of the operating system as well. It is
still copyrighted, so it is not part of izmac and never will be.

The first time you run izmac without naming a ROM, it fetches a copy of the
one it targets from the Internet Archive and saves it as `default.rom` in the
directory you ran it from. After that it uses that file and downloads nothing.
If you already have an image, name it with `-rom` and nothing is downloaded at
all.

```bash
izmac -rom macplus.rom mydisk.img
```

There were three revisions of the Macintosh Plus ROM, and izmac targets the
third, known as *Loud Harmonicas*. The other two work, but they have an older
SCSI driver that does not expect a disk to report that it has just been reset,
which is exactly what an emulated disk does; izmac prints a warning when it
finds one of them and you may find a hard disk does not come up.

## The first run

izmac has no software of its own to boot. You need a disk image with a System
on it — see [Disks and diskettes](disks.md) for what those are and where they
go. Given one, this is the whole of it:

```bash
izmac mydisk.img
```

The window opens at twice the size of the real screen, and can be resized. The
machine chimes, shows the smiling Macintosh, and a few seconds later you are
in the Finder.

Then:

- **Click on the window** to hand the mouse over to the machine. The pointer
  is captured, so your own pointer stops moving; **right click**, or press
  **Escape**, to get it back.
- **F10** opens the emulator's menu, for the things the Macintosh knows
  nothing about: full speed, a screenshot, reset, and the diskette drives.
- The **Alt** or **Option** key of your keyboard is the Macintosh **Command**
  key, the one with the clover on it. Your **Control** key is the Macintosh
  **Option**.

That is enough to use the machine. The title bar tells you the speed it is
running at and how to give the mouse over or take it back.

## What is emulated

Everything the machine needs to run its software: the processor, the memory
and the ROM overlay, the video, the sound, the VIA and the real time clock
with its parameter RAM, the keyboard and the mouse, the SCSI bus with up to
seven disks on it, and both diskette drives — reading, writing and formatting.

What is not there is the pair of serial ports. Enough of the serial chip is
emulated for the mouse, which is wired to it, and no more. Nothing that talks
over a serial port works: no LocalTalk network, no AppleTalk, no printing to
an ImageWriter or a LaserWriter, no modem. izmac starts the machine with both
ports marked as in use, which is what a real Macintosh with AppleTalk turned
off in the Chooser looks like, and is what keeps a System from trying.

## Where izmac writes

Everything goes in the directory you ran it from:

| File | What it is |
|---|---|
| `default.rom` | the ROM, if izmac had to download one |
| `pram.bin` | the parameter RAM, which is where the machine keeps the date, the volume and the desktop settings between runs. Change it with `-pram` |
| `izmac-<date>-<time>.png` | a screenshot, when you ask for one from the menu |

Your disk images are written in place, so what the Macintosh saves stays
saved.
