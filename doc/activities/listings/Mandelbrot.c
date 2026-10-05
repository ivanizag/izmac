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
#include <Packages.h>

#define MaxIterations 64
#define Four (4L << 16)
#define StatusHeight 16
#define StartWidth 288
#define StartHeight 192

#define AppleID 128
#define FileID 129
#define EditID 130
#define ViewID 131

/* What a window shows: its picture, the point of the plane at its top left,
   how far apart its pixels are, and the next row to draw */
typedef struct {
    BitMap image;
    Fixed left, top, step;
    short row;
} View, *ViewPtr;

MenuHandle appleMenu, editMenu, viewMenu;
short windows;
Boolean quitting, editEnabled, blackAndWhite;
Point lastMouse;

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

/* The 4 by 4 dither: how many of 16 a pixel needs to be black */
short dither[4][4] = {
    {0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5}
};

void DrawRow(WindowPtr window, ViewPtr view)
{
    short width = view->image.bounds.right;
    Ptr bits = view->image.baseAddr + (long) view->row * view->image.rowBytes;
    Fixed ci = view->top - view->step * view->row;
    Rect line;
    short col, n;
    Boolean black;

    for (col = 0; col < view->image.rowBytes; col++)
        bits[col] = 0;
    for (col = 0; col < width; col++) {
        n = Iterations(view->left + view->step * col, ci);
        if (blackAndWhite)
            black = n == MaxIterations;
        else
            black = n == MaxIterations || (n & 15) > dither[view->row & 3][col & 3];
        if (black)
            bits[col >> 3] |= 0x80 >> (col & 7);
    }
    SetRect(&line, 0, view->row, width, view->row + 1);
    SetPort(window);
    CopyBits(&view->image, &window->portBits, &line, &line, srcCopy, nil);
    view->row++;
}

/* Draws the next row of the frontmost window that has rows left to draw */
void DrawSomething(void)
{
    WindowPeek window;
    ViewPtr view;

    for (window = (WindowPeek) FrontWindow(); window != nil; window = window->nextWindow) {
        view = window->windowKind == userKind ? (ViewPtr) window->refCon : nil;
        if (view != nil && view->row < view->image.bounds.bottom) {
            DrawRow((WindowPtr) window, view);
            return;
        }
    }
}

/* The picture made the size of the window, but for the status line, with
   the same point of the plane in its middle, and started again */
void Resize(WindowPtr window, ViewPtr view)
{
    short width = window->portRect.right - window->portRect.left;
    short height = window->portRect.bottom - window->portRect.top - StatusHeight;
    long size;

    view->left -= view->step * ((width - view->image.bounds.right) / 2);
    view->top += view->step * ((height - view->image.bounds.bottom) / 2);

    if (view->image.baseAddr != nil)
        DisposPtr(view->image.baseAddr);
    view->image.rowBytes = (width + 15) / 16 * 2;
    SetRect(&view->image.bounds, 0, 0, width, height);
    size = (long) view->image.rowBytes * height;
    view->image.baseAddr = NewPtr(size);
    while (size > 0)
        view->image.baseAddr[--size] = 0;
    view->row = 0;
}

/* The status line: where the pointer is on the plane, and what that point
   does */
void AppendText(Str255 s, char *text)
{
    while (*text)
        s[++s[0]] = *text++;
}

void AppendDigits(Str255 s, long number, short first)
{
    Str255 digits;
    short i;

    NumToString(number, digits);
    for (i = first; i <= digits[0]; i++)
        s[++s[0]] = digits[i];
}

/* A Fixed in decimal, to five places */
void AppendFixed(Str255 s, Fixed f)
{
    long fraction;

    if (f < 0) {
        AppendText(s, "-");
        f = -f;
    }
    fraction = ((f & 0xFFFFL) * 15625 + 5120) / 10240;
    if (fraction >= 100000) {
        f += 1L << 16;
        fraction -= 100000;
    }
    AppendDigits(s, f >> 16, 1);
    AppendText(s, ".");
    AppendDigits(s, fraction + 100000, 2);
}

void DrawStatus(WindowPtr window, ViewPtr view, Str255 text)
{
    Rect strip = window->portRect;

    SetPort(window);
    strip.top = view->image.bounds.bottom;
    strip.right -= 15;
    EraseRect(&strip);
    MoveTo(strip.left, strip.top);
    LineTo(strip.right, strip.top);
    MoveTo(strip.left + 4, strip.bottom - 4);
    TextFont(geneva);
    TextSize(9);
    DrawString(text);
}

