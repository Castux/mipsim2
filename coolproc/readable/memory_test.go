package main

import "testing"

func TestLatch(t *testing.T) {
	l := newLibrary()
	r := bench(t, l, l.latch())
	step := func(data, write, notReset, want bool) {
		t.Helper()
		drive(t, r, map[string]bool{"data": data, "write": write, "not_reset": notReset})
		if got := read(t, r, "value"); got != want {
			t.Errorf("data %v write %v not_reset %v: value %v", data, write, notReset, got)
		}
		if got := read(t, r, "value_inverted"); got == want {
			t.Errorf("value_inverted equals value")
		}
	}
	step(false, false, false, false) // reset
	step(true, false, true, false)   // closed: holds 0
	step(true, true, true, true)     // open: follows
	step(false, true, true, false)
	step(true, true, true, true)
	step(true, false, true, true)    // closes on 1
	step(false, false, true, true)   // holds
	step(false, false, false, false) // reset while closed
}
