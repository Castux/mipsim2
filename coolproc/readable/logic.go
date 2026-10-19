package main

import (
	"fmt"
	"image"
)

// Small logic components, built from the leaf gates.

// xorGate is the classic four-NAND exclusive or.
func (l *Library) xorGate() *Def {
	if d, ok := l.get("xor_gate"); ok {
		return d
	}
	b := l.build("xor_gate", 40, 32)
	nand := l.nand(2)
	first := b.Place(nand, "both", 9, 11)
	top := b.Place(nand, "first_and_not_both", 20, 2)
	bottom := b.Place(nand, "second_and_not_both", 20, 21)
	out := b.Place(nand, "combine", 31, 11)
	b.Export("input_1", "input_1", image.Pt(0, 5))
	b.Export("input_2", "input_2", image.Pt(0, 26))
	b.Export("output", "output", image.Pt(39, 12))
	b.Connect("input_1", first, "input_1")
	b.Connect("input_2", first, "input_2")
	b.Connect("input_1", top, "input_1")
	b.Connect("not_both", first, "output")
	b.Connect("not_both", top, "input_2")
	b.Connect("not_both", bottom, "input_1")
	b.Connect("input_2", bottom, "input_2")
	b.Connect("first_only", top, "output")
	b.Connect("second_only", bottom, "output")
	b.Connect("first_only", out, "input_1")
	b.Connect("second_only", out, "input_2")
	b.Connect("output", out, "output")
	return b.Route()
}

// andGate is a NAND followed by an inverter.
func (l *Library) andGate(n int) *Def {
	id := fmt.Sprintf("and_%d", n)
	if d, ok := l.get(id); ok {
		return d
	}
	nand := l.nand(n)
	b := l.build(id, nand.W+16, nand.H+4)
	g := b.Place(nand, "nand", 4, 2)
	inv := b.Place(l.nand(1), "inverter", nand.W+8, 2)
	for k := range n {
		name := inputName(k, n)
		b.Export(name, name, image.Pt(0, 2+g.Def.Port(name).Y))
		b.Connect(name, g, name)
	}
	b.Connect("not_all", g, "output")
	b.Connect("not_all", inv, "input")
	b.Connect("output", inv, "output")
	b.Export("output", "output", image.Pt(b.def.W-1, 3))
	return b.Route()
}

// orGate is a NOR followed by an inverter.
func (l *Library) orGate(n int) *Def {
	id := fmt.Sprintf("or_%d", n)
	if d, ok := l.get(id); ok {
		return d
	}
	nor := l.nor(n)
	b := l.build(id, nor.W+16, nor.H+4)
	g := b.Place(nor, "nor", 4, 2)
	inv := b.Place(l.nand(1), "inverter", nor.W+8, 2)
	for k := range n {
		name := inputName(k, n)
		b.Export(name, name, image.Pt(0, 2+g.Def.Port(name).Y))
		b.Connect(name, g, name)
	}
	b.Connect("none", g, "output")
	b.Connect("none", inv, "input")
	b.Connect("output", inv, "output")
	b.Export("output", "output", image.Pt(b.def.W-1, 3))
	return b.Route()
}

// buffer is two inverters: it copies its input to a net of its own name.
func (l *Library) buffer() *Def {
	if d, ok := l.get("buffer"); ok {
		return d
	}
	b := l.build("buffer", 26, 9)
	first := b.Place(l.nand(1), "first_inverter", 4, 2)
	second := b.Place(l.nand(1), "second_inverter", 16, 2)
	b.Export("input", "input", image.Pt(0, 4))
	b.Export("output", "output", image.Pt(25, 3))
	b.Connect("input", first, "input")
	b.Connect("inverted", first, "output")
	b.Connect("inverted", second, "input")
	b.Connect("output", second, "output")
	return b.Route()
}
