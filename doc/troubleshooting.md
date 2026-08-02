# When something goes wrong

[Back to the manual](manual.md)

Most of what follows is the real machine behaving as it did. A Macintosh Plus
had few ways of telling you what was wrong, and izmac does not invent more.

## It will not start at all

**"The ROM file izmac_default.rom is not here" and then a download that
fails.** The copy izmac fetches lives at the Internet Archive, so this needs a
network and a site that is up. If either is missing, find a Macintosh Plus ROM
image yourself and name it with `-rom`.

**"No disk image was given" and then a download that fails.** With nothing
named there is nothing to boot, so izmac fetches the MacPaint diskette it
starts on, and that lives at the Internet Archive too. Name a disk image of
your own and it is not needed at all.

**"the ROM file is N bytes, a Macintosh Plus ROM is 131072".** The file is not
a Plus ROM: 128Kb exactly is the size, and a 64Kb one is from a 128K or 512K
Macintosh, which izmac does not emulate.

**"the ROM file is corrupt".** The first four bytes of a Macintosh ROM are a
checksum of the rest, and this one does not add up. The file is truncated or
mangled.

**"is not a known Macintosh Plus ROM".** It checksums correctly but is none of
the three revisions — most likely a ROM from another Macintosh model.

**"Warning: ... is not the revision izmac targets".** It will run. Revisions 1
and 2 have an older SCSI driver that does not expect a disk to report it has
just been reset, which is what an emulated disk does, so a hard disk may not
come up. Revision 3, *Loud Harmonicas*, is the one to use.

**"unsupported RAM size".** `-ram` takes `1024` or `4096` and nothing else.

**An option was ignored.** The options have to come before the file names —
the first name without a dash ends them. `izmac mydisk.img -speed full` runs
at normal speed and quietly treats `-speed` as a disk image.

## A Sad Mac

A frowning Macintosh with a six digit code below it is the ROM's power on test
reporting a failure before there is anything else to report with. The first
two digits are the class:

| | |
|---|---|
| `01` | ROM checksum |
| `02`, `03`, `04`, `05` | the four memory tests |
| `0F` | an exception before the system was loaded, which is where a software failure lands |

On a real machine this meant a chip had gone. Here it means the emulated
machine got into a state it should not have, which is a bug worth reporting.
`-trace sadmac` decodes the code, and in the headless frontend it also stops
the run at the point of failure.

## A disk with a blinking question mark

The machine looked for something to start from and found nothing. It is not an
error; it is the machine waiting for a disk, and it will pick one up the
moment you drop a bootable diskette on the window.

Either no disk was given, or none of them has a System on it, or the hard disk
image has no SCSI driver — a raw file of zeros is not a startup disk. See
[Disks and diskettes](disks.md).

## It stops on the *Welcome to Macintosh* or *Starting up* screen

If the machine gets that far and stops, the usual cause is AppleTalk. The
serial ports are not emulated, so the LocalTalk driver programs the chip,
waits for an interrupt that can never arrive, and never gives up. izmac starts
the machine with both ports marked as in use so this does not happen, but a
`izmac_pram.bin` saved by something else, or a System where AppleTalk was switched
on, can get past that.

Delete `izmac_pram.bin` and start again, or switch AppleTalk off in the Chooser
before it is saved.

## The mouse or the keyboard does nothing

**Click on the window first.** Until you do, the machine has neither, and
neither does it while the F10 menu is up. The title bar says which way round
it is.

If you have the pointer and want it back, **right click** or press
**Escape**.

**Command-something does nothing.** Your own system took it before the
machine saw it — Command-Q and Command-Tab on macOS, and whatever your window
manager claims elsewhere. The same combination with **Alt** or **Option**
gets through, since both keys are the Macintosh command key. See
[Keyboard, mouse and the menu](controls.md).

**The arrow keys do nothing.** They are not mapped, and neither is the numeric
keypad.

## Copy and paste

**A copy inside the machine did not reach my clipboard.** Some applications
keep a clipboard of their own and only publish it when they are switched away
from. Click on another window inside the machine, the Finder will do, and it
turns up. The other cause is a clipboard the System has written to a file to
free the memory, which izmac does not read.

**A paste did not arrive.** Your clipboard is handed over as the window takes
the focus, so a copy made without the window ever losing it — from a clipboard
manager, a hotkey or a script — has not been sent. Press **F11**, or use
*Paste from the host* on the F10 menu.

Nothing at all crosses if izmac was started with `-clipboard=false`.

**A paste arrived with question marks in it.** The Macintosh has one byte a
character and no room for the em dashes, curly quotes and emoji of a modern
system. What it has no equivalent for arrives as a question mark.

**Pictures do not cross.** Text only, in both directions.

## Disks

**"is N bytes: the only diskettes a Macintosh Plus can read..."** — 400K and
800K are the sizes these drives make. A 720K or 1.44M image is from a later
machine and there is no drive here that could read it.

**An image was taken for the wrong kind of disk.** izmac guesses from the
contents, and a file that is neither formatted by Apple's tools nor exactly
400K or 800K is taken for a hard disk. Say which it is with `-hd` or
`-floppy`.

**Nothing was saved to a diskette.** The image is written back when the drive
motor stops, a couple of seconds after the machine has finished with it.
Closing the window before then loses it. Eject the diskette, or wait for the
drive to go quiet.

**The Finder refuses to change anything on a disk.** The file is read only on
the host, so the machine sees a locked diskette or a hard disk it cannot write
to. For a diskette, the startup lines and the F10 menu both say `locked`
beside the name; a hard disk opened read only says nothing, so check the
permissions of the file yourself.

**A diskette was swapped and the Macintosh did not notice.** Ejecting from the
F10 menu takes the image out from under the System, which goes on believing
the disk is there — the same thing that happened if you got a real diskette
out with a paperclip. Eject from the Finder, by dragging the disk to the trash
or with Command-E, and the machine follows along.

## Speed and sound

**Everything is too fast, the chime is wrong, the clock gains.** The machine is
at full speed — press **F5**, or look at the title bar. Nothing inside the
machine knows the difference; see
[Speed, the clock and the tracers](advanced.md).

**It is slower than the real machine.** **Ctrl-F5** prints the speed being
reached. A tracer left on, `cpu` above all, costs more than the emulation
does.

**No sound.** The Macintosh has its own volume, in the Control Panel, and it
can be turned all the way down. That setting is kept in `izmac_pram.bin`.

## Nothing here fits

The tracers are the next step: `-trace toolbox` says where the machine got to
in words, `-trace scsi` and `-trace floppy` follow the disks, and the headless
frontend reports the registers and the code at the point everything stopped.
[Speed, the clock and the tracers](advanced.md) covers them, and a bug that
survives all that is worth
[reporting](https://github.com/ivanizag/izmac/issues).
