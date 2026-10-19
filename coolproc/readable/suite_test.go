package main

import (
	"fmt"
	"testing"

	"github.com/Castux/mipsim2/coolproc/isatest"
	"github.com/Castux/mipsim2/sim"
)

// TestInstructionSet runs the shared suite (coolproc/isatest): every
// instruction in every form, random programs and real programs, each
// compared with the reference emulator.
func TestInstructionSet(t *testing.T) {
	isatest.Run(t, func(t *testing.T, prog []uint16) isatest.Machine {
		return isaMachine{bootWords(t, prog)}
	})
}

// isaMachine adapts the machine to the suite.
type isaMachine struct{ m *machine }

func (a isaMachine) Run(limit int) int {
	for n := range limit {
		if err := a.m.r.Tick(12); err != nil {
			a.m.t.Fatal(err)
		}
		if v, _ := a.m.r.Value("halted"); v == sim.High {
			return n + 1
		}
	}
	return -1
}

func (a isaMachine) Reg(name string) uint16 {
	full := "program_counter.value"
	if name != "pc" {
		full = fmt.Sprintf("register_%s.value", name)
	}
	v, err := a.m.r.Number(full)
	if err != nil {
		a.m.t.Fatalf("%s: %v", name, err)
	}
	return uint16(v)
}

func (a isaMachine) Carry() bool {
	v, _ := a.m.r.Value("carry")
	return v == sim.High
}

func (a isaMachine) RAM() []uint64 { return a.m.ram.Words() }
