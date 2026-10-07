package netlist

import (
	"fmt"
	"testing"
	"time"

	"github.com/Castux/mipsim2/internal/synth"
)

// raceEnabled is set in race builds, where timings are meaningless.
var raceEnabled bool

func TestInverterChainsCompileClean(t *testing.T) {
	d := synth.InverterChains(3, 4)
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	nl := CompileDoc(d, Options{})
	if len(nl.Diagnostics) > 0 {
		t.Fatalf("diagnostics: %v", nl.Diagnostics)
	}
	// Per chain: one net per inverter output (joined with the next input),
	// plus the first input, plus one low net per inverter.
	if got, want := len(nl.Nets), 3*(4+1+4); got != want {
		t.Errorf("%d nets, want %d", got, want)
	}
	if got := len(nl.Transistors); got != 12 {
		t.Errorf("%d transistors, want 12", got)
	}
	for r := range 3 {
		for _, name := range []string{fmt.Sprintf("in_%d", r), fmt.Sprintf("out_%d", r)} {
			if _, ok := nl.Lookup(name); !ok {
				t.Errorf("no net %s", name)
			}
		}
	}
}

// TestCompileBudget reports flatten and compile times for about a million
// pixels. The budget (PLAN.md) is 50 ms on a laptop; this test only fails if
// it is wildly exceeded, so slow CI machines do not flake.
func TestCompileBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("large circuit")
	}
	d := synth.InverterChains(152, 152)
	start := time.Now()
	flat := d.Flatten()
	flatten := time.Since(start)
	start = time.Now()
	nl := Compile(flat, Options{})
	compile := time.Since(start)
	t.Logf("%d pixels, %d nets, %d transistors: flatten %v, compile %v",
		flat.Pixels.Count(), len(nl.Nets), len(nl.Transistors), flatten, compile)
	if len(nl.Diagnostics) > 0 {
		t.Fatalf("unexpected diagnostics, first: %v", nl.Diagnostics[0])
	}
	if compile > 2*time.Second && !raceEnabled {
		t.Errorf("compile took %v, far over the 50 ms budget", compile)
	}
}

func BenchmarkFlatten1M(b *testing.B) {
	d := synth.InverterChains(152, 152)
	for b.Loop() {
		d.Flatten()
	}
}

func BenchmarkCompile1M(b *testing.B) {
	flat := synth.InverterChains(152, 152).Flatten()
	for b.Loop() {
		Compile(flat, Options{})
	}
}
