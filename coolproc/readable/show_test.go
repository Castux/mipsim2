package main

import (
	"os"
	"testing"
)

// TestShow writes the component named by SHOW to $SHOW_OUT, for looking at.
func TestShow(t *testing.T) {
	name := os.Getenv("SHOW")
	if name == "" {
		t.Skip("set SHOW")
	}
	l := newLibrary()
	d := map[string]func() *Def{
		"xor":     l.xorGate,
		"and":     func() *Def { return l.andGate(3) },
		"latch":   l.latch,
		"shifter": l.shifter,
		"alu":     l.alu,
		"alubit":  l.aluSlice,
		"a": func() *Def {
			return l.registerUnit("a_register", slice{name: "a_register_bit", driver: true, output: lane1})
		},
		"x": func() *Def {
			return l.registerUnit("x_register", slice{name: "x_register_bit", driver: true, selectAlt: lane3, pass: []int{lane3}})
		},
	}[name]()
	data, err := l.Document(benchRoot(d)).Save()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("SHOW_OUT"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
