package main

import "testing"

func TestAdders(t *testing.T) {
	l := newLibrary()
	r := bench(t, l, l.fullAdder())
	for v := range 8 {
		a, b, c := v&1 == 1, v&2 == 2, v&4 == 4
		drive(t, r, map[string]bool{"first": a, "second": b, "carry_in": c})
		n := 0
		for _, x := range []bool{a, b, c} {
			if x {
				n++
			}
		}
		if s, co := read(t, r, "sum"), read(t, r, "carry_out"); s != (n%2 == 1) || co != (n >= 2) {
			t.Errorf("%v %v %v: sum %v carry %v", a, b, c, s, co)
		}
	}
}

func TestIncrementer(t *testing.T) {
	l := newLibrary()
	r := bench(t, l, l.incrementer())
	for _, v := range []uint16{0, 1, 0x00ff, 0x1234, 0x7fff, 0xffff} {
		setWord(t, r, "value", "", v, false)
		drive(t, r, nil)
		if got := word(t, r, "plus_one", ""); got != v+1 {
			t.Errorf("%#x + 1 = %#x", v, got)
		}
	}
}
