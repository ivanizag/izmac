package izmac

import (
	"fmt"
	"io"
	"os"

	"github.com/ivanizag/izmac/component"
	"github.com/ivanizag/izmac/imagewriter"
)

/*
The printer on the end of a serial port.

A Macintosh prints by opening the serial driver on one of the two ports and
writing bytes to it, so as far as the machine is concerned a printer is
nothing more than something that never says no. The chip is told there is
something on the port and hands over every byte it shifts out; what happens
to those bytes on this side is the choice below.

	raw           the bytes as they come, appended to a file
	imagewriter   an ImageWriter II, whose pages come out as images

The raw mode is the wire made visible, and it is what the ImageWriter decoder
was written against: printing a page and looking at what came out of the port
is a better description of the driver than any manual is. It stays because it
is also the answer when a program drives the port itself.

Nothing is opened until a byte arrives, so a machine that never prints leaves
nothing behind.
*/
type printer struct {
	// channel is the serial port the printer hangs from
	channel int

	// name is the mode, printerRaw or printerImageWriter, for reporting
	name string

	// target is the file, or the prefix of the files, the mode writes
	target string

	// open makes the sink the first byte needs, and is what the modes
	// differ in
	open func() (io.WriteCloser, error)

	// out is where the bytes go, nil until the first one arrives
	out io.WriteCloser

	/*
		err is the first thing that went wrong. A printer that can not
		write is a printer that is off, not a reason to stop the machine:
		the bytes are dropped from then on and the machine goes on
		believing it printed, which is what a real one with no paper in it
		does.
	*/
	err error
}

const (
	// The modes the printer option takes
	printerNone        = "none"
	printerRaw         = "raw"
	printerImageWriter = "imagewriter"

	// defaultPrinterRawFile is where the raw mode writes, and carries the
	// izmac_ prefix everything izmac writes for itself carries
	defaultPrinterRawFile = "izmac_printer.bin"

	// defaultPrinterPagePrefix is what the pages of the ImageWriter are
	// named after, the number of the page and the extension following it
	defaultPrinterPagePrefix = "izmac_page"
)

// newPrinter builds the printer a configuration asks for, or nil when it asks
// for none
func newPrinter(config *Configuration) (*printer, error) {
	channel := component.ChannelB
	if config.PrinterPort == printerPortModem {
		channel = component.ChannelA
	}

	switch config.Printer {
	case printerNone, "":
		return nil, nil

	case printerRaw:
		target := config.PrinterFile
		if target == "" {
			target = defaultPrinterRawFile
		}
		return &printer{
			channel: channel,
			name:    printerRaw,
			target:  target,
			open: func() (io.WriteCloser, error) {
				return os.OpenFile(target,
					os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			},
		}, nil

	case printerImageWriter:
		prefix := config.PrinterFile
		if prefix == "" {
			prefix = defaultPrinterPagePrefix
		}
		p := &printer{
			channel: channel,
			name:    printerImageWriter,
			target:  prefix + "_nnn.png",
		}
		p.open = func() (io.WriteCloser, error) {
			w := imagewriter.New(prefix)
			w.OnPage = func(name string) {
				fmt.Printf("The printer has finished a page: %v\n", name)
			}
			return w, nil
		}
		return p, nil
	}

	return nil, fmt.Errorf("unknown printer %q, use %v, %v or %v",
		config.Printer, printerNone, printerRaw, printerImageWriter)
}

/*
Transmit takes a byte off the serial port. It is called by the chip as the
byte finishes going out, from the emulation goroutine and nowhere else.
*/
func (p *printer) Transmit(value uint8) {
	if p.err != nil {
		return
	}

	if p.out == nil {
		out, err := p.open()
		if err != nil {
			p.fail(err)
			return
		}
		p.out = out
	}

	if _, err := p.out.Write([]uint8{value}); err != nil {
		p.fail(err)
	}
}

// fail records what went wrong and says so once, since the machine carries on
// either way and would otherwise say it on every byte
func (p *printer) fail(err error) {
	p.err = fmt.Errorf("the printer on the %v port: %w", p.portName(), err)
	fmt.Fprintln(os.Stderr, p.err)
}

// close finishes whatever was being printed, which for a page is what gets it
// written out
func (p *printer) close() error {
	if p.out == nil {
		return nil
	}

	err := p.out.Close()
	p.out = nil
	return err
}

// portName is the port the printer is on, as the machine's own software names
// it
func (p *printer) portName() string {
	if p.channel == component.ChannelA {
		return printerPortModem
	}
	return printerPortPrinter
}

// String describes the printer for the summary of the machine
func (p *printer) String() string {
	return fmt.Sprintf("Printer: %v on the %v port, writing %v",
		p.name, p.portName(), p.target)
}
