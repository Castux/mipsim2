package main

import (
	"fmt"
	"testing"

	"github.com/Castux/mipsim2/bitmap"
	"github.com/Castux/mipsim2/netlist"
	"github.com/Castux/mipsim2/runner"
)

// benchPulled is bench with a pull-up on every port named like pull.
func benchPulled(t *testing.T, l *Library, d *Def, pull func(port string) bool) *runner.Runner {
	t.Helper()
	root := benchRoot(d)
	in := root.insts[0]
	for _, name := range d.order {
		if pull(name) {
			root.cells[in.outside(name)] = bitmap.Power
		}
	}
	dc := l.Document(root)
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

func setWord(t *testing.T, r *runner.Runner, prefix, suffix string, v uint16, float bool) {
	t.Helper()
	for i := range 16 {
		s := level(v>>i&1 == 1)
		if float {
			s = "float"
		}
		if err := r.Set(fmt.Sprintf("%s_%d%s", prefix, i, suffix), s); err != nil {
			t.Fatal(err)
		}
	}
}

func word(t *testing.T, r *runner.Runner, prefix, suffix string) uint16 {
	t.Helper()
	var v uint16
	for i := range 16 {
		if read(t, r, fmt.Sprintf("%s_%d%s", prefix, i, suffix)) {
			v |= 1 << i
		}
	}
	return v
}

func TestRegisterUnit(t *testing.T) {
	l := newLibrary()
	d := l.registerUnit("a_register", slice{name: "a_register_bit", driver: true, output: lane1})
	r := benchPulled(t, l, d, func(p string) bool { return len(p) > 6 && p[len(p)-6:] == "_right" })
	set := func(m map[string]bool) { t.Helper(); drive(t, r, m) }
	set(map[string]bool{"not_reset": false, "write_low": false, "write_high": false, "read": false})
	set(map[string]bool{"not_reset": true})
	if v := word(t, r, "value", ""); v != 0 {
		t.Fatalf("after reset %#x", v)
	}
	// Write 0x1234 from the bus, both halves, then 0xab into the high half.
	setWord(t, r, "bus", "", 0x1234, false)
	set(map[string]bool{"write_low": true, "write_high": true})
	set(map[string]bool{"write_low": false, "write_high": false})
	setWord(t, r, "bus", "", 0xab00, false)
	set(map[string]bool{"write_high": true})
	set(map[string]bool{"write_high": false})
	if v := word(t, r, "value", ""); v != 0xab34 {
		t.Fatalf("value %#x, want 0xab34", v)
	}
	// Read onto the bus.
	setWord(t, r, "bus", "", 0, true)
	set(map[string]bool{"read": true})
	if v := word(t, r, "bus", ""); v != 0xab34 {
		t.Fatalf("bus %#x, want 0xab34", v)
	}
	set(map[string]bool{"read": false})
	if v := word(t, r, "bus", ""); v != 0xffff {
		t.Fatalf("released bus %#x, want all ones", v)
	}
}
