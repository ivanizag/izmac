# The izmac manual

izmac is an emulator of the Macintosh Plus, the machine Apple sold from 1986:
a 68000 running at 7.8336 MHz, one megabyte of memory, a 512 by 342 black and
white screen, two 800K diskette drives and a SCSI bus. izmac runs the real
ROM and the real software, and what you get on the screen is what the machine
put there.

This manual is in seven parts. If you have never used a Macintosh of that age,
read the first two and stop; the rest is there when you want it.

| | |
|---|---|
| **This page** | installing izmac, the first run, and what the machine is |
| [Disks and diskettes](disks.md) | where the software comes from, and how to put it in |
| [Keyboard, mouse and the menu](controls.md) | driving the machine and driving the emulator |
| [Printing](printing.md) | the ImageWriter on the printer port, and where the pages go |
| [Command line options](options.md) | every option of both frontends |
| [Speed, the clock and the tracers](advanced.md) | the emulator's own knobs, and the debugging tools |
| [When something goes wrong](troubleshooting.md) | the failures worth recognising |

## Installing and running it

There is nothing to install in the usual sense. izmac is one executable with
nothing beside it: no runtime to fetch first, no libraries, no data files. The
things it does need — the ROM and a disk to boot from — it fetches itself the
first time it wants them.

