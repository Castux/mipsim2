package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Castux/mipsim2/coolproc/isa"
	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/netlist"
)

func main() {
	out := flag.String("o", "coolproc/cpu_readable.mip", "document to write")
	prog := flag.String("prog", "coolproc/demo.asm", "program for the program memory")
	flag.Parse()
	if err := run(*out, *prog); err != nil {
		fmt.Fprintln(os.Stderr, "readable:", err)
		os.Exit(1)
	}
}

// build returns the processor as a document, with its memories (the
// program memory loads prog, relative to the document).
func build(prog string) (*doc.Document, error) {
	l := newLibrary()
	top := l.processor()
	d := l.Document(top)
	devs, err := memories(prog)
	if err != nil {
		return nil, err
	}
	d.Devices = devs
	d.Note = "coolproc, built from components (coolproc/readable, see proc.md). Pin reset high for a tick, release it, then run the clock: 12 ticks per instruction."
	return d, d.Validate()
}

func run(out, prog string) error {
	src, err := os.ReadFile(prog)
	if err != nil {
		return err
	}
	words, err := isa.Assemble(string(src))
	if err != nil {
		return fmt.Errorf("%s: %w", prog, err)
	}
	d, err := build("prog.bin")
	if err != nil {
		return err
	}
	nl := netlist.CompileDoc(d, netlist.Options{})
	for _, dg := range nl.Diagnostics {
		fmt.Fprintln(os.Stderr, dg)
	}
	data, err := d.Save()
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		return err
	}
	img := make([]byte, 0, 2*len(words))
	for _, w := range words {
		img = append(img, byte(w), byte(w>>8))
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(out), "prog.bin"), img, 0o644); err != nil {
		return err
	}
	fmt.Printf("%s: %d components, %d nets, %d transistors\n", out, len(d.Defs)-1, len(nl.Nets), len(nl.Transistors))
	return nil
}
