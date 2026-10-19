package main

import (
	"testing"

	"github.com/Castux/mipsim2/bitmap"
	"github.com/Castux/mipsim2/netlist"
	"github.com/Castux/mipsim2/runner"
	"github.com/Castux/mipsim2/sim"
)

// bench puts d at the root as "dut" and starts a runner. Warnings fail the
// test too: a readable circuit has none.
func bench(t *testing.T, l *Library, d *Def) *runner.Runner {
	t.Helper()
	dc := l.Document(benchRoot(d))
	if err := dc.Validate(); err != nil {
		t.Fatal(err)
	}
	nl := netlist.CompileDoc(dc, netlist.Options{})
	for _, dg := range nl.Diagnostics {
		t.Errorf("%v", dg)
	}
	if t.Failed() {
		t.FailNow()
	}
	r, err := runner.New(nl, runner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// benchRoot places d at the root as "dut", with a labelled wire stub on
// each port, named after the port.
func benchRoot(d *Def) *Def {
	root := newDef("top", 0, 0)
	in := &Inst{Name: "dut", Def: d}
	root.insts = append(root.insts, in)
	used := map[string]bool{}
	for _, name := range d.order {
		p := in.outside(name)
		root.cells[p] = bitmap.Wire
		// Name the stub after the net inside (a port's label), or the port.
		label := name
		if l, ok := d.labels[d.Port(name)]; ok {
			label = l
		} else if len(d.labels) > 0 {
			continue // an unlabelled second port of a labelled net
		}
		if !used[label] {
			used[label] = true
			root.labels[p] = label
		}
	}
	return root
}

func level(b bool) string {
	if b {
		return "high"
	}
	return "low"
}

// drive pins inputs (name -> value) and settles.
func drive(t *testing.T, r *runner.Runner, in map[string]bool) {
	t.Helper()
	for k, v := range in {
		if err := r.Set(k, level(v)); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Settle(); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, r *runner.Runner, name string) bool {
	t.Helper()
	v, err := r.Value(name)
	if err != nil {
		t.Fatal(err)
	}
	switch v {
	case sim.High:
		return true
	case sim.Low:
		return false
	}
	t.Fatalf("%s is %v", name, v)
	return false
}

func TestGates(t *testing.T) {
	l := newLibrary()
	for _, tc := range []struct {
		d *Def
		n int
		f func(in []bool) bool
	}{
		{l.nand(1), 1, func(in []bool) bool { return !in[0] }},
		{l.nand(2), 2, func(in []bool) bool { return !(in[0] && in[1]) }},
		{l.nand(3), 3, func(in []bool) bool { return !(in[0] && in[1] && in[2]) }},
		{l.nor(2), 2, func(in []bool) bool { return !(in[0] || in[1]) }},
		{l.nor(3), 3, func(in []bool) bool { return !(in[0] || in[1] || in[2]) }},
	} {
		r := bench(t, l, tc.d)
		for v := range 1 << tc.n {
			in := map[string]bool{}
			var bits []bool
			for k := range tc.n {
				bits = append(bits, v>>k&1 == 1)
				in[inputName(k, tc.n)] = bits[k]
			}
			drive(t, r, in)
			if got := read(t, r, "output"); got != tc.f(bits) {
				t.Errorf("%s%v = %v", tc.d.ID, bits, got)
			}
		}
	}
}
