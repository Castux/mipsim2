package main

import (
	"testing"

	"github.com/Castux/mipsim2/runner"
	"github.com/Castux/mipsim2/sim"
)

// start lays a design out, checks the drawing, and starts a runner on it.
func start(t *testing.T, design *Design) *runner.Runner {
	t.Helper()
	layout, err := design.LayOut()
	if err != nil {
		t.Fatal(err)
	}
	nets, err := design.Verify(layout.Document(), layout)
	if err != nil {
		t.Fatal(err)
	}
	r, err := runner.New(nets, runner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func level(high bool) string {
	if high {
		return "high"
	}
	return "low"
}

// set pins a signal and settles.
func set(t *testing.T, r *runner.Runner, name string, high bool) {
	t.Helper()
	if err := r.Set(name, level(high)); err != nil {
		t.Fatal(err)
	}
	r.Sim().Settle()
}

func expect(t *testing.T, r *runner.Runner, name string, high bool) {
	t.Helper()
	got, err := r.Value(name)
	if err != nil {
		t.Fatal(err)
	}
	want := sim.Low
	if high {
		want = sim.High
	}
	if got != want {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func TestMatrixGates(t *testing.T) {
	d := NewDesign()
	d.Unit("inputs")
	d.Unit("logic")
	top := d.Block("top")
	bottom := d.Block("bottom")
	d.External("a", "inputs")
	d.External("b", "inputs")
	d.External("c", "inputs")
	d.Not(top, "logic", "not_a", "a")
	d.NOR(top, "logic", "nor_a_b", "a", "b")
	d.NOR(bottom, "logic", "nor_a_b_c", "a", "b", "c") // across blocks
	d.NOR(bottom, "logic", "or_a_b_c", "nor_a_b_c")
	d.NOR(bottom, "logic", "one")
	d.NOR(top, "logic", "and_a_b", "not_a", "not_b")
	d.Not(bottom, "logic", "not_b", "b")
	r := start(t, d)
	for v := range 8 {
		a, b, c := v&1 != 0, v&2 != 0, v&4 != 0
		set(t, r, "a", a)
		set(t, r, "b", b)
		set(t, r, "c", c)
		expect(t, r, "not_a", !a)
		expect(t, r, "nor_a_b", !(a || b))
		expect(t, r, "nor_a_b_c", !(a || b || c))
		expect(t, r, "or_a_b_c", a || b || c)
		expect(t, r, "and_a_b", a && b)
		expect(t, r, "one", true)
	}
}

func TestMatrixLatch(t *testing.T) {
	d := NewDesign()
	d.Unit("inputs")
	d.Unit("latch")
	block := d.Block("only")
	d.External("data", "inputs")
	d.External("write", "inputs")
	d.External("reset", "inputs")
	d.Not(block, "latch", "data_inverted", "data")
	d.Not(block, "latch", "write_inverted", "write")
	d.NOR(block, "latch", "store_one", "data_inverted", "write_inverted")
	d.NOR(block, "latch", "store_zero", "data", "write_inverted")
	d.NOR(block, "latch", "value", "store_zero", "value_inverted", "reset")
	d.NOR(block, "latch", "value_inverted", "store_one", "value")
	r := start(t, d)
	set(t, r, "reset", true)
	set(t, r, "reset", false)
	expect(t, r, "value", false)
	set(t, r, "data", true)
	expect(t, r, "value", false) // closed
	set(t, r, "write", true)
	expect(t, r, "value", true) // open: follows the data
	set(t, r, "data", false)
	expect(t, r, "value", false)
	set(t, r, "data", true)
	set(t, r, "write", false)
	set(t, r, "data", false)
	expect(t, r, "value", true) // holds
	expect(t, r, "value_inverted", false)
}
