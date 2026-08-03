package izmac

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/ivanizag/izmac/scsi"
	"github.com/ivanizag/izmac/storage"
)

// Configuration holds the settings of an emulated machine. It is passed to
// NewMac() and not modified afterwards.
type Configuration struct {
	// RomFile is the path to a 128Kb Macintosh Plus ROM image. It has no
	// default: the ROM is copyrighted and can not be distributed.
	RomFile string

	// DiskFiles are the disk images attached to the SCSI bus, in the order
	// they take the target ids
	DiskFiles []string

	// Diskettes are the images that turned out to be diskettes, in the
	// order they go in the drives: the internal one and then the external
	Diskettes []string

	/*
		ScsiDriverFile is a disk image to take a SCSI driver from, for the
		sake of the bare volumes among DiskFiles. Those carry no SCSI driver
		and the ROM boots by loading one off the disk, so it has to be found
		somewhere before they can be attached. Nothing is written to it and
		nothing is written to them: the SCSI driver and the maps around it
		are held in memory in front of the volume.
	*/
	ScsiDriverFile string

	// PramFile is where the parameter RAM is persisted between runs
	PramFile string

	// WallClock makes the real time clock answer with the time of the host
	// on every read, instead of starting from it and counting the emulated
	// seconds of its own
	WallClock bool

	// RamSizeKb is the size of the RAM, 1024 or 4096
	RamSizeKb int

	// Clipboard shares the clipboard of the machine with the one of the
	// host, in both directions
	Clipboard bool

	// Mouse is how the pointer of the machine is driven, mouseAbsolute or
	// mouseRelative
	Mouse string

	// Printer is what is on the end of a serial port: none, raw or
	// imagewriter
	Printer string

	// PrinterPort is the port it hangs from, printer or modem
	PrinterPort string

	// PrinterFile is where the printer writes: the file the raw mode
	// appends to, or the prefix of the pages the ImageWriter draws. Empty
	// takes the default of whichever mode is in use.
	PrinterFile string

	// Speed is the processor clock in Mhz, or one of the names below
	Speed string

	// Trace is a comma separated list of the tracers to enable
	Trace string

	// Profile activates the CPU profiler
	Profile bool

	// romIsDefault tells that no ROM was named, so the default one can be
	// downloaded if it is not on the working directory
	romIsDefault bool

	// scsiDriverIsDefault tells the same of the SCSI driver image
	scsiDriverIsDefault bool

	// disketteFile is where the diskette booted when nothing was named on
	// the command line is kept. There is no option for it: it is a field
	// rather than a constant so that the tests can put it out of the way.
	disketteFile string

	// cycleDurationNs is Speed as the nanoseconds a cycle lasts, or zero
	// for no throttling at all
	cycleDurationNs float64

	// absoluteMouse is Mouse as the machine takes it
	absoluteMouse bool
}

const (
	// defaultPramFile, and the two below, carry the izmac_ prefix that
	// everything izmac writes for itself carries. See defaultRomFile.
	defaultPramFile  = "izmac_pram.bin"
	defaultRamSizeKb = 1024

	// speedPlus runs at the clock of the real machine, speedFull as fast
	// as the host can go
	speedPlus = "plus"
	speedFull = "full"

	/*
		mouseAbsolute puts the pointer of the machine where the host has its
		own, which the hardware can not be told and mousePointer.go goes around it
		to do. mouseRelative pushes the pointer by the movement of the host's,
		which is what the mouse of the machine really reports and what leaves
		a frontend with a pointer to capture.
	*/
	mouseAbsolute = "absolute"
	mouseRelative = "relative"

	// The two serial ports a printer can be put on, named as the machine's
	// own software names them
	printerPortPrinter = "printer"
	printerPortModem   = "modem"

	// defaultScsiDriverFile is where a borrowed SCSI driver is kept, and is
	// not a ROM at all: it is the front of a disk image, the maps and the
	// SCSI driver in the layout they were found in. The name says what it
	// is for.
	defaultScsiDriverFile = "izmac_hddriver.rom"

	/*
		defaultScsiDriverURL is a blank disk formatted by Apple's HD SC Setup,
		out of a collection of them kept for the SCSI adapters people put in
		real machines. It is pinned to the commit rather than the branch, so
		that what is fetched is the file that was tested and not whatever
		the branch has moved on to.
	*/
	defaultScsiDriverURL = "https://raw.githubusercontent.com/MrGasS/" +
		"Blank-SCSI-hard-disk-images-for-Macintosh/" +
		"1e4b92eed88b3d0b8d535e6387f6813bd40b512b/" +
		"Blank%20Apple%20HD%20SC%20formatted%20images/" +
		"20mb%20%5Bpce-macplus%20-%20AppleHDSC%5D.zip"

	// defaultDisketteFile is where the diskette booted when nothing was
	// named is kept
	defaultDisketteFile = "izmac_macpaint.dsk"

	/*
		defaultDisketteURL is MacPaint 1.5, a 400Kb startup diskette with
		System 2.0 and Finder 2.2 on it, kept at the Internet Archive.

		The version matters. The MacPaint 1.0 diskette of the same
		collection carries the System .97 of January 1984, which is the
		software of the 128K Macintosh and does not run on this machine: it
		reads its boot blocks, puts up the smiling Macintosh and then
		crashes into a screen of noise. 1.5 is the earliest one that comes
		up on a Plus.
	*/
	defaultDisketteURL = "https://archive.org/download/mac_Paint_2/Paint_2.dsk"
)

