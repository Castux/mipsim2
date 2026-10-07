// Command mipsim-run runs circuits headless.
//
//	mipsim-run FILE [flags]
//
// FILE is a .mip document or a .fix test fixture. Flags may come before or
// after it:
//
//	--set a=5,b=high   pin nets or buses before running (repeatable)
//	--ticks N          run N clock periods, printing after each (default 0)
//	--watch a,sum      names to print (default: every name)
//	--clock NAME       the clock net (default "clock")
//	--trace            also print every change of a named net
//	--dump ram         print a memory device's contents (hex) after the run
//	--isolated         compile with the isolated-sources rules
//
// Output is one line per tick: "tick N: name=value ...". Nets print as 0, 1,
// z (floating) or x (unstable); buses (name_0, name_1, ...) as numbers, or ?
// if a bit is not a clean level.
//
// Exit status: 0 on success, 1 if the file, the circuit or the run fails
// (compile errors, device errors such as a memory reading a floating bit),
// 2 for bad usage.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Castux/mipsim2/devices"
	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/internal/fixture"
	"github.com/Castux/mipsim2/netlist"
	"github.com/Castux/mipsim2/runner"
	"github.com/Castux/mipsim2/sim"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(s string) error { *l = append(*l, splitList(s)...); return nil }

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mipsim-run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var sets, watch listFlag
	fs.Var(&sets, "set", "pin nets or buses before running: name=value,...")
	fs.Var(&watch, "watch", "names to print: name,...")
	var dumps listFlag
	fs.Var(&dumps, "dump", "print a memory device's contents after the run: name,...")
	ticks := fs.Int("ticks", 0, "clock periods to run")
	clock := fs.String("clock", runner.DefaultClock, "name of the clock net")
	trace := fs.Bool("trace", false, "print every change of a named net")
	isolated := fs.Bool("isolated", false, "compile with the isolated-sources rules")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: mipsim-run FILE [--set a=1,...] [--ticks N] [--watch a,...] [--clock NAME] [--trace] [--dump MEM,...] [--isolated]")
		fs.PrintDefaults()
	}

	// Allow flags after the file name, as in "mipsim-run cpu.mip --ticks 10".
	var files []string
	for {
		if err := fs.Parse(args); err != nil {
			return 2
		}
		if fs.NArg() == 0 {
			break
		}
		files = append(files, fs.Arg(0))
		args = fs.Args()[1:]
	}
	if len(files) != 1 {
		fs.Usage()
		return 2
	}
	if *ticks < 0 {
		fmt.Fprintln(stderr, "mipsim-run: --ticks must not be negative")
		return 2
	}

	d, err := load(files[0])
	if err != nil {
		fmt.Fprintln(stderr, "mipsim-run:", err)
		return 1
	}
	opts := netlist.Options{}
	if *isolated {
		opts.Variant = netlist.IsolatedSources
	}
	nl := netlist.CompileDoc(d, opts)
	for _, diag := range nl.Diagnostics {
		fmt.Fprintln(stderr, diag)
	}
	if nl.HasErrors() {
		fmt.Fprintln(stderr, "mipsim-run: the circuit has errors")
		return 1
	}

	dir := filepath.Dir(files[0])
	devs, err := runner.DevicesFromDoc(d, func(name string) ([]byte, error) {
		if !filepath.IsAbs(name) {
			name = filepath.Join(dir, name) // relative to the document, as in the editor
		}
		return os.ReadFile(name)
	})
	if err != nil {
		fmt.Fprintln(stderr, "mipsim-run:", err)
		return 1
	}
	r, err := runner.New(nl, runner.Options{Clock: *clock, Devices: devs})
	if err != nil {
		fmt.Fprintln(stderr, "mipsim-run:", err)
		return 1
	}
	if len(watch) == 0 {
		watch = r.Names()
	}
	for _, name := range dumps {
		if findMemory(r, name) == nil {
			fmt.Fprintf(stderr, "mipsim-run: no memory device named %q\n", name)
			return 1
		}
	}
	for _, name := range watch {
		if r.Format(name) == "-" {
			fmt.Fprintf(stderr, "mipsim-run: no net or bus named %q\n", name)
			return 1
		}
	}
	if *trace {
		r.Sim().OnChange = func(c sim.Change) {
			if names := nl.Nets[c.Net].Names; len(names) > 0 {
				fmt.Fprintf(stdout, "  step %d: %s %v -> %v\n", c.Step, names[0], c.Old, c.New)
			}
		}
	}

	for _, kv := range sets {
		name, value, ok := strings.Cut(kv, "=")
		if !ok {
			fmt.Fprintf(stderr, "mipsim-run: --set wants name=value, got %q\n", kv)
			return 2
		}
		if err := r.Set(name, value); err != nil {
			fmt.Fprintln(stderr, "mipsim-run:", err)
			return 1
		}
	}
	if err := r.Settle(); err != nil {
		fmt.Fprintln(stderr, "mipsim-run:", err)
		return 1
	}

	print := func() {
		var b strings.Builder
		fmt.Fprintf(&b, "tick %d:", r.Ticks())
		for _, name := range watch {
			fmt.Fprintf(&b, " %s=%s", name, r.Format(name))
		}
		fmt.Fprintln(stdout, b.String())
	}
	print()
	if *ticks > 0 && !r.HasClock() {
		fmt.Fprintf(stderr, "mipsim-run: --ticks needs a clock net named %q\n", *clock)
		return 1
	}
	for range *ticks {
		if err := r.Tick(1); err != nil {
			fmt.Fprintln(stderr, "mipsim-run:", err)
			return 1
		}
		print()
	}
	for _, name := range dumps {
		dump(stdout, findMemory(r, name))
	}
	return 0
}

func findMemory(r *runner.Runner, name string) *devices.Memory {
	for _, d := range r.Devices() {
		if m, ok := d.(*devices.Memory); ok && m.Name() == name {
			return m
		}
	}
	return nil
}

// dump prints a memory device's words in hex, 16 per line.
func dump(w io.Writer, m *devices.Memory) {
	digits := (m.Config().Width + 3) / 4
	words := m.Words()
	for i := 0; i < len(words); i += 16 {
		var b strings.Builder
		fmt.Fprintf(&b, "%s %04x:", m.Name(), i)
		for j := i; j < min(i+16, len(words)); j++ {
			fmt.Fprintf(&b, " %0*x", digits, words[j])
		}
		fmt.Fprintln(w, b.String())
	}
}

func load(path string) (*doc.Document, error) {
	if filepath.Ext(path) == ".fix" {
		fx, err := fixture.ParseFile(path)
		if err != nil {
			return nil, err
		}
		return fx.Doc, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	d, err := doc.Load(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return d, nil
}
