package e2e_tests

import (
	"testing"

	"github.com/ivanizag/izmac"
	"github.com/ivanizag/izmac/operator"
)

/*
The hands of the tests: what someone at the machine does, done by the
operator package, and stopping the test when it can not be done. They keep the
names and the arguments the tests were written with, positions in the int16
the ROM keeps them in.
*/

// buildTestMac validates a configuration and builds its machine, which is
// closed when the test ends
func buildTestMac(t testing.TB, config *izmac.Configuration) *izmac.Mac {
	t.Helper()

	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	m, err := izmac.NewMac(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	return m
}

// must stops the test when a hand could not do what it was asked
func must(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// mustNot is must for the hands that are not given the test: a key that does
// not exist is a mistake in the test, not something the machine did
func mustNot(err error) {
	if err != nil {
		panic(err)
	}
}

// moveMouseTo pushes the pointer to a place on the screen
func moveMouseTo(t *testing.T, m *izmac.Mac, h int16, v int16) {
	t.Helper()
	must(t, operator.New(m).MoveTo(int(h), int(v)))
}

// clickMouse presses and releases the only button the machine has
func clickMouse(m *izmac.Mac) {
	operator.New(m).Click()
}

// doubleClickAt opens what is at a place on the screen
func doubleClickAt(t *testing.T, m *izmac.Mac, h int16, v int16) {
	t.Helper()
	must(t, operator.New(m).DoubleClickAt(int(h), int(v)))
}

// dragTo drags what is at a place on the screen to another, as a hand does
func dragTo(t *testing.T, m *izmac.Mac, fromH, fromV, toH, toV int16) {
	t.Helper()
	must(t, operator.New(m).Drag(int(fromH), int(fromV), int(toH), int(toV)))
}

// pressKey taps a key by the name the key code table knows it by
func pressKey(m *izmac.Mac, name string) {
	mustNot(operator.New(m).PressKey(name))
}

// pressCommand taps a key with the command key held, a menu accelerator
func pressCommand(m *izmac.Mac, name string) {
	mustNot(operator.New(m).Command(name))
}

// typeText types a text on the keyboard
func typeText(m *izmac.Mac, text string) {
	mustNot(operator.New(m).TypeString(text))
}

// typeKeys types keys by their names, each with its modifiers, as "Shift+1"
func typeKeys(m *izmac.Mac, keys ...string) {
	mustNot(operator.New(m).TypeKeys(keys...))
}

// waitUntil runs the machine a second at a time until something has
// happened, for as many seconds as given, and tells whether it did
func waitUntil(m *izmac.Mac, seconds int, done func() bool) bool {
	return operator.New(m).WaitUntil(seconds, done)
}

// waitForApplication runs the machine until an application is running, for
// as many seconds as given
func waitForApplication(t *testing.T, m *izmac.Mac, name string, seconds int) {
	t.Helper()
	must(t, operator.New(m).WaitForApplication(name, seconds))
}

// currentApplication is the name of the application running
func currentApplication(m *izmac.Mac) string {
	return m.CurrentApplication()
}

// mountedVolumes are the names of the volumes mounted, the startup one first
func mountedVolumes(m *izmac.Mac) []string {
	return m.MountedVolumes()
}

// startupVolume is the name of the volume the machine started from
func startupVolume(m *izmac.Mac) string {
	if volumes := m.MountedVolumes(); len(volumes) != 0 {
		return volumes[0]
	}
	return ""
}

// applicationVolume is the name of the volume the application running was
// started from
func applicationVolume(m *izmac.Mac) string {
	return m.ApplicationVolume()
}
