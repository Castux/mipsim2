package main

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/Castux/mipsim2/coolproc/asm"
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

type machine struct {
	t   *testing.T
	r   *runner.Runner
	ram *devices.Memory
}

// boot assembles src, loads it, resets, and returns the machine before the
// first instruction. The processor is built once for all tests (it takes a
// while); set QUICK to skip these tests.
func boot(t *testing.T, src string) *machine {
	t.Helper()
	if os.Getenv("QUICK") != "" {
		t.Skip("QUICK")
	}
	cpuOnce.Do(func() {
		cpuDoc, cpuErr = build("prog.bin")
		if cpuErr == nil {
			cpuNL = netlist.CompileDoc(cpuDoc, netlist.Options{})
			for _, dg := range cpuNL.Diagnostics {
				cpuErr = fmt.Errorf("%v", dg)
			}
		}
	})
	if cpuErr != nil {
		t.Fatal(cpuErr)
	}
	words, err := asm.Assemble(src)
	if err != nil {
		t.Fatal(err)
	}
	devs, err := runner.DevicesFromDoc(cpuDoc, func(name string) ([]byte, error) {
		b := make([]byte, 0, 2*len(words))
		for _, w := range words {
			b = append(b, byte(w), byte(w>>8))
		}
		return b, nil
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
		if mem, ok := d.(*devices.Memory); ok && mem.Name() == "data" {
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

func (m *machine) run(limit int) int {
	m.t.Helper()
	for n := range limit {
		m.tick(12)
		if m.r.Format("halted") == "1" {
			return n + 1
		}
	}
	m.t.Fatalf("not halted after %d instructions (pc %s)", limit, m.r.Format("program_counter.value"))
	return 0
}

// reg reads a register: a, b, c, d, x, y or program_counter.
func (m *machine) reg(name string) uint64 {
	m.t.Helper()
	full := "register_" + name + ".value"
	if name == "program_counter" {
		full = "program_counter.value"
	}
	v, err := m.r.Number(full)
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

func TestProgramHalt(t *testing.T) {
	m := boot(t, "halt")
	if n := m.run(3); n != 1 {
		t.Errorf("ran %d instructions", n)
	}
}

func TestProgramSetAndMove(t *testing.T) {
	m := boot(t, `
		setl a, 0x34
		seth a, 0x12
		setl b, 0xff
		seth c, 0xab
		setl c, 0x01
		seth c, 0xcd
		move a, d
		move d, x
		set y, 0xbeef
		halt`)
	m.run(20)
	m.expect(map[string]uint64{"a": 0x1234, "b": 0xff, "c": 0xcd01, "d": 0x1234, "x": 0x1234, "y": 0xbeef, "program_counter": 10})
}

func TestProgramALU(t *testing.T) {
	for _, tc := range []struct {
		ins        string
		a, b, want uint16
		dest       string
	}{
		{"add x", 0x1234, 0x0f0f, 0x2143, "x"},
		{"add y", 0xffff, 1, 0, "y"},
		{"sub x", 3, 10, 0xfff9, "x"},
		{"and x na", 0xff0f, 0x0ff0, 0x00f0, "x"},
		{"nand y", 0xff00, 0x0ff0, 0xf0ff, "y"},
		{"xor x", 0xff00, 0x0ff0, 0xf0f0, "x"},
		{"shl x", 0x8421, 4, 0x4210, "x"},
		{"shr y", 0x8421, 4, 0x0842, "y"},
	} {
		m := boot(t, fmt.Sprintf("set a, %d\n set b, %d\n %s\n halt", tc.a, tc.b, tc.ins))
		m.run(10)
		if got := m.reg(tc.dest); got != uint64(tc.want) {
			t.Errorf("%s %#x %#x = %#x, want %#x", tc.ins, tc.a, tc.b, got, tc.want)
		}
	}
}

func TestProgramJumpsAndMemory(t *testing.T) {
	m := boot(t, `
		set y, 0x1234
		set a, 0xcafe
		store a
		load c
		setl a, 1
		setl b, 1
		add x            ; carry 0
		set y, no_carry
		jumpcz
		setl d, 99
		halt
	no_carry:
		setl a, 0
		set y, zero
		jumpz a
		setl d, 98
		halt
	zero:
		setl d, 7
		halt`)
	m.run(40)
	m.expect(map[string]uint64{"c": 0xcafe, "d": 7})
	if w := m.ram.Words(); w[0x1234] != 0xcafe {
		t.Errorf("ram[0x1234] = %#x", w[0x1234])
	}
}

func TestProgramDemo(t *testing.T) {
	src, err := os.ReadFile("../demo.asm")
	if err != nil {
		t.Fatal(err)
	}
	m := boot(t, string(src))
	m.run(1000)
	w := m.ram.Words()
	a, b := uint64(0), uint64(1)
	for i := range 20 {
		if w[i] != b {
			t.Fatalf("ram[%d] = %d, want %d", i, w[i], b)
		}
		a, b = b, a+b
	}
}
