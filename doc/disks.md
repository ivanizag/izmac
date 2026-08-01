# Disks and diskettes

[Back to the manual](manual.md)

All the software a Macintosh Plus runs comes off a disk, and izmac takes those
disks as files: a *disk image*, which is a file holding what a real disk held,
byte for byte. There are two kinds, and the machine treats them very
differently.

- A **hard disk** hangs off the SCSI bus at the back. It can be any size, it
  stays where it is, and the machine boots from it.
- A **diskette** goes in one of the two drives, holds 400K or 800K, and can be
  put in and taken out while the machine runs.

## Just name them

You do not have to say which is which. izmac looks inside each file named on
the command line and works it out:

```bash
izmac mydisk.img
izmac system.img work.img games.dsk
```

A Macintosh hard disk that has been through Apple's formatter starts with a
driver descriptor map, which no diskette carries. A DiskCopy image says what
it is in its own header. Failing both, a file of exactly 400K or 800K is a
diskette, because those are the only sizes these drives make, and anything
else is a hard disk.

If you would rather be explicit, or if a file is unusual enough that the guess
goes wrong, say so:

```bash
izmac -hd system.img -floppy games.dsk
```

Note that the options have to come **before** the file names. That is how Go's
flag parsing works: the first name without a dash ends the options.

## Hard disks

Up to seven images go on the SCSI bus. They take the ids 0, 1, 2 and upwards
in the order you name them — the Macintosh keeps id 7 for itself, which is why
seven is the limit.

```bash
izmac system.img work.img scratch.img
```

The image is a plain sequence of 512 byte blocks, which is what you get from
`dd` over a real disk, from most emulators, and from the disk image archives.
It is read and written in place rather than loaded, so an image of any size
costs nothing to attach and the machine's writes land in the file as they
happen.

If izmac cannot open a file for writing it opens it read only instead, without
complaining. The machine still boots from it and simply cannot save anything,
which looks from inside like a disk that refuses every write.

izmac does not make blank hard disks and cannot format one — a raw file of
zeros has no driver on it and the ROM will not boot it. Start from an image
that somebody already prepared, or make one under the Macintosh with a
formatter of the period, such as Apple's HD SC Setup, running from a diskette.

## Diskettes

Both drives are emulated, the internal one and the external one on the port at
the back. Images named as diskettes go in them in the order given, the
internal drive first:

```bash
# The system diskette in the internal drive, another in the external one
izmac -floppy system.dsk -floppy games.dsk
```

A diskette image is either plain — 409 600 or 819 200 bytes, the sectors one
after another — or a DiskCopy 4.2 file, which is the same thing with a header
in front of it and is what most of the archives hold. Both work, and a
DiskCopy image is written back as a DiskCopy image, checksums and all.

400K and 800K are the only sizes a Macintosh Plus can read. A 720K or 1.44M
image is recognised as a diskette and turned away with a reason rather than
quietly attached to the SCSI bus as a hard disk.

### Putting one in

While the machine runs, **drop the image on the window**. It goes into the
internal drive, unless there is already a diskette there and the external
drive is free, in which case it goes into that one. A line at the top of the
screen says which drive it went into. If both drives are full, the one in the
internal drive is written back and replaced.

### Taking one out

The Macintosh ejects its own diskettes, and that is the way to do it: drag the
disk to the trash, or select it and press Command-E. The machine drives the
eject line, izmac writes the image back, and the System stops believing there
is a disk there.

The **F10 menu** also has a line for each drive that ejects whatever is in it.
That is for a disk the machine has already lost track of, not the everyday way
out: pulling a diskette out from under the System leaves it thinking the disk
is still there, exactly as it would on the real machine.

### Writing and formatting

Writing works, and so does formatting. Give the machine a file of the right
size full of zeros and it will offer to initialize it:

```bash
# An empty 800K diskette
dd if=/dev/zero of=blank.dsk bs=1024 count=800
```

Drop that on the window and the Macintosh says the disk is unreadable and asks
whether to initialize it. Say yes, and you have a formatted, mounted, empty
Macintosh diskette that lives in `blank.dsk`.

A diskette is held whole in memory and the file on the host is rewritten
complete when the drive motor stops, which the driver does a few seconds after
it has finished. So the file follows what the Macintosh believes it has saved,
a moment behind. Ejecting writes it back there and then.

That moment is worth knowing about: closing the window while the drive is
still turning loses whatever has not been written back yet. Give it the couple
of seconds it takes the motor to stop, or eject the diskette, before you quit.

If the file is read only on the host, the machine sees a locked diskette, with
the little tab pushed across. It mounts and reads fine, and the Finder refuses
to change anything on it. The F10 menu says `locked` beside the name.
