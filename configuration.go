package izmac

import (
	"errors"
	"flag"
	"fmt"
	"io"
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

	// PramFile is where the parameter RAM is persisted between runs
	PramFile string

	// WallClock makes the real time clock answer with the time of the host
	// on every read, instead of starting from it and counting the emulated
	// seconds of its own
	WallClock bool

	// RamSizeKb is the size of the RAM, 1024 or 4096
	RamSizeKb int

	// Speed is the processor clock in Mhz, or one of the names below
	Speed string

	// Trace is a comma separated list of the tracers to enable
	Trace string

	// Profile activates the CPU profiler
	Profile bool

	// romIsDefault tells that no ROM was named, so the default one can be
	// downloaded if it is not on the working directory
	romIsDefault bool

	// cycleDurationNs is Speed as the nanoseconds a cycle lasts, or zero
	// for no throttling at all
	cycleDurationNs float64
}

const (
	defaultPramFile  = "pram.bin"
	defaultRamSizeKb = 1024

	// speedPlus runs at the clock of the real machine, speedFull as fast
	// as the host can go
	speedPlus = "plus"
	speedFull = "full"
)

// NewConfiguration returns the default configuration
func NewConfiguration() *Configuration {
	c := &Configuration{
		PramFile:  defaultPramFile,
		RamSizeKb: defaultRamSizeKb,
		Speed:     speedPlus,
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

		if kind == storage.KindFloppy {
			c.Diskettes = append(c.Diskettes, filename)
		} else {
			c.DiskFiles = append(c.DiskFiles, filename)
		}
	}

	return nil
}

/*
stringList collects a flag given more than once, so that -disk can name
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
	fs.Var((*stringList)(&c.DiskFiles), "disk",
		"disk image to attach to the SCSI bus, repeat for more than one")
	fs.Var((*stringList)(&c.Diskettes), "floppy",
		"400K or 800K diskette image, plain or DiskCopy 4.2, to put in a "+
			"drive. Repeat for the external drive as well")
	fs.StringVar(&c.PramFile, "pram", c.PramFile,
		"path to the file where the parameter RAM is persisted")
	fs.BoolVar(&c.WallClock, "wallclock", c.WallClock,
		"read the clock from the host every time instead of counting "+
			"emulated seconds from it. Never drifts, but the date can "+
			"no longer be set from the machine")
	fs.IntVar(&c.RamSizeKb, "ram", c.RamSizeKb,
		"RAM size in Kb, 1024 or 4096")
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

	if len(c.Diskettes) > DriveCount {
		return fmt.Errorf("the machine has %v diskette drives, %v images were given",
			DriveCount, len(c.Diskettes))
	}

	return c.parseSpeed()
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
