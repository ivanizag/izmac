# Exploring the Mandelbrot set with MPW

[Back to the activities](README.md)

**MPW**, the Macintosh Programmer's Workshop, was Apple's own way of making
Macintosh software, sold to developers through APDA, Apple's programmers'
association. It looked nothing like the rest of the Macintosh: its windows are
text, and in any of them a line typed and sent with the Enter key is a command,
run there and then, with its answer written below it. The commands are the
compilers for C, Pascal and assembly language, the linker, and tools for
everything else, and they can be put together into scripts. Apple made its
own software with it.

The **Mandelbrot set** came into fashion in the same years, after *Scientific
American* showed its readers how to draw it, in August 1985. A point of the
plane belongs to it if a simple sum, repeated, never runs away to infinity;
coloured by how quickly it does run away, the edge of the set is a coastline
that never ends, with smaller copies of the whole along it. Every owner of a
computer who could program drew it, and waited for it.

This page writes a Mandelbrot set explorer in C with MPW 3.1, of 1989: it
draws the set, and a rectangle dragged over it is drawn again, closer.

## What you need

- **`system6.dsk`**, System 6 on a diskette with the AppleShare part of the
  Chooser on it, from izmac's repository: open
  [test_images/system6.dsk](https://github.com/ivanizag/izmac/blob/main/test_images/system6.dsk)
  and click *Download raw file*.
- **A hard disk.** MPW needs one, and two megabytes of memory. The one made
  in [Installing System 6 on a hard disk](installing.md), `blank.img`, is the
  one used here. Only its space is needed: the Macintosh starts from
  `system6.dsk`, because the System installed on the hard disk has no
  AppleShare to reach the file server with.
- **MPW**, on the CD Apple sent its developers in 1989:
  `mpw3-cdrom.zip`, *MPW 3.0/3.1 CD for System 6*, from the
  [Macintosh Repository](https://www.macintoshrepository.org/545-macintosh-programmer-s-workshop-mpw-3-x),
  or from izmac's repository,
  [test_images/mpw3-cdrom.zip](https://github.com/ivanizag/izmac/blob/main/test_images/mpw3-cdrom.zip).
  There is no need to unpack it: izmac takes the zip as it is.
- **A folder to share**, called *For the Mac*, where the program will be
  written, with any text editor of your computer.

## Install MPW

1. **Start izmac** with four megabytes, the System diskette, the hard disk,
   the CD and the folder:

   ```bash
   izmac -ram 4096 -share "For the Mac" system6.dsk blank.img mpw3-cdrom.zip
   ```

   izmac takes the image of the CD out of the zip and puts it on the SCSI
   bus, as a CD-ROM drive would be. The CD is locked, as a CD is: nothing can
   be written to it.

   ![The desktop](images/mpw/desktop.png)

2. **Open MPW CD-ROM.** Two versions of MPW, 3.0 and 3.1, MacsBug, the
   debugger that takes over the whole machine, SADE, the one that works in a
   window, and ResEdit.

   ![The CD](images/mpw/cd.png)

3. **Drag MPW 3.1 onto the icon of Macintosh HD.** The Finder copies it, file
   by file: about three minutes on a Macintosh Plus.

   ![Copying MPW](images/mpw/copying.png)

## The program

4. **Write the program in a file, `Mandelbrot.c`, in the folder *For the
   Mac*.** The blocks of C of this section, one after the other, are the whole
   of it.

The program begins with the Toolbox's headers, which MPW has in its
*Interfaces* folder, the sizes it draws at and the numbers of its menus. The
picture is 288 pixels by 192, kept in a bitmap of its own, `image`, and seen
through the window. `left`, `top` and `step` say what part of the plane it
shows: the point at its top left, and how far apart its pixels are.

```c
#include <Types.h>
#include <Memory.h>
#include <Quickdraw.h>
#include <Fonts.h>
#include <Events.h>
#include <OSEvents.h>
#include <Menus.h>
#include <Windows.h>
#include <TextEdit.h>
#include <Dialogs.h>
#include <Desk.h>
#include <OSUtils.h>
#include <ToolUtils.h>

#define Width 288
#define Height 192
#define RowBytes (Width / 8)
#define MaxIterations 64
#define Four (4L << 16)

#define AppleID 128
#define FileID 129
#define EditID 130
#define ViewID 131

WindowPtr window;
MenuHandle appleMenu, editMenu;
BitMap image;
Fixed left, top, step;
short row;
Boolean quitting, editEnabled;
```

The heart of it is how many times the sum goes round for a point before it
runs away: *z* starts at the point *c*, and becomes *z*² + *c*, until it is
further than 2 from 0, or 64 times have gone by and the point is taken as part
of the set. The Plus has no hardware for numbers with decimals, and doing them
in software, with SANE, the Macintosh's maths package, would take hours for a
picture. So the numbers are the Toolbox's `Fixed`, 16 bits for the whole part
and 16 for the fraction, which add as whole numbers do, and multiply with
`FixMul`, in the ROM. The points inside the big heart shape of the set and
the circle to its left would go round all 64 times for nothing: two sums tell
them apart first.

```c
short Iterations(Fixed cr, Fixed ci)
{
    Fixed zr = cr, zi = ci, zr2, zi2, x, q;
    short n;

    /* Inside the main cardioid or the bulb to its left */
    x = cr - (1L << 14);
    zi2 = FixMul(ci, ci);
    q = FixMul(x, x) + zi2;
    if (FixMul(q, q + x) < zi2 >> 2)
        return MaxIterations;
    x = cr + (1L << 16);
    if (FixMul(x, x) + zi2 < 1L << 12)
        return MaxIterations;

    for (n = 0; n < MaxIterations; n++) {
        zr2 = FixMul(zr, zr);
        zi2 = FixMul(zi, zi);
        if (zr2 + zi2 > Four)
            return n;
        zi = (FixMul(zr, zi) << 1) + ci;
        zr = zr2 - zi2 + cr;
    }
    return MaxIterations;
}
```

A row of the picture is worked out a pixel at a time, into the bitmap, and
copied onto the window with `CopyBits`. The points of the set are black. The
others are dithered: the number of times the sum went round, from 0 to 15, is
how many dots of 16 are black around it, so that the grays draw the bands
around the set, the lightest furthest away. A screen of only black and white
had to do grays this way.

```c
/* The 4 by 4 dither: how many of 16 a pixel needs to be black */
short dither[4][4] = {
    {0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5}
};

void DrawRow(void)
{
    Ptr bits = image.baseAddr + (long) row * RowBytes;
    Rect line;
    Fixed ci = top - step * row;
    short col, n;

    for (col = 0; col < RowBytes; col++)
        bits[col] = 0;
    for (col = 0; col < Width; col++) {
        n = Iterations(left + step * col, ci);
        if (n == MaxIterations || (n & 15) > dither[row & 3][col & 3])
            bits[col >> 3] |= 0x80 >> (col & 7);
    }
    SetRect(&line, 0, row, Width, row + 1);
    SetPort(window);
    CopyBits(&image, &window->portBits, &line, &line, srcCopy, nil);
    row++;
}
```

Where the picture is: the whole set at the start, 3.5 wide from -2.5, twice
as far at each Zoom Out, and the rectangle dragged for a zoom in. The
rectangle is drawn with the pen in `patXor` mode, which draws by inverting:
drawn again in the same place, it is gone, and what was under it is back. It
keeps the shape of the window, three wide to two high, whichever way it is
dragged. A `Fixed` has no more than 16 bits of fraction, so a zoom that would
put the pixels closer than 4 of 65536 apart beeps instead.

```c
/* The whole set: 3.5 wide, from -2.5 */
#define StartStep ((7L << 15) / Width)

void StartOver(void)
{
    step = StartStep;
    left = -(5L << 15);
    top = step * (Height / 2);
    row = 0;
}

void ZoomOut(void)
{
    left -= step * (Width / 2);
    top += step * (Height / 2);
    step <<= 1;
    if (step > StartStep)
        StartOver();
    row = 0;
}

/* The rectangle from a corner towards a point, as wide as the window is to
   its height, three to two */
void Selection(Point from, Point to, Rect *frame)
{
    short wide = to.h - from.h, tall = to.v - from.v;
    short size = wide < 0 ? -wide : wide;

    if ((tall < 0 ? -tall : tall) * 3 / 2 > size)
        size = (tall < 0 ? -tall : tall) * 3 / 2;
    SetRect(frame, from.h, from.v, from.h + size, from.v + size * 2 / 3);
    if (wide < 0)
        OffsetRect(frame, -size, 0);
    if (tall < 0)
        OffsetRect(frame, 0, -(size * 2 / 3));
}

void ZoomIn(Point from)
{
    Point to;
    Rect frame;
    short size;

    SetPort(window);
    GlobalToLocal(&from);
    PenMode(patXor);
    to = from;
    Selection(from, to, &frame);
    FrameRect(&frame);
    while (StillDown()) {
        Point now;

        GetMouse(&now);
        if (now.h != to.h || now.v != to.v) {
            FrameRect(&frame);
            to = now;
            Selection(from, to, &frame);
            FrameRect(&frame);
        }
    }
    FrameRect(&frame);
    PenNormal();

    size = frame.right - frame.left;
    if (size < 4)
        return;
    if (step * size / Width < 4) {
        SysBeep(1);
        return;
    }
    left += step * frame.left;
    top -= step * frame.top;
    step = step * size / Width;
    row = 0;
}
```

The rest is what every Macintosh program had to do for itself. The menus are
handled here: the Apple menu with *About Mandelbrot* and the desk
accessories, the File menu, the Edit menu for the desk accessories, which is
dimmed while the program's window is in front, and the View menu. A click is
taken where it fell: on the menu bar, on the title bar to drag the window, on
its close box to quit, or in the picture to zoom.

```c
void ShowAbout(void)
{
    Rect box;
    WindowPtr about;

    HiliteMenu(0);
    SetRect(&box, 96, 110, 416, 190);
    about = NewWindow(nil, &box, "\p", true, dBoxProc, (WindowPtr) -1, false, 0);
    SetPort(about);
    MoveTo(20, 30);
    DrawString("\pMandelbrot, written in MPW C.");
    MoveTo(20, 55);
    DrawString("\pDrag a rectangle in it to zoom in.");
    while (!Button())
        SystemTask();
    while (Button())
        ;
    FlushEvents(everyEvent, 0);
    DisposeWindow(about);
    SetPort(window);
}

void DoMenu(long choice)
{
    short item = LoWord(choice);
    Str255 name;

    switch (HiWord(choice)) {
    case AppleID:
        if (item == 1)
            ShowAbout();
        else {
            GetItem(appleMenu, item, name);
            OpenDeskAcc(name);
        }
        break;
    case FileID:
        quitting = true;
        break;
    case EditID:
        SystemEdit(item - 1);
        break;
    case ViewID:
        if (item == 1)
            ZoomOut();
        else
            StartOver();
        break;
    }
    HiliteMenu(0);
}

void DoMouseDown(EventRecord *event)
{
    WindowPtr which;
    Rect limits;

    switch (FindWindow(event->where, &which)) {
    case inMenuBar:
        DoMenu(MenuSelect(event->where));
        break;
    case inSysWindow:
        SystemClick(event, which);
        break;
    case inDrag:
        limits = qd.screenBits.bounds;
        InsetRect(&limits, 4, 4);
        DragWindow(which, event->where, &limits);
        break;
    case inGoAway:
        if (TrackGoAway(which, event->where))
            quitting = true;
        break;
    case inContent:
        if (which != FrontWindow())
            SelectWindow(which);
        else
            ZoomIn(event->where);
        break;
    }
}

void UpdateEditMenu(void)
{
    WindowPeek front = (WindowPeek) FrontWindow();
    Boolean forAccessory = front != nil && front->windowKind < 0;

    if (forAccessory != editEnabled) {
        editEnabled = forAccessory;
        if (editEnabled)
            EnableItem(editMenu, 0);
        else
            DisableItem(editMenu, 0);
        DrawMenuBar();
    }
}
```

And the start: the managers of the Toolbox started one by one, the menus made
in the program rather than in a resource file, and the window. Then the
*event loop*: when nothing has happened, it draws the next row, so that the
picture builds up a row at a time while the menus and the mouse still work,
and a zoom can start before the picture is finished.

```c
void Setup(void)
{
    Rect box;
    MenuHandle menu;
    long i;

    InitGraf(&qd.thePort);
    InitFonts();
    InitWindows();
    InitMenus();
    TEInit();
    InitDialogs(nil);
    InitCursor();
    FlushEvents(everyEvent, 0);

    appleMenu = NewMenu(AppleID, "\p\024");
    AppendMenu(appleMenu, "\pAbout Mandelbrot...;(-");
    AddResMenu(appleMenu, 'DRVR');
    InsertMenu(appleMenu, 0);
    menu = NewMenu(FileID, "\pFile");
    AppendMenu(menu, "\pQuit/Q");
    InsertMenu(menu, 0);
    editMenu = NewMenu(EditID, "\pEdit");
    AppendMenu(editMenu, "\pUndo/Z;(-;Cut/X;Copy/C;Paste/V;Clear");
    InsertMenu(editMenu, 0);
    DisableItem(editMenu, 0);
    menu = NewMenu(ViewID, "\pView");
    AppendMenu(menu, "\pZoom Out/-;Start Over/R");
    InsertMenu(menu, 0);
    DrawMenuBar();

    image.rowBytes = RowBytes;
    SetRect(&image.bounds, 0, 0, Width, Height);
    image.baseAddr = NewPtr((long) RowBytes * Height);
    for (i = 0; i < (long) RowBytes * Height; i++)
        image.baseAddr[i] = 0;

    SetRect(&box, 0, 0, Width, Height);
    OffsetRect(&box, (512 - Width) / 2, 60);
    window = NewWindow(nil, &box, "\pMandelbrot", true, noGrowDocProc,
        (WindowPtr) -1, true, 0);
    StartOver();
}

main()
{
    EventRecord event;

    Setup();
    while (!quitting) {
        SystemTask();
        UpdateEditMenu();
        if (GetNextEvent(everyEvent, &event)) {
            switch (event.what) {
            case mouseDown:
                DoMouseDown(&event);
                break;
            case keyDown:
            case autoKey:
                if (event.modifiers & cmdKey)
                    DoMenu(MenuKey(event.message & charCodeMask));
                break;
            case updateEvt:
                BeginUpdate((WindowPtr) event.message);
                CopyBits(&image, &window->portBits, &image.bounds,
                    &image.bounds, srcCopy, nil);
                EndUpdate((WindowPtr) event.message);
                break;
            }
        } else if (row < Height)
            DrawRow();
    }
}
```

5. **Change the ends of the lines** of the file to the ones of the Macintosh.
   Computers of today end a line with a *line feed*, and the Macintosh with a
   *carriage return*. MPW's C takes a file with line feeds as one long line,
   and compiles nothing out of it, without a word. On macOS or Linux, in a
   terminal on the folder *For the Mac*:

   ```bash
   perl -pi -e 's/\r?\n/\r/g' Mandelbrot.c
   ```

   On Windows, in PowerShell:

   ```powershell
   (Get-Content Mandelbrot.c -Raw) -replace "\r?\n", "`r" | Set-Content -NoNewline Mandelbrot.c
   ```

## Build it

6. **Mount the folder**: choose *Chooser* from the Apple menu, click
   *AppleShare*, then your computer, then *OK*, and log in as a *Guest*.
   Choose *For the Mac*, click *OK* and close the Chooser.
   [Your computer as a file server](file-server.md) tells this step by step.

7. **Open Macintosh HD and its MPW 3.1 folder, and double-click MPW Shell.**
   Its window is the *Worksheet*, a text file kept from one session to the
   next, where commands are typed. It starts with a few of them and what they
   say.

   ![The Worksheet](images/mpw/worksheet.png)

8. **Select all of it and type the commands**, one at a time, each sent with
   **Enter**: the Enter of the numeric keypad, or Command and Return together
   on a keyboard with no keypad. Return alone only starts a new line, as in
   any text.

   ```
   Directory "For the Mac:"
   C Mandelbrot.c
   Link -o Mandelbrot Mandelbrot.c.o "{Libraries}"Interface.o "{CLibraries}"CRuntime.o "{CLibraries}"CInterface.o -t APPL -c '????'
   ```

   *Directory* makes the folder the one the commands work in: `For the Mac:`
   is the folder, a volume of its own on the desktop, named with the colon
   that ends a volume's name on the Macintosh. *C* compiles the program into
   `Mandelbrot.c.o`, in about half a minute. *Link* puts that together with three
   libraries of MPW: the Toolbox's calls, the start of a C program, and its
   glue to the Toolbox. `{Libraries}` and `{CLibraries}` are variables of the
   Shell, the folders they are in. `-t APPL` makes the file an application,
   and `-c '????'` gives it no creator of its own. The commands say nothing
   when all goes well; a mistake in the program stops the compiler with the
   line and what is wrong with it.

   ![Built](images/mpw/built.png)

   The program is in the folder *For the Mac* on your computer too, next to
   its source: the compiler and the linker wrote there, over the network.

## Explore

9. **Type `Mandelbrot` and press Enter.** A command that is not one of the
   Shell's is a program, which it runs. The picture comes a row at a time,
   in about three minutes on a Macintosh Plus, shown here ten times as fast.

   ![The Mandelbrot set](images/mpw/drawing.gif)

10. **Drag a rectangle around the left end of the line** that runs out of the
    set. Its outline follows the mouse, and when the button is let go, that
    part is drawn again, filling the window.

    ![Zooming in](images/mpw/selecting.png)

    Along the line are smaller blobs, which are not blobs.

    ![The antenna](images/mpw/antenna.png)

11. **Drag a rectangle around the black one in the middle.** It is the whole
    set again, a copy many times smaller, with its own bands around it. It
    takes about eleven minutes: the black is most of the picture now, and
    every point of it goes round all 64 times.

    ![A copy of the set](images/mpw/copy.png)

    *Zoom Out* in the View menu, or Command and the minus key, goes back out,
    and *Start Over*, or Command-R, to the whole set.

12. **Choose Quit from the File menu**, or press Command-Q. The MPW Shell
    comes back, ready for the next change to the program.

## What next

The same MPW compiles Pascal, with `Pascal` in place of `C`, and assembly
language, with `Asm`, and its *Examples* folder has the sample programs Apple
gave its developers, each with the commands that build it. The MPW 3.0
manuals are on the Internet Archive, starting with
[the Reference](https://archive.org/details/bitsavers_applemacdenceVolume11988_30969146).
[Writing a game in THINK Pascal](think-pascal.md) is the other way programs
were written, with the editor, the compiler and the debugger in one
application.
