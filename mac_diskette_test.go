package izmac

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

/*
Nothing has been dropped on almost every frame there ever is, and a frontend
says so by handing over no file system at all rather than an empty one. Asking
it for its contents is what crashed the windowed frontend the first time it
was run.
*/
func TestNothingDroppedIsNotAFile(t *testing.T) {
	if name, ok := PathOfDroppedImage(nil); ok {
		t.Errorf("a nil file system gave the file %q", name)
	}

	if name, ok := PathOfDroppedImage(fstest.MapFS{}); ok {
		t.Errorf("an empty file system gave the file %q", name)
	}
}

/*
A file system whose files are not real ones is a browser, where a dropped file
only exists inside the page. izmac would have nowhere to write a changed
diskette back to, so it takes nothing rather than half of it.
*/
func TestADroppedFileThatIsNotOnTheHostIsIgnored(t *testing.T) {
	made := fstest.MapFS{
		"disk.dsk": &fstest.MapFile{Data: []uint8("not a real file")},
	}

	if name, ok := PathOfDroppedImage(made); ok {
		t.Errorf("a file system of made up files gave the file %q", name)
	}
}

// A real file gives its path on the host, which is what a diskette is opened
// and written back through
func TestADroppedFileGivesItsPath(t *testing.T) {
	dir := t.TempDir()

	wanted := filepath.Join(dir, "work.dsk")
	if err := os.WriteFile(wanted, []uint8("a diskette"), 0666); err != nil {
		t.Fatal(err)
	}

	name, ok := PathOfDroppedImage(os.DirFS(dir))
	if !ok {
		t.Fatal("a real file dropped on the window was not taken")
	}
	if name != wanted {
		t.Errorf("the path came back as %q, wanted %q", name, wanted)
	}
}

// A folder holds no diskette, and dropping one has to be passed over rather
// than handed on as a file that will not open
func TestADroppedFolderIsPassedOver(t *testing.T) {
	dir := t.TempDir()

	if err := os.Mkdir(filepath.Join(dir, "aFolder"), 0777); err != nil {
		t.Fatal(err)
	}

	if name, ok := PathOfDroppedImage(os.DirFS(dir)); ok {
		t.Errorf("a folder gave the file %q", name)
	}

	// And a file alongside it is still found
	wanted := filepath.Join(dir, "zzz.dsk")
	if err := os.WriteFile(wanted, []uint8("a diskette"), 0666); err != nil {
		t.Fatal(err)
	}

	name, ok := PathOfDroppedImage(os.DirFS(dir))
	if !ok || name != wanted {
		t.Errorf("the file beside the folder came back as %q, %v", name, ok)
	}
}

// ShortImageName is what a menu and its messages are drawn from, and a line
// of a menu is only so wide
func TestALongNameIsShortenedFromTheMiddle(t *testing.T) {
	for _, c := range []struct{ path, wanted string }{
		{"work.dsk", "work.dsk"},
		{"/home/ivan/disks/work.dsk", "work.dsk"},
		{`C:\disks\work.dsk`, "work.dsk"},
		{"/disks/a-very-long-diskette-name-indeed.dsk", "a-very-lon...-indeed.dsk"},
		{"/disks/discos con acentuación en el nombre.dsk", "discos con... nombre.dsk"},
	} {
		if got := ShortImageName(c.path); got != c.wanted {
			t.Errorf("%q came out as %q, wanted %q", c.path, got, c.wanted)
		}
	}

	// Whatever it does, it has to fit on the line it is drawn on
	for _, name := range []string{
		"/disks/" + strings.Repeat("x", 300) + ".dsk",
		"/disks/" + strings.Repeat("ñ", 300) + ".dsk",
	} {
		if got := ShortImageName(name); len([]rune(got)) > 24 {
			t.Errorf("a very long name came out %v characters long: %q",
				len([]rune(got)), got)
		}
	}
}
