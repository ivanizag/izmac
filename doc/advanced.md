# Speed, the clock and the tracers

[Back to the manual](manual.md)

The machine inside izmac keeps its own time, counted in the cycles of its
processor, and nothing in it can see how fast those cycles go by here. That is
what lets it run faster than the real machine, start at a time of your
choosing, and run the same way twice. This page is about those knobs, and
about the tracers that show what the machine is doing.

## Speed

izmac runs at the speed of the Macintosh Plus, 7.8336 MHz, unless told
otherwise:

| | |
|---|---|
| `-speed plus` | the real speed, the default |
| `-speed full` | as fast as your computer can go |
| `-speed 16` | any other clock, in MHz |
| **F5** | full speed on and off while it runs |
| **Ctrl-F5** | print on the terminal the speed being reached |

The title bar of the window shows the speed, and says *full speed* when it
is on.

Nothing inside the machine knows the difference. A second for the machine is
7,833,600 cycles however long they take here, so at full speed everything
that counts time goes faster with it: the clock gains, animations hurry, and
the sound comes out wrong. Faster than the real machine, a click holds it at
its real speed for a second, so that a double click is still one: the machine
measures the time between the two clicks in its own time.

## The clock

The Macintosh keeps the date and time in a clock chip of its own, which izmac
starts at the time of your computer and moves on once a second of the
machine's time. So at full speed the clock gains, and a date set in the
Control Panel or the Alarm Clock holds for as long as izmac runs: the next
run starts from your computer's time again.

| | |
|---|---|
| `-wallclock` | read the time from your computer every time the machine asks. It never gains, at any speed, but the machine can not set it |
| `-starttime "1987-03-02 10:00:00"` | start the clock at that date and time instead of yours, and count the machine's seconds from it |

`-starttime` is what makes a run repeatable. The file server of `-share` goes
by the machine's time always, so with a start time nothing the machine sees
depends on when it runs: two runs with the same disks and the same input do
exactly the same, to the cycle. The pictures of the
[activities](activities/README.md) are made that way, and so are the tests
that check it. The two options can not be used together.

## The tracers

`-trace` turns on tracers, which print on the terminal what the machine is
doing as it does it. Several go together, separated by commas:

| Tracer | What it prints |
|---|---|
| `cpu` | every instruction the processor runs. **F4** turns it on and off while the machine runs |
| `toolbox` | every call to the Toolbox and the operating system, by name |
| `sadmac` | nothing until the machine stops on a Sad Mac, or on any loop it never leaves; then it stops the run, and the headless frontend says what the Sad Mac code means |
| `scsi` | every command on the SCSI bus, with the disk it went to |
| `floppy` | the diskettes going in and out, and the tracks read and written |

```bash
izmac -trace toolbox,floppy mydisk.img
```

The toolbox tracer prints a line a call: the cycle it was made at, its
address, the word of the trap and its name.

```
056975116  $4008c4  $a9a0  GetResource
057022164  $4006ac  $a86e  InitGraf
```

They print a lot. `cpu` above all slows the machine down far more than the
emulation itself does, and `toolbox` prints tens of thousands of lines for a
start-up.

## The headless frontend

The tracers go best with `headless`, the frontend with no window that runs a
number of frames and reports where the machine ended up: the registers, the
instructions around where it stopped, and the screen as a PNG if asked. Its
options are in [Command line options](options.md).

```bash
go run ./frontend/headless -trace sadmac -frames 600 -png screen.png mydisk.img
```

## Profiling izmac itself

`-profile` writes a CPU profile of the emulator, for working on izmac rather
than on the machine. The window frontend prints on the terminal where it
writes it, and `go tool pprof` reads it. The headless frontend takes the
option and does nothing with it.
