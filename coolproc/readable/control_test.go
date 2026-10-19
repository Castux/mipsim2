package main

import (
	"fmt"
	"testing"
)

func TestLogicBlock(t *testing.T) {
	l := newLibrary()
	d := l.logicBlock(logic{
		id:      "sample_logic",
		inputs:  []string{"p", "q", "r"},
		outputs: []string{"both", "either_or_r", "not_p", "copy"},
		terms: map[string][][]string{
			"both":        {{"p", "q"}},
			"either_or_r": {{"p", "!q"}, {"r"}},
			"not_p":       {{"!p"}},
			"copy":        {{"q"}},
		},
	})
	r := bench(t, l, d)
	for v := range 8 {
		p, q, rr := v&1 == 1, v&2 == 2, v&4 == 4
		drive(t, r, map[string]bool{"p": p, "q": q, "r": rr})
		want := map[string]bool{"both": p && q, "either_or_r": p && !q || rr, "not_p": !p, "copy": q}
		for k, w := range want {
			if got := read(t, r, k); got != w {
				t.Errorf("%v %v %v: %s = %v", p, q, rr, k, got)
			}
		}
	}
}

func TestStepCounter(t *testing.T) {
	l := newLibrary()
	r := bench(t, l, l.stepCounter())
	state := func() string {
		s := ""
		for k := range 6 {
			if read(t, r, fmt.Sprintf("stage_%d", k)) {
				s += "1"
			} else {
				s += "0"
			}
		}
		return s
	}
	drive(t, r, map[string]bool{"clock": false, "not_reset": false})
	drive(t, r, map[string]bool{"not_reset": true})
	want := []string{"000000", "100000", "110000", "111000", "111100", "111110", "111111", "011111", "001111", "000111", "000011", "000001", "000000"}
	for k, w := range want {
		if got := state(); got != w {
			t.Fatalf("tick %d: %s, want %s", k, got, w)
		}
		drive(t, r, map[string]bool{"clock": true})
		drive(t, r, map[string]bool{"clock": false})
	}
}

func TestInstructionRegister(t *testing.T) {
	l := newLibrary()
	r := bench(t, l, l.instructionRegister())
	drive(t, r, map[string]bool{"not_reset": false, "write": false})
	drive(t, r, map[string]bool{"not_reset": true})
	setWord(t, r, "data", "", 0xbeef, false)
	drive(t, r, map[string]bool{"write": true})
	drive(t, r, map[string]bool{"write": false})
	setWord(t, r, "data", "", 0, false)
	drive(t, r, nil)
	if got := word(t, r, "instruction", ""); got != 0xbeef {
		t.Errorf("instruction %#x", got)
	}
}
