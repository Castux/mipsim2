package main

import (
	"image"
	"testing"

	"github.com/Castux/mipsim2/runner"
	"github.com/Castux/mipsim2/sim"
)

// build lays out d and starts a runner on it.
func build(t *testing.T, d *Design) *runner.Runner {
	t.Helper()
	l, err := d.Lay()
	if err != nil {
		t.Fatal(err)
	}
	dc := l.Document(image.Point{})
	nl, err := d.Verify(dc, l, image.Point{})
	if err != nil {
		t.Fatal(err)
	}
	r, err := runner.New(nl, runner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func level(b bool) string {
	if b {
		return "high"
	}
	return "low"
}

func expect(t *testing.T, r *runner.Runner, name string, want bool) {
	t.Helper()
	v, err := r.Value(name)
	if err != nil {
		t.Fatal(err)
	}
	w := sim.Low
	if want {
		w = sim.High
	}
	if v != w {
		t.Errorf("%s = %v, want %v", name, v, w)
	}
}

func TestMatrixGates(t *testing.T) {
	d := NewDesign()
	d.Unit("in")
	d.Unit("logic")
	b0 := d.Block("top")
	b1 := d.Block("bottom")
	d.Ext("a", "in")
	d.Ext("b", "in")
	d.Ext("c", "in")
	d.Not(b0, "logic", "na", "a")
	d.NOR(b0, "logic", "nor2", "a", "b")
	d.NOR(b1, "logic", "nor3", "a", "b", "c")
	d.NOR(b1, "logic", "or3", "nor3")
	d.NOR(b1, "logic", "one")
	d.NOR(b0, "logic", "and2", "na", "nb")
	d.Not(b1, "logic", "nb", "b")
	r := build(t, d)
	for v := range 8 {
		a, b, c := v&1 != 0, v&2 != 0, v&4 != 0
		for _, p := range [][2]string{{"a", level(a)}, {"b", level(b)}, {"c", level(c)}} {
			if err := r.Set(p[0], p[1]); err != nil {
				t.Fatal(err)
			}
		}
		r.Sim().Settle()
		expect(t, r, "na", !a)
		expect(t, r, "nor2", !(a || b))
		expect(t, r, "nor3", !(a || b || c))
		expect(t, r, "or3", a || b || c)
		expect(t, r, "and2", a && b)
		expect(t, r, "one", true)
	}
}

func TestMatrixLatch(t *testing.T) {
	d := NewDesign()
	d.Unit("in")
	d.Unit("latch")
	b := d.Block("only")
	d.Ext("d", "in")
	d.Ext("w", "in")
	d.Ext("rst", "in")
	d.Not(b, "latch", "dn", "d")
	d.Not(b, "latch", "wn", "w")
	d.NOR(b, "latch", "s", "dn", "wn")
	d.NOR(b, "latch", "r", "d", "wn")
	d.NOR(b, "latch", "q", "r", "qn", "rst")
	d.NOR(b, "latch", "qn", "s", "q")
	run := build(t, d)
	set := func(name string, v bool) {
		if err := run.Set(name, level(v)); err != nil {
			t.Fatal(err)
		}
		run.Sim().Settle()
	}
	set("rst", true)
	set("rst", false)
	expect(t, run, "q", false)
	set("d", true)
	expect(t, run, "q", false) // closed
	set("w", true)
	expect(t, run, "q", true) // open: follows d
	set("d", false)
	expect(t, run, "q", false)
	set("d", true)
	set("w", false)
	set("d", false)
	expect(t, run, "q", true) // holds
	expect(t, run, "qn", false)
}