[Download](https://github.com/ivanizag/izmac/releases) the archive for Linux,
Windows or macOS and unpack it. Inside are the executable, a README and the
licence, flat. On macOS it is also on homebrew, which is the easier way of the
two since it puts izmac on your path and updates it with everything else:

```bash
brew install ivanizag/tap/izmac
```

Then run it. From homebrew it is on your path already, so its name is enough;
from the archive it is a file in whatever directory you unpacked it into, and
on Linux and macOS a program in the directory you are in has to be named as
one:

```bash
izmac                  # homebrew, or anywhere on your path
./izmac                # the unpacked archive, from that directory
izmac.exe              # Windows, from that directory
```

**Run it from a directory you can write to**, and preferably one of its own.
izmac writes what it downloads and what it remembers beside itself in the
directory you ran it from, not in a hidden folder somewhere: the ROM, the
MacPaint diskette, the parameter RAM, any screenshot. [Where izmac
writes](#where-izmac-writes) at the end of this page has the list. A directory
of its own keeps that together and keeps the second run from downloading
anything again.

Two things the operating system may do the first time, neither of them izmac
going wrong. On macOS an executable out of an archive is unsigned and
Gatekeeper stops it; open *System Settings > Privacy & Security*, where it
will have offered to let it run anyway, and it is not asked again. The
homebrew install does not have this. On Windows, SmartScreen does much the
same and takes *More info > Run anyway*.

Everywhere below, `izmac` means that executable, and the options and the disk
images go after it:

```bash
izmac mydisk.img
izmac -rom macplus.rom -ram 4096 system.img
```

## Running from the source

The other way is to run the source directly, which is what you want if you
are changing izmac, or if you are on a platform there is no archive for. You
need [Go](https://go.dev/dl/) 1.26 or newer and nothing else; on Linux you
also need the X11 and OpenGL development headers, which is what
[Ebitengine](https://ebitengine.org/en/documents/install.html) needs to build
its window.

```bash
git clone https://github.com/ivanizag/izmac
cd izmac
go run ./frontend/macebiten
```

The first build takes a minute while Go fetches the dependencies and compiles
them; after that it is a second or two. What izmac downloads and remembers
still lands in the directory you ran it from, which here is the checkout
itself; the repository ignores all of it, so it does not show up as something
you have changed.

Everything this manual says about `izmac` applies, with the command in its
place:

```bash
go run ./frontend/macebiten mydisk.img
```

There is a second frontend in the source that the releases do not ship,
`headless`, which runs the machine without a window and reports on the
terminal. It is a debugging tool rather than a way to use the machine — see
[Command line options](options.md) and [Speed, the clock and the
tracers](advanced.md).

```bash
go run ./frontend/headless -frames 120 mydisk.img
```

If you would rather have an executable than a `go run`, build one, and it
behaves exactly like the one out of the archive:

```bash
go build -o izmac ./frontend/macebiten
```

## The ROM

A Macintosh without its ROM does nothing at all: the ROM is the machine's
firmware, and on this one it holds most of the operating system as well. It is
still copyrighted, so it is not part of izmac and never will be.

The first time you run izmac without naming a ROM, it fetches a copy of the
one it targets from the Internet Archive and saves it as `izmac_default.rom`
in the directory you ran it from. After that it uses that file and downloads
nothing.
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

Run izmac with nothing at all and it comes up on MacPaint:

```bash
izmac
```

There is no software inside izmac either, so what happens the first time is
the same as with the ROM. With no image named there is nothing to boot, so
izmac fetches a MacPaint 1.5 diskette — 400K, with a System and a Finder of
its own on it — saves it as `izmac_macpaint.dsk` in the directory you ran it
from, and puts it in the internal drive. After that it uses that file and
downloads nothing.

What is on it is a Macintosh of 1985 and not much more: a System, a Finder,
and the program that sold the machine. To run anything else you need a disk
image of your own, and naming one is all it takes — see
[Disks and diskettes](disks.md) for what those are and where they go.

```bash
izmac mydisk.img
```

The window opens at twice the size of the real screen, and can be resized. The
machine chimes, shows the smiling Macintosh, and a few seconds later you are
in the Finder.

Then:

- **Point at the window** and the machine points where you point. There is
  nothing to click on first, and your own pointer is hidden while it is over
  the screen because the machine is drawing one under it.
- **F10** opens the emulator's menu, for the things the Macintosh knows
  nothing about: full speed, a screenshot, reset, and the diskette drives.
- Your **Command** key is the Macintosh **Command** key, the one with the
  clover on it, and so is **Alt** or **Option** for the combinations your own
  system keeps. Your **Control** key is the Macintosh **Option**.
- **The clipboard is shared**, both ways and text only, so you can copy in the
  machine and paste outside it, and the other way round.

That is enough to use the machine. The title bar tells you the speed it is
running at, and how to give the mouse over or take it back on the days you ask
for the mouse the hardware really had — see
[Keyboard, mouse and the menu](controls.md).

## What is emulated

Everything the machine needs to run its software: the processor, the memory
and the ROM overlay, the video, the sound, the VIA and the real time clock
with its parameter RAM, the keyboard and the mouse, the SCSI bus with up to
seven disks on it, and both diskette drives — reading, writing and formatting.

The serial ports go one way. What the machine sends out of one of them arrives,
which is what a printer needs: choose the ImageWriter in the Chooser and the
pages come out as images beside the emulator. See [Printing](printing.md).
Nothing arrives the other way, and nothing that has to be answered works: no
LocalTalk network, no AppleTalk, no LaserWriter, no modem. izmac starts the
machine with both ports marked as in use for a serial device, which is what a
real Macintosh with AppleTalk turned off in the Chooser looks like, and is what
keeps a System from taking a port for the network and waiting forever for it.

## Where izmac writes

Everything goes in the directory you ran it from:

| File | What it is |
|---|---|
| `izmac_default.rom` | the ROM, if izmac had to download one |
| `izmac_macpaint.dsk` | the MacPaint diskette, if izmac had to download one because you named no image |
| `izmac_pram.bin` | the parameter RAM, which is where the machine keeps the date, the volume and the desktop settings between runs. Change it with `-pram` |
| `izmac_<date>-<time>.png` | a screenshot, when you ask for one with F12 or from the menu |
| `izmac_page_<n>.png` | a page the machine printed. Change it with `-printerfile`, or turn the printer off with `-printer none` |

Your disk images are written in place, so what the Macintosh saves stays
saved.
