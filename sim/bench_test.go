package sim

import (
	"fmt"
	"testing"
	"time"

	"github.com/Castux/mipsim2/internal/synth"
	"github.com/Castux/mipsim2/netlist"
)

func chainSim(tb testing.TB, rows, cols int) (*Sim, []netlist.NetID, []netlist.NetID) {
	nl := netlist.CompileDoc(synth.InverterChains(rows, cols), netlist.Options{})
	s, err := New(nl, Options{})
	if err != nil {
		tb.Fatal(err)
	}
	var ins, outs []netlist.NetID
	for r := range rows {
		in, _ := nl.Lookup(fmt.Sprintf("in_%d", r))
		out, _ := nl.Lookup(fmt.Sprintf("out_%d", r))
		ins, outs = append(ins, in), append(outs, out)
	}
	return s, ins, outs
}

// halfTick pins every chain input to v and settles, like one clock edge that
// ripples through the whole circuit.
func halfTick(s *Sim, ins []netlist.NetID, v Value) {
	for _, in := range ins {
		s.Pin(in, v)
	}
	s.Settle()
}

func TestInverterChainsPropagate(t *testing.T) {
	s, ins, outs := chainSim(t, 3, 5) // odd length: out = !in
	for _, v := range []Value{High, Low, High} {
		halfTick(s, ins, v)
		want := map[Value]Value{High: Low, Low: High}[v]
		for r, out := range outs {
			if got := s.Value(out); got != want {
				t.Errorf("in=%v: out_%d = %v, want %v", v, r, got, want)
			}
		}
	}
}

// TestTickBudget reports the time for a full period (two half ticks, each
// rippling through all 23k transistors) on the synthetic circuit. The plan's
// budget is 1,000 ticks per second for a processor; this circuit switches
// every transistor every half tick, which is a worst case.
func TestTickBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("large circuit")
	}
	s, ins, _ := chainSim(t, 152, 152)
	const ticks = 20
	start := time.Now()
	for range ticks {
		halfTick(s, ins, High)
		halfTick(s, ins, Low)
	}
	per := time.Since(start) / ticks
	t.Logf("%d transistors: %v per tick (%.0f ticks/s), %d steps total",
		len(s.Netlist().Transistors), per, float64(time.Second)/float64(per), s.Steps())
}

func BenchmarkTickInverterChains(b *testing.B) {
	s, ins, _ := chainSim(b, 152, 152)
	for b.Loop() {
		halfTick(s, ins, High)
		halfTick(s, ins, Low)
	}
}
