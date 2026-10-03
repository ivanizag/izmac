# Test images

The ROM and the disks the end to end tests run on. They are here so that every
test runs on a fresh checkout and none skips for want of a file. They are only
read by the tests, through `testImages_test.go`, never by the program, and no
test changes them: each machine gets a copy of its own.

| Image | Size | What is on it |
|---|---|---|
| `macplus.rom` | 128K | The Macintosh Plus ROM v3, checksum 4D1F8172, the revision izmac targets |
| `system6.dsk` | 800K | A startup diskette of System 6.0.8, with the System of the Utilities 1 diskette, which has the drivers AppleShare needs and only the Chooser for desk accessory. AppleShare, MultiFinder, TeachText and a text file, *Read Me* |
| `system6.img` | 2M | System 6.0.8 on a bare HFS volume, no partition map and no driver, with the desk accessories in its System: Alarm Clock, Calculator, Chooser, Control Panel, Find File, Key Caps and Scrapbook. TeachText, *Read Me* and an empty folder |
| `system7.img` | 4M | System 7.1.2 on a partitioned disk with the Apple driver. Chooser, Note Pad, Calculator, Key Caps and Scrapbook; AppleShare and File Sharing, with Sharing Setup and Users & Groups; TeachText, *Read Me* and a folder, *Shared* |
| `macpaint.dsk` | 400K | MacPaint 1.5 with System 2.0 and Finder 2.2, the diskette izmac starts when nothing is named |
| `hddriver.img` | 64K | A blank disk formatted by Apple's HD SC Setup: the SCSI driver a bare volume such as `system6.img` borrows |

The System 6 hard disk is bare and the System 7 one partitioned on purpose,
so that both ways a disk reaches the bus are tested. System 7 does not fit on
a diskette the Plus can read, which is why it is on a hard disk at all.

The bootable ones have been started once and shut down, so the Finder's
desktop file is already on them and the tests do not wait for it to be built.

## Making them again

`build.sh` makes them from the disks they were taken from, with hfsutils, and
says which those are. Then each is started once, in place:

```bash
test_images/build.sh
IZMAC_SETTLE_TEST_IMAGES=1 go test -run TestSettleTestImages .
```

The positions the tests click at depend on what is on the disks: a desk
accessory more in a System, or a file more on a disk, moves things the tests
look for.
