package main

import (
	"testing"
)

func TestALU(t *testing.T) {
	l := newLibrary()
	r := bench(t, l, l.alu())
	ops := []string{"and", "or", "xor", "add", "shift"}
	for _, tc := range []struct {
		op         string
		invA, invB bool
		carry      bool
		a, b       uint16
		want       uint16
		carryOut   bool
	}{
		{"add", false, false, false, 0x1234, 0x0f0f, 0x2143, false},
		{"add", false, false, false, 0xffff, 1, 0, true},
		{"add", false, true, true, 10, 3, 7, true}, // a - b
		{"add", false, true, true, 3, 10, 0xfff9, false},
		{"and", false, false, false, 0xff0f, 0x0ff0, 0x0f00, false},
		{"and", true, false, false, 0xff0f, 0x0ff0, 0x00f0, false},
		{"or", false, false, false, 0xf000, 0x000f, 0xf00f, false},
		{"or", true, true, false, 0xff00, 0x0ff0, 0xf0ff, false}, // nand
		{"xor", false, false, false, 0xff00, 0x0ff0, 0xf0f0, false},
		{"xor", true, false, false, 0xff00, 0x0ff0, 0x0f0f, false},
	} {
		setWord(t, r, "operand_a", "", tc.a, false)
		setWord(t, r, "operand_b", "", tc.b, false)
		setWord(t, r, "shifted", "", 0x5555, false)
		in := map[string]bool{"invert_a": tc.invA, "invert_b": tc.invB, "carry_in_0": tc.carry}
		for _, o := range ops {
			in["select_"+o] = o == tc.op
		}
		drive(t, r, in)
		if got := word(t, r, "result", ""); got != tc.want {
			t.Errorf("%s %#x %#x = %#x, want %#x", tc.op, tc.a, tc.b, got, tc.want)
		}
		if tc.op == "add" {
			if got := read(t, r, "carry_out_15"); got != tc.carryOut {
				t.Errorf("%s %#x %#x: carry %v", tc.op, tc.a, tc.b, got)
			}
		}
	}
	// Shift passes the shifted lane.
	in := map[string]bool{}
	for _, o := range ops {
		in["select_"+o] = o == "shift"
	}
	drive(t, r, in)
	if got := word(t, r, "result", ""); got != 0x5555 {
		t.Errorf("shift: %#x", got)
	}
}

func TestShifter(t *testing.T) {
	l := newLibrary()
	r := bench(t, l, l.shifter())
	for _, tc := range []struct {
		a     uint16
		n     uint16
		right bool
		want  uint16
		carry bool
	}{
		{0x8421, 4, false, 0x4210, false},
		{0x8421, 4, true, 0x0842, false},
		{0x1234, 0, false, 0x1234, false},
		{1, 15, false, 0x8000, false},
		{0x8000, 15, true, 1, false},
		{0x8000, 1, false, 0, true},
		{1, 1, true, 0, true},
		{4, 3, true, 0, true},
		{0xffff, 0x13, false, 0xfff8, true},
	} {
		setWord(t, r, "operand_a", "", tc.a, false)
		setWord(t, r, "operand_b", "", tc.n, false)
		drive(t, r, map[string]bool{"shift_right": tc.right})
		if got := word(t, r, "shifted", ""); got != tc.want {
			t.Errorf("%#x shift %d right %v = %#x, want %#x", tc.a, tc.n, tc.right, got, tc.want)
		}
		if got := read(t, r, "carry"); got != tc.carry {
			t.Errorf("%#x shift %d right %v: carry %v", tc.a, tc.n, tc.right, got)
		}
	}
}