// NewConfiguration returns the default configuration
func NewConfiguration() *Configuration {
	c := &Configuration{
		PramFile:      defaultPramFile,
		RamSizeKb:     defaultRamSizeKb,
		Speed:         speedPlus,
		Mouse:         mouseAbsolute,
		Clipboard:     true,
		Printer:       printerImageWriter,
		PrinterPort:   printerPortPrinter,
		disketteFile:  defaultDisketteFile,
		absoluteMouse: true,
	}
	c.cycleDurationNs = cycleDurationOf(CPUClockMhz)
	return c
}

// ParseFlags fills the configuration from the command line arguments. A
// private FlagSet is used instead of the package level one so that a
// Configuration can be built more than once in the same process, as the
// tests do. A frontend with options of its own builds the FlagSet itself and
// calls AddFlags() and Validate().
func (c *Configuration) ParseFlags(name string, args []string, output io.Writer) error {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(output)
	c.AddFlags(fs)

	err := fs.Parse(args)
	if err != nil {
		return err
	}

	// What is left over are disk images, as izapple2 takes them
	err = c.AddFiles(fs.Args())
	if err != nil {
		return err
	}

	return c.Validate()
}

/*
IsHelpRequested tells whether parsing the flags failed only because the help
was asked for. The usage has been printed by then, so a frontend has nothing
to add and nothing to complain about: it exits quietly and successfully.
*/
func IsHelpRequested(err error) bool {
	return errors.Is(err, flag.ErrHelp)
}

/*
AddFiles takes the files named on the command line without a flag and puts
each where it belongs, working out from the image itself whether it is a hard
disk or a diskette.
*/
func (c *Configuration) AddFiles(filenames []string) error {
	if len(filenames) == 0 {
		return nil
	}

	for _, filename := range filenames {
		kind, err := storage.Classify(filename)
		if err != nil {
			return err
		}

		// A bare volume goes on the bus with the rest of the hard disks.
		// What it lacks is made up when it is attached, so there is
		// nothing to sort it out from them here.
		if kind == storage.KindFloppy {
			c.Diskettes = append(c.Diskettes, filename)
		} else {
			c.DiskFiles = append(c.DiskFiles, filename)
		}
	}

	return nil
}

/*
needsScsiDriver tells whether any of the disks is a bare volume, which is the
only reason a SCSI driver has to be found. An image that can not be looked at
is left alone: opening it later says so, and says it better than this could.
*/
func (c *Configuration) needsScsiDriver() (bool, error) {
	for _, filename := range c.DiskFiles {
		kind, err := storage.Classify(filename)
		if err != nil {
			continue
		}
		if kind == storage.KindBareVolume {
			return true, nil
		}
	}
	return false, nil
}

/*
ensureScsiDriver finds a SCSI driver for the bare volumes on the bus, if there
are any. Those carry none of their own and the ROM boots by loading one off
the disk, so one has to be borrowed before they can be attached; what is
missing around it is made up when they are.

A SCSI driver named on the command line is used as it is and never
downloaded. The default one is fetched if it is not there already, the way
the ROM is. A machine with nothing but properly formatted disks on it needs
none of this and never goes looking.
*/
func (c *Configuration) ensureScsiDriver(out io.Writer) (*storage.ScsiDriver, error) {
	wanted, err := c.needsScsiDriver()
	if err != nil {
		return nil, err
	}
	if !wanted {
		return nil, nil
	}

	if _, err := os.Stat(c.ScsiDriverFile); err != nil {
		if !c.scsiDriverIsDefault {
			return nil, fmt.Errorf("can not open the SCSI driver image: %w", err)
		}

		fmt.Fprintf(out, "A disk with no SCSI driver on it is attached, and %v is not here.\n",
			c.ScsiDriverFile)
		fmt.Fprintf(out, "Downloading one from %v\n", defaultScsiDriverURL)

		scsiDriver, err := storage.DownloadScsiDriver(c.ScsiDriverFile, defaultScsiDriverURL)
		if err != nil {
			return nil, err
		}

		fmt.Fprintf(out, "Saved as %v: %v blocks of %v code\n",
			c.ScsiDriverFile, scsiDriver.Blocks(), scsiDriver.Processor)
		return scsiDriver, nil
	}

	return storage.ReadScsiDriver(c.ScsiDriverFile)
}

