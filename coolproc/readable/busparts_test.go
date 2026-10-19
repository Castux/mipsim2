package main

import (
	"fmt"
	"testing"
)

func TestBusEnd(t *testing.T) {
	l := newLibrary()
	r := bench(t, l, l.busEnd())
	for _, v := range []uint16{0, 1, 0x8000, 0x0100, 0xffff} {
		for i := range 16 {
			s := "float" // the pull-up makes it 1
			if v>>i&1 == 0 {
				s = "low"
			}
			if err := r.Set(fmt.Sprintf("bus_%d", i), s); err != nil {
				t.Fatal(err)
			}
		}
		drive(t, r, nil)
		if got := read(t, r, "bus_is_zero"); got != (v == 0) {
			t.Errorf("%#x: zero %v", v, got)
		}
		if got := read(t, r, "bus_15"); got != (v&0x8000 != 0) {
			t.Errorf("%#x: bit 15 %v", v, got)
		}
	}
}

func TestImmediate(t *testing.T) {
	l := newLibrary()
	r := benchPulled(t, l, l.immediate(), func(p string) bool { return len(p) > 6 && p[len(p)-6:] == "_right" })
	arg := func(v uint8) {
		for k := range 8 {
			drive(t, r, map[string]bool{fmt.Sprintf("argument_inverted_%d", k): v>>k&1 == 0})
		}
	}
	arg(0xa5)
	drive(t, r, map[string]bool{"immediate_low_to_bus": true, "immediate_high_to_bus": false})
	if got := word(t, r, "bus", ""); got != 0x00a5 {
		t.Errorf("setl: bus %#x", got)
	}
	drive(t, r, map[string]bool{"immediate_low_to_bus": false, "immediate_high_to_bus": true})
	if got := word(t, r, "bus", ""); got != 0xa5ff {
		t.Errorf("seth: bus %#x (low byte not driven: pulled up)", got)
	}
}