void TrackMouse(void)
{
    WindowPtr window = FrontWindow();
    ViewPtr view;
    Point where;
    Fixed cr, ci;
    short n;
    Str255 text;

    if (window == nil || ((WindowPeek) window)->windowKind != userKind)
        return;
    view = (ViewPtr) GetWRefCon(window);
    SetPort(window);
    GetMouse(&where);
    if (where.h == lastMouse.h && where.v == lastMouse.v)
        return;
    lastMouse = where;
    text[0] = 0;
    if (PtInRect(where, &view->image.bounds)) {
        cr = view->left + view->step * where.h;
        ci = view->top - view->step * where.v;
        n = Iterations(cr, ci);
        AppendFixed(text, cr);
        AppendText(text, "  ");
        AppendFixed(text, ci);
        if (n == MaxIterations)
            AppendText(text, "  in the set");
        else {
            AppendText(text, "  out after ");
            AppendDigits(text, n, 1);
        }
    }
    DrawStatus(window, view, text);
}

/* The grow box, without the lines of the scroll bars there are none of */
void DrawGrowBox(WindowPtr window)
{
    Rect box = window->portRect;
    RgnHandle clip = NewRgn();

    SetPort(window);
    GetClip(clip);
    box.left = box.right - 15;
    box.top = box.bottom - 15;
    ClipRect(&box);
    DrawGrowIcon(window);
    SetClip(clip);
    DisposeRgn(clip);
}

/* A window of its own for a view of the plane, at a place on the screen */
void NewView(Rect *box, Fixed left, Fixed top, Fixed step)
{
    WindowPtr window;
    ViewPtr view;
    Str255 title;

    windows++;
    title[0] = 0;
    AppendText(title, "Mandelbrot ");
    AppendDigits(title, windows, 1);
    window = NewWindow(nil, box, title, true, documentProc, (WindowPtr) -1, true, 0);
    view = (ViewPtr) NewPtr(sizeof(View));
    view->image.baseAddr = nil;
    SetRect(&view->image.bounds, 0, 0, box->right - box->left,
        box->bottom - box->top - StatusHeight);
    view->left = left;
    view->top = top;
    view->step = step;
    SetWRefCon(window, (long) view);
    Resize(window, view);
}

/* The whole set: 3.5 wide, from -2.5, in the window the program starts
   with */
#define StartStep ((7L << 15) / StartWidth)

void WholeSet(ViewPtr view)
{
    view->step = StartStep;
    view->left = -(5L << 15);
    view->top = view->step * (view->image.bounds.bottom / 2);
    view->row = 0;
}

void NewWholeSet(void)
{
    Rect box;
    ViewPtr view;

    SetRect(&box, 0, 0, StartWidth, StartHeight + StatusHeight);
    OffsetRect(&box, (512 - StartWidth) / 2 + windows % 8 * 16, 50 + windows % 8 * 16);
    NewView(&box, 0, 0, 0);
    view = (ViewPtr) GetWRefCon(FrontWindow());
    WholeSet(view);
}

void ZoomOut(ViewPtr view)
{
    if (view->step > StartStep * 4) {
        SysBeep(1);
        return;
    }
    view->left -= view->step * (view->image.bounds.right / 2);
    view->top += view->step * (view->image.bounds.bottom / 2);
    view->step <<= 1;
    view->row = 0;
}

/* The rectangle from a corner towards a point, of the shape of the picture */
void Selection(ViewPtr view, Point from, Point to, Rect *frame)
{
    short width = view->image.bounds.right, height = view->image.bounds.bottom;
    short wide = to.h - from.h, tall = to.v - from.v;
    short size = wide < 0 ? -wide : wide;

    if ((long) (tall < 0 ? -tall : tall) * width / height > size)
        size = (long) (tall < 0 ? -tall : tall) * width / height;
    SetRect(frame, from.h, from.v, from.h + size,
        from.v + (short) ((long) size * height / width));
    if (wide < 0)
        OffsetRect(frame, -size, 0);
    if (tall < 0)
        OffsetRect(frame, 0, -(frame->bottom - frame->top));
}

/* A rectangle dragged in the picture, and a new window with what is in it */
void ZoomIn(WindowPtr window, ViewPtr view, Point from)
{
    Point to, now;
    Rect frame, box;
    short size;

    SetPort(window);
    GlobalToLocal(&from);
    PenMode(patXor);
    to = from;
    Selection(view, from, to, &frame);
    FrameRect(&frame);
    while (StillDown()) {
        GetMouse(&now);
        if (now.h != to.h || now.v != to.v) {
            FrameRect(&frame);
            to = now;
            Selection(view, from, to, &frame);
            FrameRect(&frame);
        }
    }
    FrameRect(&frame);
    PenNormal();

    size = frame.right - frame.left;
    if (size < 4)
        return;
    if (view->step * size / view->image.bounds.right < 4) {
        SysBeep(1);
        return;
    }
    box = window->portRect;
    LocalToGlobal((Point *) &box.top);
    LocalToGlobal((Point *) &box.bottom);
    OffsetRect(&box, 16, 16);
    if (box.bottom > qd.screenBits.bounds.bottom - 4)
        OffsetRect(&box, 0, qd.screenBits.bounds.bottom - 4 - box.bottom);
    if (box.right > qd.screenBits.bounds.right - 4)
        OffsetRect(&box, qd.screenBits.bounds.right - 4 - box.right, 0);
    NewView(&box, view->left + view->step * frame.left,
        view->top - view->step * frame.top,
        view->step * size / view->image.bounds.right);
}

