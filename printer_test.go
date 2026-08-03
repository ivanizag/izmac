package izmac

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ivanizag/izmac/storage"
)

/*
The printer, from the address the machine writes to. What these check is the
wiring rather than the printing: that a byte written to the data register of
a serial port comes out of the printer that was put on that port, and only
that one.

The addresses are the write side of the chip, $bffff9, with the data
registers of the two channels four and six bytes along.
*/
const (
	sccWriteDataB = 0xbf_fffd
	sccWriteDataA = 0xbf_ffff
)

// printerMac builds a machine with a printer on it, running no ROM: the tests
// below write to the chip themselves rather than getting a Macintosh to do it
func printerMac(t *testing.T, config *Configuration) *Mac {
	t.Helper()

	config.RomFile = "<test>"
	config.PramFile = filepath.Join(t.TempDir(), "pram.bin")
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}

	return ensureNewMac(t, config,
		storage.RomFromData(make([]uint8, storage.RomSize)), nil, nil)
}

// print writes bytes to a serial port and gives them the time they take to go
// out on the wire
func print(m *Mac, address uint32, text string) {
	for _, b := range []uint8(text) {
		m.mm.Poke(address, b)
		m.scc.Tick(cyclesPerSecond)
	}
}

func TestWhatIsPrintedReachesTheFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "printed.bin")

	config := NewConfiguration()
	config.Printer = printerRaw
	config.PrinterFile = file
	m := printerMac(t, config)

	print(m, sccWriteDataB, "Hello")

	printed, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(printed) != "Hello" {
		t.Errorf("the printer wrote %q, wanted %q", printed, "Hello")
	}
}

// A machine that never prints leaves nothing behind, which is why the file is
// not opened until the first byte
func TestAMachineThatDoesNotPrintWritesNothing(t *testing.T) {
	file := filepath.Join(t.TempDir(), "printed.bin")

	config := NewConfiguration()
	config.Printer = printerRaw
	config.PrinterFile = file
	printerMac(t, config)

	if _, err := os.Stat(file); err == nil {
		t.Errorf("%v was made without anything being printed", file)
	}
}

/*
The printer is on one port and not on both. The modem port carries the x axis
of the mouse on the same channel, so a printer that answered to both would be
a printer that printed the mouse.
*/
func TestOnlyThePortThePrinterIsOnPrints(t *testing.T) {
	file := filepath.Join(t.TempDir(), "printed.bin")

	config := NewConfiguration()
	config.Printer = printerRaw
	config.PrinterFile = file
	m := printerMac(t, config)

	print(m, sccWriteDataA, "modem")
	if _, err := os.Stat(file); err == nil {
		t.Error("what went out of the modem port reached the printer on the printer port")
	}

	print(m, sccWriteDataB, "printer")
	printed, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(printed) != "printer" {
		t.Errorf("the printer wrote %q, wanted %q", printed, "printer")
	}
}

// And it goes on the modem port when it is asked to
func TestThePrinterCanGoOnTheModemPort(t *testing.T) {
	file := filepath.Join(t.TempDir(), "printed.bin")

	config := NewConfiguration()
	config.Printer = printerRaw
	config.PrinterPort = printerPortModem
	config.PrinterFile = file
	m := printerMac(t, config)

	print(m, sccWriteDataA, "modem")

	printed, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(printed) != "modem" {
		t.Errorf("the printer wrote %q, wanted %q", printed, "modem")
	}
}

/*
The ImageWriter, from the same place: a page printed by writing the commands
to the chip a byte at a time, which is the whole path from the address the
machine writes to the file on the host.
*/
func TestAPagePrintedThroughTheChipIsWritten(t *testing.T) {
	prefix := filepath.Join(t.TempDir(), "page")

	config := NewConfiguration()
	config.Printer = printerImageWriter
	config.PrinterFile = prefix
	m := printerMac(t, config)

	// A run of graphics of one column, and the form feed that takes the
	// page out
	print(m, sccWriteDataB, "\x1bG0001\xff\x0c")

	if _, err := os.Stat(prefix + "_001.png"); err != nil {
		t.Fatalf("the page was not written: %v", err)
	}
}

// A machine with no printer on it has nothing on either port, and writing to
// one is a byte that goes nowhere rather than a crash
func TestAMachineCanHaveNoPrinter(t *testing.T) {
	config := NewConfiguration()
	config.Printer = printerNone
	m := printerMac(t, config)

	if m.printer != nil {
		t.Error("a printer was attached to a machine that asked for none")
	}

	print(m, sccWriteDataB, "nowhere")
}

func TestAPrinterThatIsNotOneIsRefused(t *testing.T) {
	config := NewConfiguration()
	config.Printer = "teletype"

	if err := config.Validate(); err == nil {
		t.Error("a printer izmac does not have was accepted")
	}
}

func TestAPortThatIsNotOneIsRefused(t *testing.T) {
	config := NewConfiguration()
	config.PrinterPort = "usb"

	if err := config.Validate(); err == nil {
		t.Error("a serial port the machine does not have was accepted")
	}
}

// The summary names the printer, so that a run says what it has on it
func TestTheSummaryNamesThePrinter(t *testing.T) {
	config := NewConfiguration()
	config.Printer = printerImageWriter
	m := printerMac(t, config)

	named := false
	for _, line := range m.Summary() {
		if strings.Contains(line, printerImageWriter) &&
			strings.Contains(line, printerPortPrinter) {
			named = true
		}
	}
	if !named {
		t.Errorf("the summary does not name the printer: %v", m.Summary())
	}
}
