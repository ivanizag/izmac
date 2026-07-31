package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"

	"github.com/ivanizag/izmac"
)

func main() {
	err := run(os.Args[0], os.Args[1:])
	if izmac.IsHelpRequested(err) {
		os.Exit(0)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(name string, args []string) error {
	config := izmac.NewConfiguration()

	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	config.AddFlags(fs)

	frames := fs.Uint64("frames", 60,
		"frames to run before reporting")
	pngFile := fs.String("png", "",
		"file to write the screen to as a PNG image")
	disasmFrom := fs.Uint64("disasm", 0,
		"address to disassemble before running, in hex")
	disasmCount := fs.Int("disasmcount", 16,
		"instructions to disassemble")
	watchFrom := fs.Uint64("watch", 0,
		"address of a RAM range to report the writes of, in hex")
	watchLen := fs.Uint64("watchlen", 4,
		"length of the range watched")

	err := fs.Parse(args)
	if err != nil {
		return err
	}

	// What is left over are disk images, as izapple2 takes them. Note that
	// the flag package stops at the first of them, so the options have to
	// come first.
	err = config.AddFiles(fs.Args())
	if err != nil {
		return err
	}

	err = config.Validate()
	if err != nil {
		return err
	}

	m, err := izmac.NewMac(config)
	if err != nil {
		return err
	}

	for _, line := range m.Summary() {
		fmt.Println(line)
	}

	if *disasmFrom != 0 {
		fmt.Print(m.Disasm(uint32(*disasmFrom), *disasmCount))
		return nil
	}

	if *watchFrom != 0 {
		m.WatchWrites(uint32(*watchFrom), uint32(*watchFrom+*watchLen-1))
	}

	m.RunFrames(*frames)

	/*
		This frontend never starts the run loop, so the kill command that
		would put a changed diskette away is never sent. It is asked for
		here instead, before anything is reported.
	*/
	if err := m.FlushDiskettes(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	fmt.Printf("Ran %v frames, %v cycles, stopped at $%06x\n",
		m.GetFrames(), m.GetCycles(), m.GetPC())

	// With the sadmac tracer on, the run stops as soon as the machine
	// settles on the loop the ROM ends in after a failure
	report, halted := m.GetSadMac()
	if halted {
		// A halt is not always a failure reported by the ROM: a poll of a
		// device that never answers looks the same from here. The screen
		// and the disassembly below tell them apart.
		fmt.Printf("The machine stopped making progress.\n")
		fmt.Printf("Read as a power on failure it would be: %v\n", report)
	}

	fmt.Printf("\n%v\n%v\n", m.DumpRegisters(), m.Disasm(m.GetPC(), 6))

	if *pngFile != "" {
		err = writePng(*pngFile, m)
		if err != nil {
			return err
		}
		fmt.Printf("Screen written to %v\n", *pngFile)
	}

	return nil
}

func writePng(filename string, m *izmac.Mac) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, m.GetImage())
}