/*
ensureStartupDiskette gives the machine something to boot when the command
line named no image at all, which would otherwise leave it sitting on the
blinking diskette forever. What is fetched is MacPaint 1.5, a startup
diskette carrying a System, a Finder and the program the machine was sold on,
and it goes in the internal drive.

It is kept on the working directory the way the ROM and the driver are, so
the download happens once. Naming any image, a hard disk or a diskette, is
enough to say what to boot instead, and then nothing is fetched at all.
*/
func (c *Configuration) ensureStartupDiskette(out io.Writer) error {
	if len(c.DiskFiles) != 0 || len(c.Diskettes) != 0 {
		return nil
	}

	if _, err := os.Stat(c.disketteFile); err != nil {
		fmt.Fprintf(out, "No disk image was given, and %v is not here.\n",
			c.disketteFile)
		fmt.Fprintf(out, "  Downloading MacPaint from %v\n", defaultDisketteURL)

		diskette, err := storage.DownloadDiskette(c.disketteFile, defaultDisketteURL)
		if err != nil {
			return err
		}

		fmt.Fprintf(out, "  Saved as %v\n", diskette)
	}

	c.Diskettes = append(c.Diskettes, c.disketteFile)
	return nil
}

/*
stringList collects a flag given more than once, so that -hd can name
several images.
*/
type stringList []string

func (l *stringList) String() string {
	return strings.Join(*l, ", ")
}

func (l *stringList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

// AddFlags registers the machine options on a FlagSet
func (c *Configuration) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.RomFile, "rom", c.RomFile,
		"path to the Macintosh Plus ROM image, 128Kb. Downloaded to "+
			defaultRomFile+" if not given and not already there")
	fs.Var((*stringList)(&c.DiskFiles), "hd",
		"hard disk image to attach to the SCSI bus, repeat for more than one")
	fs.Var((*stringList)(&c.Diskettes), "floppy",
		"400K or 800K diskette image, plain or DiskCopy 4.2, to put in a "+
			"drive. Repeat for the external drive as well")
	fs.StringVar(&c.ScsiDriverFile, "scsidriver", c.ScsiDriverFile,
		"a disk image to borrow a SCSI driver from, needed only to attach a "+
			"bare volume, an image with no partition map on it. Nothing is "+
			"written to either")
	fs.StringVar(&c.PramFile, "pram", c.PramFile,
		"path to the file where the parameter RAM is persisted")
	fs.BoolVar(&c.WallClock, "wallclock", c.WallClock,
		"read the clock from the host every time instead of counting "+
			"emulated seconds from it. Never drifts, but the date can "+
			"no longer be set from the machine")
	fs.IntVar(&c.RamSizeKb, "ram", c.RamSizeKb,
		"RAM size in Kb, 1024 or 4096")
	fs.BoolVar(&c.Clipboard, "clipboard", c.Clipboard,
		"share the clipboard with the host, so that a copy on the machine "+
			"can be pasted on the host and the other way round. "+
			"Use -clipboard=false to keep them apart")
	fs.StringVar(&c.Mouse, "mouse", c.Mouse,
		"how the mouse is driven: '"+mouseAbsolute+"' puts the pointer of "+
			"the machine where yours is, '"+mouseRelative+"' pushes it by "+
			"the movement of yours, the way the hardware does, and the "+
			"window captures your pointer to do it")
	fs.StringVar(&c.Printer, "printer", c.Printer,
		"what to attach to the serial port: '"+printerNone+"', '"+
			printerRaw+"' to append every byte sent to a file, or '"+
			printerImageWriter+"' to draw the pages an ImageWriter II "+
			"would print")
	fs.StringVar(&c.PrinterPort, "printerport", c.PrinterPort,
		"the serial port the printer is on, '"+printerPortPrinter+
			"' or '"+printerPortModem+"'")
	fs.StringVar(&c.PrinterFile, "printerfile", c.PrinterFile,
		"where the printer writes: the file the raw mode appends to, or "+
			"the prefix of the page images. Each mode has its own default")
	fs.StringVar(&c.Speed, "speed", c.Speed,
		"cpu speed in Mhz, '"+speedPlus+"' for the real "+
			"7.8336Mhz of the machine, '"+speedFull+"' for as fast as "+
			"possible, or a decimal number")
	fs.StringVar(&c.Trace, "trace", c.Trace,
		"comma separated list of tracers to enable: cpu, toolbox, sadmac, scsi, floppy")
	fs.BoolVar(&c.Profile, "profile", c.Profile,
		"generate a CPU profile")
}

