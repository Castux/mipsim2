package main

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/Castux/mipsim2/coolproc/isa"
	"github.com/Castux/mipsim2/coolproc/isatest"
	"github.com/Castux/mipsim2/devices"
	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/netlist"
	"github.com/Castux/mipsim2/runner"
	"github.com/Castux/mipsim2/sim"
)

// The processor is built once for all tests and benchmarks (it takes a
// build in a second). Set QUICK to skip the tests that run it.
var (
	processorOnce     sync.Once
	processorDocument *doc.Document
	processorNetlist  *netlist.Netlist
	processorError    error
)

func builtProcessor(tb testing.TB) (*doc.Document, *netlist.Netlist) {
	tb.Helper()
	if os.Getenv("QUICK") != "" {
		tb.Skip("QUICK")
	}
	processorOnce.Do(func() {
		processorDocument, processorError = processorDocumentOnly("prog.bin")
		if processorError == nil {
			processorNetlist = netlist.CompileDoc(processorDocument, netlist.Options{})
			for _, d := range processorNetlist.Diagnostics {
				processorError = fmt.Errorf("%v", d)
			}
		}
	})
	if processorError != nil {
		tb.Fatal(processorError)
	}
	return processorDocument, processorNetlist
}

// machine is the processor running a program.
type machine struct {
	tb     testing.TB
	runner *runner.Runner
	data   *devices.Memory
}

// boot loads a program into the program memory and resets the processor:
// reset high for a tick, then low.
func boot(tb testing.TB, program []uint16) *machine {
	tb.Helper()
	document, nets := builtProcessor(tb)
	image := make([]byte, 0, 2*len(program))
	for _, word := range program {
		image = append(image, byte(word), byte(word>>8)) // little-endian, as the memory reads it
	}
	memories, err := runner.DevicesFromDoc(document, func(string) ([]byte, error) { return image, nil })
	if err != nil {
		tb.Fatal(err)
	}
	r, err := runner.New(nets, runner.Options{Devices: memories})
	if err != nil {
		tb.Fatal(err)
	}
	m := &machine{tb: tb, runner: r}
	for _, d := range memories {
		if memory, ok := d.(*devices.Memory); ok && memory.Name() == "data" {
			m.data = memory
		}
	}
	m.set("reset", "high")
	m.tick(1)
	m.set("reset", "low")
	return m
}

func (m *machine) set(name, value string) {
	m.tb.Helper()
	if err := m.runner.Set(name, value); err != nil {
		m.tb.Fatal(err)
	}
	if err := m.runner.Settle(); err != nil {
		m.tb.Fatal(err)
	}
}

func (m *machine) tick(n int) {
	m.tb.Helper()
	if err := m.runner.Tick(n); err != nil {
		m.tb.Fatal(err)
	}
}

// ticksPerInstruction: four steps of three states (proc.md, Implementation).
const ticksPerInstruction = 12

// The suite's view of the machine (isatest.Machine).

func (m *machine) Run(limit int) int {
	for n := range limit {
		m.tick(ticksPerInstruction)
		if v, _ := m.runner.Value("halted"); v == sim.High {
			return n + 1
		}
	}
	return -1
}

func (m *machine) number(name string) uint16 {
	m.tb.Helper()
	v, err := m.runner.Number(name)
	if err != nil {
		m.tb.Fatalf("%s: %v", name, err)
	}
	return uint16(v)
}

func (m *machine) Register(r isa.Register) uint16 {
	return m.number("register_" + r.String())
}
func (m *machine) ProgramCounter() uint16 { return m.number("program_counter") }
func (m *machine) Memory() []uint64       { return m.data.Words() }

func (m *machine) Carry() bool {
	v, _ := m.runner.Value("carry")
	return v == sim.High
}

func bootForSuite(tb testing.TB, program []uint16) isatest.Machine { return boot(tb, program) }

// TestInstructionSet runs the shared suite (coolproc/isatest): every
// instruction in every form, random programs and real programs, each
// compared with the reference emulator.
func TestInstructionSet(t *testing.T) { isatest.Run(t, bootForSuite) }

// BenchmarkProcessor measures the processor on the suite's benchmark
// program.
func BenchmarkProcessor(b *testing.B) { isatest.Benchmark(b, bootForSuite) }

// TestDemo runs the demo the document's program memory loads: the
// Fibonacci numbers.
func TestDemo(t *testing.T) {
	source, err := os.ReadFile("../demo.asm")
	if err != nil {
		t.Fatal(err)
	}
	m := boot(t, isatest.Assemble(t, string(source)))
	if m.Run(1000) < 0 {
		t.Fatal("the demo did not halt")
	}
	a, b := uint64(0), uint64(1)
	for k := range 20 {
		if got := m.Memory()[k]; got != b {
			t.Fatalf("memory[%d] = %d, want %d", k, got, b)
		}
		a, b = b, a+b
	}
}
