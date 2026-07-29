package izmac

import (
	"io"
	"testing"
)

func TestTheDefaultRomIsUsedWhenNoneIsNamed(t *testing.T) {
	c := NewConfiguration()
	err := c.ParseFlags("izmac", []string{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	if c.RomFile != defaultRomFile {
		t.Errorf("the ROM defaults to %v, wanted %v", c.RomFile, defaultRomFile)
	}
	if !c.romIsDefault {
		t.Error("the default ROM was not marked as downloadable")
	}
}

func TestANamedRomIsNeverDownloaded(t *testing.T) {
	c := NewConfiguration()
	err := c.ParseFlags("izmac", []string{"-rom", "mine.rom"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	if c.romIsDefault {
		t.Error("a ROM named on the command line was marked as downloadable")
	}
}

func TestTheDefaults(t *testing.T) {
	c := NewConfiguration()
	err := c.ParseFlags("izmac", []string{"-rom", "rom.bin"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	if c.RamSizeKb != 1024 {
		t.Errorf("the default RAM size is %vKb, wanted 1024Kb", c.RamSizeKb)
	}
	if c.PramFile != "pram.bin" {
		t.Errorf("the parameter RAM defaults to %v, wanted pram.bin on the working directory",
			c.PramFile)
	}
}

func TestTheRamSizeIsChecked(t *testing.T) {
	for _, c := range []struct {
		size     string
		accepted bool
	}{
		{"1024", true},
		{"4096", true},
		{"512", false},
		{"2048", false},
	} {
		config := NewConfiguration()
		err := config.ParseFlags("izmac",
			[]string{"-rom", "rom.bin", "-ram", c.size}, io.Discard)

		if c.accepted && err != nil {
			t.Errorf("the RAM size %vKb was rejected: %v", c.size, err)
		}
		if !c.accepted && err == nil {
			t.Errorf("the RAM size %vKb was accepted", c.size)
		}
	}
}

func TestTheTracersAreListed(t *testing.T) {
	c := NewConfiguration()
	err := c.ParseFlags("izmac",
		[]string{"-rom", "rom.bin", "-trace", "cpu, toolbox"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	if !c.hasTracer("cpu") || !c.hasTracer("toolbox") {
		t.Error("the tracers listed were not recognized")
	}
	if c.hasTracer("sadmac") {
		t.Error("a tracer not listed was recognized")
	}
}

// A Configuration must be buildable more than once in the same process, which
// is why the flags do not go on the package level FlagSet
func TestTheConfigurationCanBeBuiltTwice(t *testing.T) {
	for i := 0; i < 2; i++ {
		c := NewConfiguration()
		err := c.ParseFlags("izmac", []string{"-rom", "rom.bin"}, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestTheSpeedOption(t *testing.T) {
	native := 1000.0 / CPUClockMhz

	for _, c := range []struct {
		speed    string
		duration float64
		accepted bool
	}{
		{"", native, true},       // The default is the real machine
		{"plus", native, true},   // And so is naming it
		{"full", 0, true},        // Zero means no throttling at all
		{"1", 1000.0, true},      // A slow motion Macintosh
		{"15.6672", 63.83, true}, // Or twice the real speed
		{"0", 0, false},          // A stopped clock is not a speed
		{"-2", 0, false},         //
		{"quick", 0, false},      //
	} {
		config := NewConfiguration()
		args := []string{"-rom", "rom.bin"}
		if c.speed != "" {
			args = append(args, "-speed", c.speed)
		}

		err := config.ParseFlags("izmac", args, io.Discard)

		if !c.accepted {
			if err == nil {
				t.Errorf("the speed %q was accepted", c.speed)
			}
			continue
		}

		if err != nil {
			t.Errorf("the speed %q was rejected: %v", c.speed, err)
			continue
		}
		if diff := config.cycleDurationNs - c.duration; diff > 0.01 || diff < -0.01 {
			t.Errorf("the speed %q gives %.2fns a cycle, wanted %.2f",
				c.speed, config.cycleDurationNs, c.duration)
		}
	}
}

func TestFullSpeedReachesTheMachine(t *testing.T) {
	config := NewConfiguration()
	err := config.ParseFlags("izmac",
		[]string{"-rom", "rom.bin", "-speed", "full"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	m := newMac(config, &rom{data: make([]uint8, romSize)}, nil)
	if !m.IsFullSpeed() {
		t.Error("the machine is throttled with the full speed option")
	}

	// And the reported clock falls back to the real one, there is no
	// meaningful number to give until it has run
	if m.GetClockMhz() != CPUClockMhz {
		t.Errorf("the clock reads %v at full speed, wanted the real %v",
			m.GetClockMhz(), CPUClockMhz)
	}
}

func TestTheSpeedCanBeToggled(t *testing.T) {
	config := NewConfiguration()
	err := config.ParseFlags("izmac", []string{"-rom", "rom.bin"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	m := newMac(config, &rom{data: make([]uint8, romSize)}, nil)
	if m.IsFullSpeed() {
		t.Fatal("the machine starts unthrottled by default")
	}

	m.toggleSpeed()
	if !m.IsFullSpeed() {
		t.Error("the toggle did not go to full speed")
	}

	m.toggleSpeed()
	if m.IsFullSpeed() {
		t.Error("the toggle did not come back to the speed configured")
	}
	if m.GetClockMhz() != CPUClockMhz {
		t.Errorf("the speed came back as %v, wanted %v", m.GetClockMhz(), CPUClockMhz)
	}
}

// Toggling from a machine already configured for full speed has to give the
// real one, otherwise the toggle would do nothing
func TestTogglingFromFullSpeedGivesTheRealOne(t *testing.T) {
	config := NewConfiguration()
	err := config.ParseFlags("izmac",
		[]string{"-rom", "rom.bin", "-speed", "full"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	m := newMac(config, &rom{data: make([]uint8, romSize)}, nil)
	m.toggleSpeed()

	if m.IsFullSpeed() {
		t.Fatal("the toggle left the machine at full speed")
	}
	if m.GetClockMhz() != CPUClockMhz {
		t.Errorf("the toggle gave %v Mhz, wanted the real %v", m.GetClockMhz(), CPUClockMhz)
	}
}
