package main

import "image"

// halfAdder adds two bits: sum is their xor, carry their and.
func (l *Library) halfAdder() *Def {
	if d, ok := l.get("half_adder"); ok {
		return d
	}
	b := l.build("half_adder", 90, 38)
	x := b.Place(l.xorGate(), "sum_gate", 12, 4)
	a := b.Place(l.andGate(2), "carry_gate", 62, 20)
	b.Export("first", "first", image.Pt(0, 9))
	b.Export("second", "second", image.Pt(0, 24))
	b.Export("sum", "sum", image.Pt(89, 14))
	b.Export("carry", "carry", image.Pt(89, 30))
	b.Connect("first", x, "input_1")
	b.Connect("second", x, "input_2")
	b.Connect("first", a, "input_1")
	b.Connect("second", a, "input_2")
	b.Connect("sum", x, "output")
	b.Connect("carry", a, "output")
	return b.Route()
}

// fullAdder adds two bits and a carry: two half adders and an or.
func (l *Library) fullAdder() *Def {
	if d, ok := l.get("full_adder"); ok {
		return d
	}
	b := l.build("full_adder", 250, 60)
	first := b.Place(l.halfAdder(), "operands_adder", 12, 4)
	second := b.Place(l.halfAdder(), "carry_adder", 116, 4)
	or := b.Place(l.orGate(2), "carry_or", 215, 30)
	b.Export("first", "first", image.Pt(0, 13))
	b.Export("second", "second", image.Pt(0, 28))
	b.Export("carry_in", "carry_in", image.Pt(0, 50))
	b.Export("sum", "sum", image.Pt(249, 18))
	b.Export("carry_out", "carry_out", image.Pt(249, 37))
	b.Connect("first", first, "first")
	b.Connect("second", first, "second")
	b.Connect("partial_sum", first, "sum")
	b.Connect("partial_sum", second, "first")
	b.Connect("carry_in", second, "second")
	b.Connect("sum", second, "sum")
	b.Connect("operands_carry", first, "carry")
	b.Connect("sum_carry", second, "carry")
	b.Connect("operands_carry", or, "input_1")
	b.Connect("sum_carry", or, "input_2")
	b.Connect("carry_out", or, "output")
	return b.Route()
}

// incrementerSlice adds the carry from the bit below to one bit of the
// program counter: the value comes on lane 1, the result leaves on lane 2.
func (l *Library) incrementerSlice() *Def {
	if d, ok := l.get("incrementer_bit"); ok {
		return d
	}
	const w = 140
	b := l.build("incrementer_bit", w, sliceH)
	h := b.Place(l.halfAdder(), "half_adder", 24, 20)
	b.Export("bus", "bus_left", image.Pt(0, busLane))
	b.Export("bus", "bus_right", image.Pt(w-1, busLane))
	b.Export("value", "value", image.Pt(0, lane1))
	b.Export("plus_one", "plus_one", image.Pt(w-1, lane2))
	b.Export("carry_in", "carry_in", image.Pt(124, sliceH-1))
	b.Export("carry_out", "carry_out", image.Pt(124, 0))
	b.Connect("value", h, "first")
	b.Connect("carry_in", h, "second")
	b.Connect("plus_one", h, "sum")
	b.Connect("carry_out", h, "carry")
	return b.Route()
}

// incrementer adds 1 to the program counter.
func (l *Library) incrementer() *Def {
	return l.stackUnit(stack{
		name:     "incrementer",
		slice:    func(int) *Def { return l.incrementerSlice() },
		left:     12,
		right:    12,
		chain:    [][2]string{{"carry_out", "carry_in"}},
		carryOne: true,
	})
}