// Validate checks the configuration once the flags are parsed
func (c *Configuration) Validate() error {
	if c.RomFile == "" {
		// No ROM was named, take the default one and allow downloading it
		c.RomFile = defaultRomFile
		c.romIsDefault = true
	}
	if c.RamSizeKb != 1024 && c.RamSizeKb != 4096 {
		return fmt.Errorf("unsupported RAM size %vKb, use 1024 or 4096", c.RamSizeKb)
	}

	if len(c.DiskFiles) > scsi.TargetCount {
		return fmt.Errorf("the bus takes %v disks, %v were given",
			scsi.TargetCount, len(c.DiskFiles))
	}

	if c.ScsiDriverFile == "" {
		// No SCSI driver was named, take the default one and allow
		// downloading it, which only happens if a disk turns out to want it
		c.ScsiDriverFile = defaultScsiDriverFile
		c.scsiDriverIsDefault = true
	}

	if len(c.Diskettes) > DriveCount {
		return fmt.Errorf("the machine has %v diskette drives, %v images were given",
			DriveCount, len(c.Diskettes))
	}

	if err := c.parseMouse(); err != nil {
		return err
	}

	if err := c.validatePrinter(); err != nil {
		return err
	}

	return c.parseSpeed()
}

// parseMouse turns the mouse option into the one thing the machine takes from
// it, whether the pointer is placed or pushed
func (c *Configuration) parseMouse() error {
	switch c.Mouse {
	case mouseAbsolute, "":
		c.absoluteMouse = true
	case mouseRelative:
		c.absoluteMouse = false
	default:
		return fmt.Errorf("invalid mouse %q, use '%v' or '%v'",
			c.Mouse, mouseAbsolute, mouseRelative)
	}
	return nil
}

/*
validatePrinter checks the printer options, so that a mode or a port that was
mistyped is said so at once rather than on the first byte printed, which is
minutes into a session and after the mistake has been forgotten.
*/
func (c *Configuration) validatePrinter() error {
	switch c.Printer {
	case printerNone, "", printerRaw, printerImageWriter:
	default:
		return fmt.Errorf("unknown printer %q, use %v, %v or %v",
			c.Printer, printerNone, printerRaw, printerImageWriter)
	}

	switch c.PrinterPort {
	case printerPortPrinter, "", printerPortModem:
	default:
		return fmt.Errorf("unknown serial port %q, use %v or %v",
			c.PrinterPort, printerPortPrinter, printerPortModem)
	}

	return nil
}

/*
parseSpeed turns the speed option into the duration of a cycle. Running at a
speed other than the real one is useful to get through a boot quickly or to
watch something happen slowly, but the machine has no idea: the scan line
tick, and with it the vertical blanking and the sound, are counted in cycles
and stay where they are relative to the code being run.
*/
func (c *Configuration) parseSpeed() error {
	switch c.Speed {
	case speedPlus, "":
		c.cycleDurationNs = cycleDurationOf(CPUClockMhz)
	case speedFull:
		c.cycleDurationNs = 0
	default:
		mhz, err := strconv.ParseFloat(c.Speed, 64)
		if err != nil || mhz <= 0 {
			return fmt.Errorf("invalid speed %q, use '%v', '%v' or a positive number of Mhz",
				c.Speed, speedPlus, speedFull)
		}
		c.cycleDurationNs = cycleDurationOf(mhz)
	}
	return nil
}

// cycleDurationOf returns the nanoseconds a cycle lasts at a given clock
func cycleDurationOf(mhz float64) float64 {
	return 1000.0 / mhz
}

// hasTracer returns true when the given tracer is listed in the trace option
func (c *Configuration) hasTracer(name string) bool {
	for _, v := range strings.Split(c.Trace, ",") {
		if strings.TrimSpace(v) == name {
			return true
		}
	}
	return false
}
