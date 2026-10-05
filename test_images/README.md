# Test images

The ROM and the disks the end to end tests run on. They are here so that every
test runs on a fresh checkout and none skips for want of a file. They are only
read by the tests, through `testImages_test.go`, never by the program, and no
test changes them: each machine gets a copy of its own.

| Image | Size | What is on it |
|---|---|---|
| `macplus.rom` | 128K | The Macintosh Plus ROM v3, checksum 4D1F8172, the revision izmac targets |
| `system6.dsk` | 800K | A startup diskette of System 6.0.8, with the System of the Utilities 1 diskette, which has the drivers AppleShare needs and only the Chooser for desk accessory. AppleShare, MultiFinder, TeachText and a text file, *Read Me* |
| `system6.img` | 2M | System 6.0.8 on a bare HFS volume, no partition map and no driver, with the desk accessories in its System: Alarm Clock, Calculator, Chooser, Control Panel, Find File, Key Caps and Scrapbook, the Key Layout Key Caps needs, the Keyboard, Mouse and Sound control panels, and the ImageWriter driver. TeachText, *Read Me* and an empty folder |
| `system7.img` | 4M | System 7.1.2 on a partitioned disk with the Apple driver. Chooser, Note Pad, Calculator, Key Caps and Scrapbook; AppleShare and File Sharing, with Sharing Setup and Users & Groups; TeachText, *Read Me* and a folder, *Shared* |
| `macpaint.dsk` | 400K | MacPaint 1.5 with System 2.0 and Finder 4.1, the diskette izmac starts when nothing is named |
| `system41.dsk` | 800K | System 4.1 and the Finder 5.5, of 1987, on a startup diskette and nothing else |
| `teachtext.bin` | 19K | TeachText in MacBinary, an application as the archives keep them |
| `hddriver.img` | 64K | The start of a disk formatted by Apple's HD SC Setup, its partition map and driver: the SCSI driver a bare volume such as `system6.img` borrows |
| `system-tools.dsk`, `utilities-1.dsk`, `utilities-2.dsk`, `printing-tools.dsk` | 800K each | The four diskettes System 6.0.8 came on, as they were: the Installer, HD SC Setup, Font/DA Mover and the printer drivers |

The disks and archives of the [activities](../doc/activities/README.md) are
kept as they were downloaded, each the one the activity sends its reader to,
so that the pictures it shows are what the reader will see:

| Image | Size | What is on it |
|---|---|---|
| `macwrite.dsk` | 400K | MacWrite 4.5 with System 2.0, *About MacWrite* and a *Sample Memo* |
| `multiplan.dsk` | 400K | Microsoft Multiplan 1.11, with its help file |
| `basic.dsk` | 400K | Microsoft BASIC 2.0 and its sample programs, in DiskCopy 4.2 |
| `hypercard.dsk` | 800K | HyperCard 1.1, its Startup diskette, which starts it with no Finder |
| `resedit.sit` | 426K | ResEdit 2.1, the diskette of the book *ResEdit Complete*, in StuffIt |
| `bolo.sit` | 731K | Bolo 0.99.7, the tank game, in StuffIt, as the Tucows archive kept it |
| `loderunner.dsk` | 400K | Lode Runner |
| `darkcastle.dsk` | 800K | Dark Castle 1.2, which starts the game with no Finder |
| `thinkpascal-1.dsk`, `thinkpascal-2.dsk` | 800K each | The first two of the four diskettes of THINK Pascal 4.0: the application, and the archive of its interfaces and libraries |

The System 6 hard disk is bare and the System 7 one partitioned on purpose,
so that both ways a disk reaches the bus are tested. System 7 does not fit on
a diskette the Plus can read, which is why it is on a hard disk at all.

The bootable ones made here, the System disks, have been started once and
shut down, so the Finder's desktop file is already on them and the tests do
not wait for it to be built. Those kept as they were downloaded have not.

## Making them again

`build.sh` makes them with hfsutils, python3, unzip and unar, from disks it
downloads from the Internet Archive: the Plus ROM of the Macintosh ROM
archive, the MacPaint diskette izmac itself starts, and the hard disk, the
supplement disk and the diskettes of the
[MacPack](https://archive.org/details/macpack), a pack of software for the
Macintosh Plus core of MiSTer. The software of the activities comes from the
Internet Archive too, its Macintosh software and its copy of the Tucows
archive, but for THINK Pascal, which comes from WinWorld. The downloads, about
485 MB, are kept in
`~/.cache/izmac-test-images` and checked against the SHA-256 they had when the
images were made. Then each image is started once, in place:

```bash
test_images/build.sh
IZMAC_SETTLE_TEST_IMAGES=1 go test -run TestSettleTestImages ./e2e_tests
```

The positions the tests click at depend on what is on the disks: a desk
accessory more in a System, or a file more on a disk, moves things the tests
look for. After making them again, run all the tests.
