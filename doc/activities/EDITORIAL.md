# Writing activities

How the pages in this folder are made, and what makes them good. This is
mainly for the language models that write new activities, and for whoever
reviews their work. Nothing links here on purpose: it is not documentation
for izmac's users.

## What an activity is

An activity is one thing a Macintosh Plus owner really did in the second half
of the eighties. It is walked through step by step, and the reader sees on the
way what the machine showed back then. Its parts are:

- a page, `doc/activities/<name>.md`;
- an entry on [README.md](README.md), with a title, a picture that links to
  the page, and a paragraph;
- its images, in `doc/activities/images/<name>/`. They are made by running the
  machine, never drawn or edited by hand.

The list of activities is linked from the "Things to do" row of
`doc/manual.md`. A new activity only needs an entry in `README.md`.

## How to make one

1. **Propose before writing.** List candidate activities with a line each, and
   let the user choose. Write only the ones chosen.
2. **Do the activity on the emulator first**, in `activities_test.go`, and
   look at every image it makes. Write the text from what the machine really
   does, not from memory of what it did.
3. **Add a function to the generator**: `<name>Screenshots(t *testing.T)` in
   `activities_test.go`, run from `TestActivityScreenshots` with `t.Run`. It
   drives the machine with the e2e test helpers, such as:
   - `buildTestMac`, `waitForApplication`, `waitUntil`;
   - `moveMouseTo`, `clickMouse`, `doubleClickAt`, `dragTo`;
   - `pressKey`, `pressCommand`, `typeKeys`, `typeText`.

   Then it takes pictures with `screenshot`, `screenshotOf` (which adds a
   label), `writeScreenshot`, or a `record(...)` recording.
4. **Generate the images:**

   ```bash
   IZMAC_ACTIVITY_SCREENSHOTS=1 go test -count=1 -run 'TestActivityScreenshots/<name>' .
   ```

   Without the variable the test skips itself. It overwrites the images of
   the activities it runs.
5. **Look at every image and every GIF before committing**, every frame of
   the GIFs included. A contact sheet of the frames, made with PIL, is the
   quickest way to check. Then delete any image the pages no longer use, and
   check that every image a page names exists.
6. **Commit the generator, the pages and the images together.** The images
   are not ignored, thanks to the `!doc/activities/images/*/*.png` and `*.gif`
   lines of `.gitignore`.

## The page

- **Start with the history.** One or two paragraphs on what this was in its
  day, and why people did it. Then a paragraph on what this page does.
- **"What you need"** says what to download and from where, as an end user
  would get it:
  - The Internet Archive is preferred.
  - A file from izmac's repository is linked on GitHub, with *Download raw
    file* named as the button to click.
  - Say how to rename or copy a file when the steps need it, for example
    `ada.img` and `grace.img`.
  - When izmac downloads everything by itself, say so and say what it
    downloads.
- **Number the steps once for the whole page.** The numbering continues
  across the `##` sections; it does not restart in each one.
- **Each step starts with the action in bold**, such as "**Double-click the
  diskette.**". Then comes what happens, then the picture, then the
  explanation of what the picture shows and why it mattered then.
- **Write for someone who has never seen a Macintosh.** Name a button or
  menu item by the words on it, in italics, and say where it is, such as "the
  box at its top left" or "the right one of the sixth row". Explain the
  keyboard mapping when a key is needed: the Command key is Command on a Mac,
  the Windows key on a PC, or Alt on either.
- **Command lines go in `bash` blocks**, followed by a sentence on each
  option that matters.
- **Say when an option's file need not exist yet.** Readers worry about it.
  For example, a `-pram` file is created on the first write.
- **Every claim must be true of what the emulator shows.** Check versions,
  sizes, names and timings on the machine. Remove claims that cannot be
  checked: a "diskette noise" line and a claim about System 7's memory were
  taken out for that reason.
- **Give the real time of anything slow**, such as "about a minute on a
  Macintosh Plus", especially when a recording shows it faster.
- **When two Macintoshes take part**, name them by their owners, for example
  *Ada's Mac* and *Grace's Mac*. Say at the start of each section which
  machine its steps are on. Show the second machine's pictures on the right
  with `<p align="right"><img src="..." alt="..."></p>`. The first machine's
  pictures stay as normal Markdown images on the left. The label in the frame
  says which machine each picture is from.
- **End with "What next"**, linking to the next activity or to the part of
  the manual that goes deeper.
- **Start each page with a link back**, `[Back to the activities](README.md)`.

## Screenshots

- **Every picture is made by the generator**, from the images committed in
  `test_images/` or from what izmac downloads by itself. Never use anything
  local and uncommitted.
- **The frame is part of the picture.** `framed` draws a black, CRT-like
  border with rounded corners and leaves the corners outside it transparent.
  Rounding the screen itself looked like cut-off triangles.
- **Label the frame** (`screenshotOf` with a label, or `record(m, label)`)
  only when more than one machine takes part.
- **Take the picture once the screen is finished.**
  - A fixed wait is fine for something quick.
  - For anything that builds up over time, detect the state from the pixels
    and keep the last good frame. MacPaint's page while printing is an
    example: `showsThePage` keeps the last frame with the page on screen,
    drawn whole. A picture taken too early, with the page half drawn, was
    sent back in review.
