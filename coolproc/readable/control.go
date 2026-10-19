package main

import (
	"fmt"
	"image"
	"sort"
	"strings"
)

// A logic block computes named outputs from named inputs as sums of
// products. "!x" is the inverse of x (an inverter in the block). The
// drawing has columns: the inputs on the left edge, inverters, AND gates,
// OR gates, the outputs on the right edge.
type logic struct {
	id      string
	inputs  []string
	outputs []string
	// each output is an OR of products, each product an AND of literals
	terms map[string][][]string
}

func (l *Library) logicBlock(lg logic) *Def {
	if d, ok := l.get(lg.id); ok {
		return d
	}
	// Inverters needed.
	var invs []string
	seenInv := map[string]bool{}
	lit := func(s string) string {
		if strings.HasPrefix(s, "!") {
			base := s[1:]
			if !seenInv[base] {
				seenInv[base] = true
				invs = append(invs, base)
			}
			return base + "_inverted"
		}
		return s
	}
	type gate struct {
		def  *Def
		out  string
		ins  []string
		name string
	}
	var ands, ors []gate
	for _, out := range lg.outputs {
		products := lg.terms[out]
		var sums []string
		for k, p := range products {
			var ins []string
			for _, s := range p {
				ins = append(ins, lit(s))
			}
			if len(ins) == 1 {
				sums = append(sums, ins[0])
				continue
			}
			name := out
			if len(products) > 1 {
				name = fmt.Sprintf("%s_term_%d", out, k+1)
			}
			ands = append(ands, gate{l.andGate(len(ins)), name, ins, name + "_gate"})
			sums = append(sums, name)
		}
		if len(sums) > 1 {
			ors = append(ors, gate{l.orGate(len(sums)), out, sums, out + "_gate"})
		} else if sums[0] != out {
			// An output that is just one input or product: a buffer keeps
			// the names apart.
			ands = append(ands, gate{l.buffer(), out, []string{sums[0]}, out + "_buffer"})
		}
	}
	// Geometry.
	const gap = 8
	colH := func(gs []gate) int {
		h := 0
		for _, g := range gs {
			h += g.def.H + gap
		}
		return h
	}
	inputsH := 6*len(lg.inputs) + 10
	invH := len(invs) * (l.nand(1).H + gap)
	h := max(inputsH, invH, colH(ands), colH(ors), 6*len(lg.outputs)+10) + 8
	x0 := 20 + 6*len(lg.inputs) // room for the input lines to fan out
	x1 := x0 + 30 + 6*len(invs)
	x2 := x1 + 50 + 4*len(ands)
	w := x2 + 50 + 4*len(ors) + 10
	b := l.build(lg.id, w, h)
	for k, in := range lg.inputs {
		b.Export(in, in, image.Pt(0, 4+6*k))
	}
	for k, out := range lg.outputs {
		b.Export(out, out, image.Pt(w-1, 4+6*k))
	}
	y := 4
	for _, s := range invs {
		g := b.Place(l.nand(1), s+"_inverter", x0, y)
		b.Connect(s, g, "input")
		b.Connect(s+"_inverted", g, "output")
		y += l.nand(1).H + gap
	}
	place := func(gs []gate, x int) {
		y := 4
		for _, g := range gs {
			in := b.Place(g.def, g.name, x, y)
			for k, s := range g.ins {
				port := inputName(k, len(g.ins))
				if g.def.ID == "buffer" {
					port = "input"
				}
				b.Connect(s, in, port)
			}
			b.Connect(g.out, in, "output")
			y += g.def.H + gap
		}
	}
	place(ands, x1)
	place(ors, x2)
	return b.Route()
}

