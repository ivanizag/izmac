package unwrap

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAFolderOfTheHostIsReadWithItsFolders(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []uint8) {
		t.Helper()
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	resource := someImage(600)
	write("Read Me", []uint8("Read me."))
	write("Game/Levels/One", []uint8{1})
	write("Game/._App", encodeAppleDouble(resource, "APPLGAME"))

	files, err := ReadHost(dir)
	if err != nil {
		t.Fatal(err)
	}

	byName := make(map[string]File)
	for _, f := range files {
		byName[strings.Join(append(f.Folders, f.Name), "/")] = f
	}

	if f, ok := byName["Read Me"]; !ok || string(f.Data) != "Read me." {
		t.Errorf("Read Me is not at the top of the folder")
	}
	if _, ok := byName["Game/Levels/One"]; !ok {
		t.Errorf("One is not in Game/Levels")
	}
	if f, ok := byName["Game/App"]; !ok || !bytes.Equal(f.Resource, resource) {
		t.Errorf("the application kept in a ._ file alone is not in Game")
	}
	if len(files) != 3 {
		t.Errorf("the folder gave %v files, wanted 3", len(files))
	}
}

func TestAFileOfTheHostTakesTheAppleDoubleBesideIt(t *testing.T) {
	dir := t.TempDir()
	resource := someImage(400)
	name := filepath.Join(dir, "Game")
	if err := os.WriteFile(name, []uint8("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "._Game"),
		encodeAppleDouble(resource, "APPLGAME"), 0o600); err != nil {
		t.Fatal(err)
	}

	if !HasHostMetadata(name) {
		t.Errorf("the ._ file beside Game was not noticed")
	}

	files, err := ReadHost(name)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || !bytes.Equal(files[0].Resource, resource) ||
		string(files[0].Creator[:]) != "GAME" || string(files[0].Data) != "data" {
		t.Errorf("Game did not come with its resource fork and creator")
	}
}
