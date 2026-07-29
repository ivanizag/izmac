# izmac

A Macintosh Plus emulator written in Go, built on
[iz68000](https://github.com/ivanizag/iz68000).


```bash
go run ./frontend/macebiten mydisk.img
go run ./frontend/macebiten system.img work.img scratch.img
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
| `-pram` | where the parameter RAM is kept, `pram.bin` by default |
| `-ram` | RAM size in Kb, 1024 or 4096 |
| `-speed` | `plus` for the real 7.8336Mhz, `full` for as fast as the host goes, or a number of Mhz |
| `-trace` | tracers to enable: `cpu`, `toolbox`, `sadmac`, `scsi` |

Files named without an option are disk images. What each one
is is worked out from the image itself: a Macintosh hard disk starts with a
driver descriptor map, and a diskette is exactly 400K or 800K. Diskettes are
reported and set aside, since the drives are not emulated yet.

Up to seven disks go on the SCSI bus, taking the ids 0 upwards in the order
given. Note that the options have to come before the file names.

