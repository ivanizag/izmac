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

The modifiers are not where their names suggest:

| Your keyboard | The Macintosh |
|---|---|
| **Command**, **Windows** or **Super**, either side | **Command**, the clover key |
| **Alt** or **Option**, either side | **Command** as well |
| **Control**, either side | **Option** |
| Shift, either side | Shift |
| Backspace | Backspace |
| Enter on the numeric keypad | Enter, the key beside the space bar |

Two keys of yours give the Macintosh command key because your own system
keeps some combinations for itself: Command-Q and Command-Tab on macOS never
reach the machine, and on most Linux desktops the super key belongs to the
window manager. Whatever your system swallows, the same thing with **Alt**
gets through. So Command-C is Command-C, and Alt-C when it is not.

The interrupt and reset that the programmer's switch gave you are not on the
keyboard at all — reset is on the menu.

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
| **F11** | force your clipboard into the machine, see below |
| **F12**, **Print Screen** | write the screen to a file, see below |
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
| **Paste from the host** | hands your clipboard to the machine, the same as F11 |
| **Save a screenshot** | writes the screen as it is now, the same as F12 |
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

## Screenshots

**F12**, or **Print Screen** on a keyboard that has one, writes the screen as
it is now to `izmac_<date>-<time>.png` in the working directory, at the real
512 by 342. *Save a screenshot* on the F10 menu does the same thing.

The name is the moment it was taken, so one never overwrites the last. It is
printed on the terminal as well as over the screen for a moment, since the
line over the screen is gone before there is time to copy a name off it.

What is written is the screen of the Macintosh alone. Neither the menu nor the
line it leaves behind is in the file: the picture comes from the frame buffer
rather than from the window, so you can take a shot from the menu without the
menu being in it.

## Copy and paste

The clipboard of the machine and the clipboard of your own system are shared,
both ways. Text only: a picture copied inside the machine is a PICT, a format
nothing on your system reads, so it is left alone rather than replacing what
you had on your clipboard.

**Copying out of the machine.** Copy as you normally would in the application
— Command-C, or Copy on its Edit menu — and the text is on your clipboard a
moment later, ready to paste anywhere. There is nothing to press on the izmac
side.

**Pasting into the machine.** Copy on your system, then click on the izmac
window. Your clipboard is handed to the machine as the window takes the focus,
so by the time you are back in the Macintosh it is already there, and
Command-V in the application pastes it.

It arrives as the clipboard of the Macintosh and not as a burst of typing, so
it pastes into anything that has a Paste on its Edit menu, keeps its line
breaks, and does not depend on the application having a text field ready.

**F11 forces it**, and so does *Paste from the host* on the F10 menu. That is
for when the window never lost the focus: a clipboard manager, a hotkey, a
script, or a copy inside the machine that has replaced what you sent earlier.

Accented characters and the ones the Macintosh does not have are dealt with on
the way. The machine used Mac OS Roman, one byte a character, so the é of a
modern system arrives as the é of a Macintosh; anything with no Macintosh
equivalent — an em dash, a curly quote from a modern editor, an emoji —
arrives as a question mark rather than stopping the paste.

Two things worth knowing when a copy does not turn up, neither of them izmac
being careful:

- **Some applications keep a clipboard of their own** and only publish it when
  they are switched away from, which under MultiFinder and System 7 means
  clicking on another window inside the machine. Until they do there is
  nothing for izmac to see. Switching to the Finder inside the machine and
  back is enough.
- **The clipboard is sometimes written to a file** instead of being kept in
  memory. The System does that when it needs the space, and a clipboard that
  has gone to disk is not picked up.

To keep the two clipboards apart entirely, start izmac with
`-clipboard=false`.

## Sound

The sound comes out of your default audio device with no setting of its own.
The machine's own volume, in the Control Panel, is the volume you get, and the
startup chime is the first thing you hear.