- **Copy `m.GetImage()` before keeping it.** It returns the same buffer on
  every call, which the machine keeps drawing into.
- **Don't take pictures of random things.** The Puzzle shuffles itself
  differently every run, so a picture of a moved tile was dropped. The
  shuffled puzzle stays, but it changes in every regeneration.
- **The layout must be the same on every run.**
  - The System 7 Finder rearranges the icons of a disk window if it is left a
    while, so open the window as soon as the Finder starts.
  - Do things in the order that keeps icons where the text says they are.
- **Show the result of each important step**, not every step. A picture that
  looks the same as the one before it adds nothing.
- **Keep the main page's thumbnails as PNGs.** `disk-window.png` is still
  generated because `README.md` uses it, even though the page uses a GIF
  there.

## Animated GIFs

- **Use a GIF when the movement is the point**, such as:
  - the start-up sequence;
  - a window zooming out of its icon;
  - a menu dropping and its items highlighting;
  - an outline dragged to the Trash;
  - a rectangle stretching;
  - the spray can;
  - the blinking question-mark disk;
  - File Sharing starting;
  - the copy dialog counting down.
- **A before-and-after also works**, even when the change is instant. The
  paint bucket fills at once on a Plus, and still makes a satisfying GIF.
- **Use a PNG when what matters is the end result** and the process is long
  and slow. MacPaint's printing was tried as a sped-up GIF and rejected; a
  PNG of the page fully drawn replaced it.
- **The first and the last frame must both be worth looking at.**
  - Before recording, move the pointer, unrecorded, onto what is about to be
    used, so the first frame shows the setup and not the pointer travelling
    across the screen.
  - End on the result: the window open, the menu down with an item
    highlighted, the copied folder inside the opened folder, *Stop* on the
    button.
  - Do anything that would spoil the last frame, such as letting go of a
    menu, after `save`.
- **Skip boring beginnings.** Start-up begins at the question-mark disk
  (`waitForAQuestionMark`), not at the black screen and the memory test.
- **Real speed by default.** Speed up only waits that would be tedious, and
  say so in the text, with the real time. File Sharing starting is recorded at
  five times its speed by running 30 frames per capture of 10 hundredths of a
  second.
- **Still stretches are capped** at `longestStill`, three seconds, so waits for
  the diskette or the network don't stall the GIF. The last frame gets an
  extra hold, the `hold` of `save`, so the result can be seen before the loop
  starts again.
- **Use `recording.glide` to move the pointer during a recording.**
  `moveMouseTo` jumps, while `glide` moves in small steps and captures each
  one.
  - Use `press` for the button and `doubleClick` for a double click, and
    `run(frames, every)` to let the machine go on while capturing.
  - Capture every 2 frames or more: browsers slow down GIF delays shorter
    than two hundredths of a second.
- **Keep the files small.** The recorder uses four colours and stores only
  the part of the screen that changed. The GIFs so far are 4 to 33 KB.
- **The pointer may not show.** MacPaint hides it while spraying, so only the
  paint shows the movement. Say so if it matters.

## Driving the machine

- **Go down a menu first, then across.** Moving diagonally from the menu bar
  goes over the next menu's title and opens that menu. `chooseFromMenu`
  already does it in this order. With `glide`, stay well below the menu bar:
  the pointer can overshoot into the bar when it moves back up.
- **The Finder 4.1 of System 2.0 has no Command-W.** Close windows with
  their close box, for example Get Info's at (24,40).
- **Some windows ask to save when closed.** In System 7, closing a sharing
  window asks for confirmation, and Return answers it.
- **Wait for System 7 to settle** before using the Apple menu: run
  `systemSevenBootFrames` after the Finder starts.
- **Guest access to a System 7.1.2 File Sharing server didn't work.** The
  Chooser greys out Guest even after it is allowed in Users & Groups. Log in
  as the owner, a registered user, instead.
- **For two machines**, put them on one `localtalk.NewNetwork()`. Run the
  first on a goroutine of its own while the second is driven, and stop it
  before taking its pictures.
- **MacPaint 1.5:**
  - Tools are in two columns, at x = 29 and x = 54, with rows at y = 40, 62,
    84, 106, 128, 150, 172, 194, 216 and 238.
  - The paint bucket is at (29,84), the spray can at (54,84), the pencil at
    (54,106), and black, the first pattern, at (127,306).
  - Command-click with the pencil opens FatBits on the place clicked; FatBits
    from the Goodies menu opens it somewhere else.
  - Print Final has no dialog. It draws the page as it prints, which takes
    about forty seconds.
- **Find coordinates on a screenshot**, then check them in the next image.
  The frame adds 14 pixels on each side, so subtract 14 from coordinates
  measured on a framed image.

## Review

- **The user reviews the style of the first pages and comments on the pull
  request.** Address each comment, regenerate, look again, push, and answer
  on the comment.
- **Points made in review apply to all activities**, not just the one they
  were made on. When one comes up, add it to this file.
