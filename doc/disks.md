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
diskette, because those are the only sizes these drives make. A file of zeros
is a blank hard disk waiting to be formatted. Anything else is no disk image at
all, and goes on a new volume of its own: see
[Files and folders](#files-and-folders).

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

izmac does not make blank hard disks itself, but a file of zeros is one, and
the Macintosh formats it the way it formatted a new drive: with Apple HD SC
Setup, from the Utilities diskette of the System. The disk answers what HD SC
Setup asks of a drive before it lists it, Apple's own mode page among them, so
it shows up as an Apple drive and can be initialized, given its partitions and
driver, and mounted.

```bash
# An empty 20 MB hard disk
dd if=/dev/zero of=blank.img bs=1048576 count=20
```


### Images with no SCSI driver on them

A real Macintosh boots from a hard disk in two steps. The ROM reads block 0,
finds a *driver descriptor map* there, loads the SCSI driver it names and jumps
into it; the SCSI driver is what then finds the volume and reads it. A disk
that has been through Apple's formatter has all of that in front of the volume,
in the first 96 blocks.

Many of the images you can download do not. They are the HFS volume on its own,
made for Mini vMac and Basilisk II, which patch the ROM so that a driver of
their own stands in. Under an unpatched ROM such an image gives you the
blinking diskette and no explanation.

izmac takes one anyway. What is missing is made up as the disk is attached and
kept in memory in front of the file, so the machine sees a disk 96 blocks
longer than the image, laid out the way a formatter would have written it:

```bash
izmac volume.dsk
```

Nothing is written to the image. Reads and writes past those 96 blocks are the
file, and the volume is the same volume afterwards as it was before — still
good for the emulator it was made for.

The SCSI driver is the one part that cannot be made up, because it is real code
that the ROM jumps into. It is Apple's, so izmac carries none and fetches one
the first time a disk turns out to want it, exactly as it fetches the ROM. What
it keeps is the front of a blank disk that has a SCSI driver on it, saved as
`izmac_hddriver.rom` on the working directory. A machine with nothing but
properly formatted disks on it never goes looking.

To use a SCSI driver you already have rather than the one izmac would fetch,
name a disk image that has one. It is only ever read:

```bash
izmac -scsidriver bootable.img volume.dsk
```

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

### Archives

Most of the old Macintosh software on the web does not come as a disk image
but wrapped for the mail and FTP servers of its day: BinHex, MacBinary,
StuffIt, zip, often several of them one inside the other. izmac takes those as
they come, the way [macprep](https://github.com/mastorak/macprep) prepares them
for Mini vMac, and attaches the disk images it finds inside:

```bash
izmac "Mac System Software 6.0.8.7z"
```

```
Unpacking Mac System Software 6.0.8.7z, a 7-Zip archive
  + Printing Tools.img, an 800Kb diskette, in memory
  + System Tools.img, an 800Kb diskette, in memory
  - Utilities 1.img, left out: both drives are taken
  - Utilities 2.img, left out: both drives are taken
```

BinHex (`.hqx`), MacBinary (`.bin`), zip and gzip are unpacked by izmac
itself. StuffIt in all its versions, Compact Pro, 7-Zip and RAR are handed to
`unar`, from The Unarchiver, which has to be installed for those
(`brew install unar`, or `unar` in most package managers). Without it izmac says
which archive needed it.

The disk images in an archive go where they belong as any other image does,
diskettes in the drives and hard disks on the bus. When there are more than fit
the rest are left out, with a line saying so.

What else is in the archive goes on a new volume, as described in
[Files and folders](#files-and-folders) below. An archive with no disk image
in it, an application or a game the way most of them were published, becomes a
volume of its own:

```
Unpacking boot-editor-104.hqx, a BinHex file
  Packing 2 files on a new volume, boot-editor-104
  + boot-editor-104, an 800Kb diskette, in memory
```

An archive that has disk images in it as well puts only its Macintosh files on
the new volume, the ones with a resource fork or a type and creator, and leaves
out the rest: those are the read me and the checksums meant for the host.

The same goes for a file dropped on the window: the first diskette in it goes
in the drive.

**What is unpacked lives in memory, and is gone when izmac stops.** The
archive is never touched. The Macintosh can write to the images all the same,
but what it saves on them is lost with them. To keep the images, and what is
saved on them, add `-persist`:

```bash
izmac -persist "Mac System Software 6.0.8.7z"
```

They are then written to the working directory, named after the archive, and
used from there like any other image: `izmac_Game.dsk` for an archive with one
image in it, `izmac_Mac System Software 6.0.8 - System Tools.dsk` for one of
several. The next run with `-persist` goes straight to them, without unpacking
the archive at all, so what the Macintosh saved on them is kept rather than
unpacked over. Delete them to start again from the archive, or name them
directly to leave the archive out of it.

izmac remembers which kept image came from which archive in
`izmac_kept.json`, next to them. That is how it knows to skip the unpacking,
and how two archives of the same name in different places are kept apart: the
second one's images are numbered, `izmac_Game 2.dsk`. An archive is known by
where it is, so one that is moved is unpacked again, under a numbered name of
its own; the images of the old place stay where they are until deleted.

A diskette image that has picked up some padding on its travels, 401K or 807K
rather than 400K or 800K, is mended on the way in: the volume inside is checked
to end where the diskette would, and the rest is dropped. That happens in
memory too, and is kept the same way with `-persist`.

### Files and folders

A folder of the host, or a file that is neither a disk image nor an archive,
goes on a new volume made for it, named after it:

```bash
izmac System.img ~/Documents/Letters
```

The new volume is an 800K diskette when everything fits on one, so that it can
be swapped in and out while the machine runs, and a hard disk with a megabyte
or so to spare when it does not. A hard disk made this way goes on the bus with
a SCSI driver made up in front of it, the way a bare volume does. Neither kind
is a startup disk: that takes boot blocks, which come with a System and not
with the files.

Whatever made the files Macintosh files is kept: the resource fork, where an
application keeps most of itself, the type and creator that give a document
its icon and its application, and the folders they were in. Out of an archive
those come from the archive. On the host they come from wherever the host keeps
them: macOS keeps them with the file, and any system keeps them in the `._`
files macOS leaves beside the others on a USB stick. A file with none of them
arrives as a plain document with no type, which TeachText can still open if it
is text.

A folder or a file dropped on the window goes the same way, into a drive when
it fits on a diskette.

A diskette like this has no boot blocks, and the machine looks in the drives
first when it starts, ejecting any diskette it cannot start from. So a
diskette named on the command line that does not start the machine, a new
volume or any other diskette of documents and applications, goes in its drive
once the Finder is running instead, as though you had put it in then. The
machine starts from the hard disk, or from a startup diskette in the other
drive, and the diskette appears on the desktop a moment later.

The Finder sees the files as new ones, the way it sees files copied from
another disk: it places their icons in the windows itself and reads the icons
of the applications out of them.

Like an unpacked image, the volume lives in memory and is gone when izmac
stops, unless it is kept with `-persist`. Either way it is a copy: what the
Macintosh writes to it does not reach the files it was made from. With
`-persist` the folder is read once, on the first run, and the next runs use the
kept volume without looking at the folder again, so a change to the folder
shows only once the kept volume is deleted.

### When you name nothing at all

A machine with no disk in it sits on the blinking diskette forever, so izmac
does not leave you there. If the command line names no image, neither a hard
disk nor a diskette, it fetches one the first time and puts it in the internal
drive:

```bash
izmac
```

What it fetches is MacPaint 1.5, a 400K startup diskette with System 2.0 and
Finder 2.2 on it, and it is saved as `izmac_macpaint.dsk` on the working
directory, the way the ROM is. It is downloaded once and used from the file
after that, and naming any image of your own is enough to stop it happening at
all.

The diskette is written back like any other, so what you draw and save on it
stays on it. Delete the file and the next run with nothing named fetches a
fresh copy.

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
a moment behind. Ejecting writes it back there and then, and so does closing
the window, for a diskette whose drive was still turning.

If the file is read only on the host, the machine sees a locked diskette, with
the little tab pushed across. It mounts and reads fine, and the Finder refuses
to change anything on it. The F10 menu says `locked` beside the name.
