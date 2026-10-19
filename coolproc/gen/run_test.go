package main

import (
	"fmt"
	"github.com/Castux/mipsim2/coolproc/asm"
	"sync"
	"testing"

	"github.com/Castux/mipsim2/devices"
	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/netlist"
	"github.com/Castux/mipsim2/runner"
)

var (
	cpuOnce sync.Once
	cpuDoc  *doc.Document
	cpuNL   *netlist.Netlist
	cpuErr  error
)

// machine is the processor running a program.
type machine struct {
	t   *testing.T
	r   *runner.Runner
	ram *devices.Memory
}

// boot assembles src and boots it.
func boot(t *testing.T, src string) *machine {
	t.Helper()
	words, err := asm.Assemble(src)
	if err != nil {
		t.Fatal(err)
	}
	return bootWords(t, words)
}

// bootWords loads a program into the ROM, resets and returns the
// machine, before the first instruction.
func bootWords(t *testing.T, words []uint16) *machine {
	t.Helper()
	cpuOnce.Do(func() {
		cpuDoc, _, _, cpuErr = CPUDocument("prog.bin")
		if cpuErr == nil {
			cpuNL = netlist.CompileDoc(cpuDoc, netlist.Options{})
		}
	})
	if cpuErr != nil {
		t.Fatal(cpuErr)
	}
	devs, err := runner.DevicesFromDoc(cpuDoc, func(name string) ([]byte, error) {
		if name != "prog.bin" {
			return nil, fmt.Errorf("no file %s", name)
		}
		return romImage(words), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	r, err := runner.New(cpuNL, runner.Options{Devices: devs})
	if err != nil {
		t.Fatal(err)
	}
	m := &machine{t: t, r: r}
	for _, d := range devs {
		if mem, ok := d.(*devices.Memory); ok && mem.Name() == "ram" {
			m.ram = mem
		}
	}
	m.set("reset", "high")
	m.tick(1)
	m.set("reset", "low")
	return m
}

func (m *machine) set(name, v string) {
	m.t.Helper()
	if err := m.r.Set(name, v); err != nil {
		m.t.Fatal(err)
	}
	if err := m.r.Settle(); err != nil {
		m.t.Fatal(err)
	}
}

func (m *machine) tick(n int) {
	m.t.Helper()
	if err := m.r.Tick(n); err != nil {
		m.t.Fatal(err)
	}
}

// run executes until halted, at most limit instructions, and returns the
// number of instructions run.
func (m *machine) run(limit int) int {
	m.t.Helper()
	for n := range limit {
		m.tick(12)
		if m.r.Format("halted") == "1" {
			return n + 1
		}
	}
	m.t.Fatalf("not halted after %d instructions (pc %s)", limit, m.r.Format("pc"))
	return 0
}

func (m *machine) reg(name string) uint64 {
	m.t.Helper()
	v, err := m.r.Number(name)
	if err != nil {
		m.t.Fatalf("%s: %v", name, err)
	}
	return v
}

func (m *machine) expect(want map[string]uint64) {
	m.t.Helper()
	for name, v := range want {
		if got := m.reg(name); got != v {
			m.t.Errorf("%s = %#x, want %#x", name, got, v)
		}
	}
}

func TestHalt(t *testing.T) {
	m := boot(t, "halt")
	if n := m.run(3); n != 1 {
		t.Errorf("ran %d instructions", n)
	}
	m.expect(map[string]uint64{"pc": 0})
}
