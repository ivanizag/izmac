# Keyboard, mouse and the menu

[Back to the manual](manual.md)

## The mouse

The Macintosh mouse is a relative device: it tells the machine how far it has
moved and never where it is, and the ROM keeps the pointer itself. There is no
way to put the pointer somewhere, only to push it.

That is why izmac captures the pointer. **Click on the window** and your own
pointer disappears; movement goes to the machine, and it keeps coming when you
reach the edge of your screen. **Right click**, or press **Escape**, to get it
back — the Macintosh mouse has one button and its keyboard has no escape key,
so neither is anything the machine wants. Opening the F10 menu also releases
it, since the menu needs a pointer to click with.

The title bar always says which way round you are: *click to use the mouse* or
*right click releases the mouse*.

The one pointer of the machine may not land where yours is. It is the
machine's pointer, moved by the same amount as yours, so the two drift apart —
that is how the hardware works and not something izmac hides.

## The keyboard

Keys are delivered as key presses and releases rather than as characters,
because that is what the hardware sends and the ROM does its own translation.
The layout you get is the United States one the Macintosh keyboard had,
whatever your host keyboard is set to. On a non-US layout, the letters and
digits are where you expect and the punctuation may not be.

Two keys are not where their names suggest:

| Your keyboard | The Macintosh |
|---|---|
| **Alt** or **Option**, either side | **Command**, the clover key |
| **Control**, either side | **Option** |
| Shift, either side | Shift |
| Backspace | Backspace |
| Enter on the numeric keypad | Enter, the key beside the space bar |

The command key is taken from Alt rather than from your own command or
Windows key because window managers tend to keep that one for themselves.

So Command-Q is Alt-Q, Command-S is Alt-S, and the interrupt and reset that
the programmer's switch gave you are not on the keyboard at all — reset is on
the menu.

**What is missing.** The arrow keys and the numeric keypad are not mapped, and
neither is Escape, which is used to release the mouse. The Macintosh Plus
keyboard had no arrow keys in the main block either, and software of the
period is built to be driven by the mouse, but a program that wants the arrows
or the keypad cannot be given them.

## The emulator keys

These drive izmac rather than the Macintosh. They are function keys because
the Macintosh keyboard has none of them and so cannot want them.

| Key | What it does |
|---|---|
| **F10** | open and close the menu |
| **F5** | run as fast as your machine can, and back |
| **Ctrl-F5** | print on the terminal the speed being reached |
| **F4** | show or hide the trace of the processor on the terminal |
| **Ctrl-F2** | reset, as the programmer's switch did |
| **Pause** | stop the machine, and let it go again |

Pause is the key of that name on a PC keyboard; Apple keyboards do not have
one, and there is no other way to pause. The title bar says `PAUSED` while the
machine is stopped, and the screen holds the last frame.

## The menu

**F10** puts a small menu over the screen. It is drawn by izmac rather than
being a menu of your window manager, and it is nothing to do with the menu bar
of the Macintosh at the top of the screen.

Move through it with the **arrow keys** and choose with **Enter** or
**Space**, or point at a line and click. **Escape** or **F10** closes it. The
lines are:

| Line | What it does |
|---|---|
| **Full speed** / **Normal speed** | the same as F5 |
| **Save a screenshot** | writes the screen as it is now to `izmac-<date>-<time>.png` in the working directory, at the real 512 by 342 |
| **Reset** | restarts the machine, as the programmer's switch did. Anything unsaved is lost |
| **internal drive** | what is in the internal diskette drive, and eject it |
| **external drive** | the same for the external drive |
| **Close this menu** | closes it |

The drive lines name the diskette in each drive, with `locked` after it if the
image is read only, or say `empty`. Choosing one ejects — see
[Disks and diskettes](disks.md) for when to use that and when to let the
Macintosh do it.

While the menu is up the machine sees neither the keys nor the pointer, and
anything you were holding down is released, so it does not stay held while you
are looking at the menu.

## Sound

The sound comes out of your default audio device with no setting of its own.
The machine's own volume, in the Control Panel, is the volume you get, and the
startup chime is the first thing you hear.
