package main

import "image"

// latch is a gated D latch of four NANDs: while write is high, value
// follows data; when write is low, value holds. not_reset low clears it.
func (l *Library) latch() *Def {
	if d, ok := l.get("latch"); ok {
		return d
	}
	b := l.build("latch", 40, 36)
	set := b.Place(l.nand(3), "set_gate", 10, 3)
	clear := b.Place(l.nand(2), "clear_gate", 10, 19)
	value := b.Place(l.nand(2), "value_gate", 25, 3)
	inverted := b.Place(l.nand(3), "inverted_gate", 25, 18)
	b.Export("data", "data", image.Pt(0, 5))
	b.Export("write", "write", image.Pt(0, 15))
	b.Export("not_reset", "not_reset", image.Pt(0, 33))
	b.Export("value", "value", image.Pt(39, 4))
	b.Export("value_inverted", "value_inverted", image.Pt(39, 19))
	b.Connect("data", set, "input_1")
	b.Connect("write", set, "input_2")
	b.Connect("not_reset", set, "input_3")
	b.Connect("set_inverted", set, "output")
	b.Connect("set_inverted", clear, "input_1")
	b.Connect("write", clear, "input_2")
	b.Connect("clear_inverted", clear, "output")
	b.Connect("set_inverted", value, "input_1")
	b.Connect("value_inverted", value, "input_2")
	b.Connect("value", value, "output")
	b.Connect("clear_inverted", inverted, "input_1")
	b.Connect("value", inverted, "input_2")
	b.Connect("not_reset", inverted, "input_3")
	b.Connect("value_inverted", inverted, "output")
	return b.Route()
}