void CloseView(WindowPtr window)
{
    ViewPtr view = (ViewPtr) GetWRefCon(window);

    DisposPtr(view->image.baseAddr);
    DisposPtr((Ptr) view);
    DisposeWindow(window);
}

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
}

/* Every window started again, after a change of how it is drawn */
void RedrawAll(void)
{
    WindowPeek window;

    for (window = (WindowPeek) FrontWindow(); window != nil; window = window->nextWindow)
        if (window->windowKind == userKind)
            ((ViewPtr) window->refCon)->row = 0;
}

void DoMenu(long choice)
{
    short item = LoWord(choice);
    WindowPtr front = FrontWindow();
    ViewPtr view = nil;
    Str255 name;

    if (front != nil && ((WindowPeek) front)->windowKind == userKind)
        view = (ViewPtr) GetWRefCon(front);
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
        if (item == 1)
            NewWholeSet();
        else if (item == 2 && front != nil) {
            if (((WindowPeek) front)->windowKind < 0)
                CloseDeskAcc(((WindowPeek) front)->windowKind);
            else if (view != nil)
                CloseView(front);
        } else if (item == 4)
            quitting = true;
        break;
    case EditID:
        SystemEdit(item - 1);
        break;
    case ViewID:
        if (item == 1 && view != nil)
            ZoomOut(view);
        else if (item == 2 && view != nil)
            WholeSet(view);
        else if (item == 4) {
            blackAndWhite = !blackAndWhite;
            CheckItem(viewMenu, 4, blackAndWhite);
            RedrawAll();
        }
        break;
    }
    HiliteMenu(0);
}

void DoMouseDown(EventRecord *event)
{
    WindowPtr which;
    Rect limits;
    long size;

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
    case inGrow:
        SetRect(&limits, 128, 64 + StatusHeight, qd.screenBits.bounds.right,
            qd.screenBits.bounds.bottom);
        size = GrowWindow(which, event->where, &limits);
        if (size != 0) {
            SizeWindow(which, LoWord(size), HiWord(size), true);
            Resize(which, (ViewPtr) GetWRefCon(which));
            SetPort(which);
            InvalRect(&which->portRect);
        }
        break;
    case inGoAway:
        if (TrackGoAway(which, event->where))
            CloseView(which);
        break;
    case inContent:
        if (which != FrontWindow())
            SelectWindow(which);
        else
            ZoomIn(which, (ViewPtr) GetWRefCon(which), event->where);
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

void Update(WindowPtr window)
{
    ViewPtr view = (ViewPtr) GetWRefCon(window);
    Rect drawn;

    BeginUpdate(window);
    SetPort(window);
    EraseRect(&window->portRect);
    drawn = view->image.bounds;
    if (view->row < drawn.bottom)
        drawn.bottom = view->row;
    CopyBits(&view->image, &window->portBits, &drawn, &drawn, srcCopy, nil);
    DrawStatus(window, view, "\p");
    DrawGrowBox(window);
    EndUpdate(window);
    lastMouse.h = -1;
}

void Setup(void)
{
    MenuHandle menu;

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
    AppendMenu(menu, "\pNew/N;Close/W;(-;Quit/Q");
    InsertMenu(menu, 0);
    editMenu = NewMenu(EditID, "\pEdit");
    AppendMenu(editMenu, "\pUndo/Z;(-;Cut/X;Copy/C;Paste/V;Clear");
    InsertMenu(editMenu, 0);
    DisableItem(editMenu, 0);
    viewMenu = NewMenu(ViewID, "\pView");
    AppendMenu(viewMenu, "\pZoom Out/-;Whole Set/R;(-;Black and White/B");
    InsertMenu(viewMenu, 0);
    DrawMenuBar();

    NewWholeSet();
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
                Update((WindowPtr) event.message);
                break;
            case activateEvt:
                if (((WindowPeek) event.message)->windowKind == userKind)
                    DrawGrowBox((WindowPtr) event.message);
                lastMouse.h = -1;
                break;
            }
        } else {
            TrackMouse();
            DrawSomething();
        }
    }
}
