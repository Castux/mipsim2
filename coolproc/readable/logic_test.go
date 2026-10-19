package main

import "testing"

func truth(t *testing.T, l *Library, d *Def, n int, f func([]bool) bool) {
	t.Helper()
	r := bench(t, l, d)
	for v := range 1 << n {
		in := map[string]bool{}
		var bits []bool
		for k := range n {
			bits = append(bits, v>>k&1 == 1)
			in[inputName(k, n)] = bits[k]
		}
		drive(t, r, in)
		if got := read(t, r, "output"); got != f(bits) {
			t.Errorf("%s%v = %v", d.ID, bits, got)
		}
	}
}

func TestLogic(t *testing.T) {
	l := newLibrary()
	truth(t, l, l.xorGate(), 2, func(b []bool) bool { return b[0] != b[1] })
	truth(t, l, l.andGate(2), 2, func(b []bool) bool { return b[0] && b[1] })
	truth(t, l, l.andGate(3), 3, func(b []bool) bool { return b[0] && b[1] && b[2] })
	truth(t, l, l.orGate(3), 3, func(b []bool) bool { return b[0] || b[1] || b[2] })
}