// sortedKeys is for deterministic iteration in tests.
func sortedKeys(m map[string][][]string) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// flipFlop is a master-slave D flip-flop of two latches: it takes data
// when the clock rises.
func (l *Library) flipFlop() *Def {
	if d, ok := l.get("flip_flop"); ok {
		return d
	}
	b := l.build("flip_flop", 130, 50)
	inv := b.Place(l.nand(1), "clock_inverter", 8, 40)
	master := b.Place(l.latch(), "master", 22, 4)
	slave := b.Place(l.latch(), "slave", 80, 4)
	b.Export("data", "data", image.Pt(0, 9))
	b.Export("clock", "clock", image.Pt(0, 43))
	b.Export("not_reset", "not_reset", image.Pt(0, 47))
	b.Export("value", "value", image.Pt(129, 8))
	b.Export("value_inverted", "value_inverted", image.Pt(129, 23))
	b.Connect("clock", inv, "input")
	b.Connect("clock_inverted", inv, "output")
	b.Connect("data", master, "data")
	b.Connect("clock_inverted", master, "write") // open while the clock is low
	b.Connect("not_reset", master, "not_reset")
	b.Connect("held", master, "value")
	b.Connect("held", slave, "data")
	b.Connect("clock", slave, "write") // passes it on while the clock is high
	b.Connect("not_reset", slave, "not_reset")
	b.Connect("value", slave, "value")
	b.Connect("value_inverted", slave, "value_inverted")
	return b.Route()
}

// stepCounter is a Johnson counter of six flip-flops: twelve states, one
// clock tick each, in which one flip-flop changes at a time.
func (l *Library) stepCounter() *Def {
	if d, ok := l.get("step_counter"); ok {
		return d
	}
	ff := l.flipFlop()
	b := l.build("step_counter", 6*(ff.W+30)+40, ff.H+60)
	b.Export("clock", "clock", image.Pt(0, 30))
	b.Export("not_reset", "not_reset", image.Pt(0, 40))
	for k := range 6 {
		f := b.Place(ff, fmt.Sprintf("stage_%d", k), 30+k*(ff.W+30), 20)
		data := fmt.Sprintf("stage_%d", k-1)
		if k == 0 {
			data = "stage_5_inverted" // the twist that makes it a Johnson counter
		}
		b.Connect(data, f, "data")
		b.Connect("clock", f, "clock")
		b.Connect("not_reset", f, "not_reset")
		b.Connect(fmt.Sprintf("stage_%d", k), f, "value")
		b.Connect(fmt.Sprintf("stage_%d_inverted", k), f, "value_inverted")
		b.Export(fmt.Sprintf("stage_%d", k), fmt.Sprintf("stage_%d", k), image.Pt(30+k*(ff.W+30)+20, ff.H+59))
		b.Export(fmt.Sprintf("stage_%d_inverted", k), fmt.Sprintf("stage_%d_inverted", k), image.Pt(30+k*(ff.W+30)+40, ff.H+59))
	}
	return b.Route()
}

// instructionRegister holds the instruction fetched from the program
// memory: 16 latches side by side.
func (l *Library) instructionRegister() *Def {
	if d, ok := l.get("instruction_register"); ok {
		return d
	}
	la := l.latch()
	pitch := la.W + 30
	b := l.build("instruction_register", 16*pitch+40, la.H+60)
	b.Export("write", "write", image.Pt(0, 20))
	b.Export("not_reset", "not_reset", image.Pt(0, 30))
	for i := range 16 {
		x := 30 + (15-i)*pitch // bit 15 on the left
		in := b.Place(la, fmt.Sprintf("bit_%d", i), x, 14)
		data := fmt.Sprintf("data_%d", i)
		b.Export(data, data, image.Pt(x+5, 0))
		b.Connect(data, in, "data")
		b.Connect("write", in, "write")
		b.Connect("not_reset", in, "not_reset")
		v, vi := fmt.Sprintf("instruction_%d", i), fmt.Sprintf("instruction_%d_inverted", i)
		b.Connect(v, in, "value")
		b.Connect(vi, in, "value_inverted")
		b.Export(v, v, image.Pt(x+la.W+4, la.H+59))
		b.Export(vi, vi, image.Pt(x+la.W+10, la.H+59))
	}
	return b.Route()
}
