package main

import (
	"fmt"
	"image"

	"github.com/Castux/mipsim2/bitmap"
)

// resultSelector passes the value whose select line is high (one at a
// time): an AND-OR of five value and select pairs, as NANDs.
func (l *Library) resultSelector() *Def {
	if d, ok := l.get("result_selector"); ok {
		return d
	}
	names := []string{"and", "or", "xor", "add", "shift"}
	b := l.build("result_selector", 64, 66)
	combine := b.Place(l.nand(5), "combine", 50, 24)
	for k, n := range names {
		g := b.Place(l.nand(2), "pick_"+n, 24, 3+12*k)
		b.Export(n+"_value", n+"_value", image.Pt(0, 4+12*k))
		b.Export("select_"+n, "select_"+n, image.Pt(0, 8+12*k))
		b.Connect(n+"_value", g, "input_1")
		b.Connect("select_"+n, g, "input_2")
		b.Connect(n+"_not_picked", g, "output")
		b.Connect(n+"_not_picked", combine, inputName(k, 5))
	}
	b.Export("result", "result", image.Pt(63, 25))
	b.Connect("result", combine, "output")
	return b.Route()
}

// aluSlice is one bit of the ALU. The operands come in on lanes 1 and 2
// (and the shifted value on lane 4), the result leaves on lane 3, the
// carry runs from the slice below to the slice above.
func (l *Library) aluSlice() *Def {
	if d, ok := l.get("alu_bit"); ok {
		return d
	}
	const w = 540
	b := l.build("alu_bit", w, sliceH)
	invA := b.Place(l.xorGate(), "operand_a_inverter", 40, parts+1)
	invB := b.Place(l.xorGate(), "operand_b_inverter", 40, parts+40)
	and := b.Place(l.andGate(2), "and_gate", 110, parts+2)
	or := b.Place(l.orGate(2), "or_gate", 110, parts+17)
	xor := b.Place(l.xorGate(), "xor_gate", 110, parts+40)
	add := b.Place(l.fullAdder(), "adder", 180, parts+18)
	sel := b.Place(l.resultSelector(), "result_selector", 460, parts+10)
	b.Export("bus", "bus_left", image.Pt(0, busLane))
	b.Export("bus", "bus_right", image.Pt(w-1, busLane))
	b.Export("operand_a", "operand_a", image.Pt(0, lane1))
	b.Export("operand_b", "operand_b", image.Pt(0, lane2))
	b.Export("shifted", "shifted", image.Pt(0, lane4))
	b.Export("result", "result", image.Pt(w-1, lane3))
	b.Export("carry_in", "carry_in", image.Pt(440, sliceH-1))
	b.Export("carry_out", "carry_out", image.Pt(440, 0))
	controls := []string{"invert_a", "invert_b", "select_and", "select_or", "select_xor", "select_add", "select_shift"}
	for k, c := range controls {
		b.Export(c, c, image.Pt(0, 20+8*k))
	}
	b.Connect("operand_a", invA, "input_1")
	b.Connect("invert_a", invA, "input_2")
	b.Connect("a", invA, "output")
	b.Connect("operand_b", invB, "input_1")
	b.Connect("invert_b", invB, "input_2")
	b.Connect("b", invB, "output")
	for _, g := range []*Inst{and, or, xor} {
		b.Connect("a", g, "input_1")
		b.Connect("b", g, "input_2")
	}
	b.Connect("a", add, "first")
	b.Connect("b", add, "second")
	b.Connect("carry_in", add, "carry_in")
	b.Connect("carry_out", add, "carry_out")
	b.Connect("and_value", and, "output")
	b.Connect("or_value", or, "output")
	b.Connect("xor_value", xor, "output")
	b.Connect("add_value", add, "sum")
	b.Connect("shifted", sel, "shift_value")
	for _, n := range []string{"and", "or", "xor", "add"} {
		b.Connect(n+"_value", sel, n+"_value")
	}
	for _, n := range []string{"and", "or", "xor", "add", "shift"} {
		b.Connect("select_"+n, sel, "select_"+n)
	}
	b.Connect("result", sel, "result")
	return b.Route()
}

// alu is the arithmetic and logic unit: 16 slices, with a ripple carry.
func (l *Library) alu() *Def {
	same := func(c string) func(int) string { return func(int) string { return c } }
	control := map[string]func(int) string{}
	for _, c := range []string{"invert_a", "invert_b", "select_and", "select_or", "select_xor", "select_add", "select_shift"} {
		control[c] = same(c)
	}
	return l.stackUnit(stack{
		name:    "alu",
		slice:   func(int) *Def { return l.aluSlice() },
		left:    60,
		right:   12,
		control: control,
		chain:   [][2]string{{"carry_out", "carry_in"}},
	})
}

// shiftMux picks keep, from_below or from_above by its three select lines
// (one high at a time).
func (l *Library) shiftMux() *Def {
	if d, ok := l.get("shift_selector"); ok {
		return d
	}
	b := l.build("shift_selector", 46, 42)
	combine := b.Place(l.nand(3), "combine", 34, 14)
	for k, n := range []string{"keep", "from_below", "from_above"} {
		g := b.Place(l.nand(2), "pick_"+n, 16, 3+12*k)
		b.Export(n, n, image.Pt(0, 4+12*k))
		b.Export("select_"+n, "select_"+n, image.Pt(0, 8+12*k))
		b.Connect(n, g, "input_1")
		b.Connect("select_"+n, g, "input_2")
		b.Connect(n+"_not_picked", g, "output")
		b.Connect(n+"_not_picked", combine, inputName(k, 3))
	}
	b.Export("output", "output", image.Pt(45, 15))
	b.Connect("output", combine, "output")
	return b.Route()
}

// shiftRowY is where the selectors of shifter position p sit: positions -1
// and 16, which catch the last bit shifted out, sit below bit 0 and above
// bit 15.
func shiftRowY(p int) int { return sliceY(p) + parts }

// shifter is a barrel shifter: four stages shifting A by 1, 2, 4 and 8
// when the matching bit of B is set, left or right. Positions -1 and 16
// catch the bit shifted out last, which becomes the shift's carry.
func (l *Library) shifter() *Def {
	if d, ok := l.get("shifter"); ok {
		return d
	}
	mux := l.shiftMux()
	const stageW = 150
	left := 50
	w := left + 4*stageW + 80
	b := l.build("shifter", w, unitHeight)
	pos := func(p int) string {
		switch p {
		case -1:
			return "below_0"
		case 16:
			return "above_15"
		}
		return fmt.Sprint(p)
	}
	// Lanes pass straight through, tapped for A (stage 0) and B[3:0].
	for i := range 16 {
		for _, lane := range []struct {
			name string
			y    int
		}{{"bus", busLane}, {"operand_a", lane1}, {"operand_b", lane2}} {
			n := fmt.Sprintf("%s_%d", lane.name, i)
			b.Export(n, n+"_left", image.Pt(0, sliceY(i)+lane.y))
			b.Export(n, n+"_right", image.Pt(w-1, sliceY(i)+lane.y))
		}
		n := fmt.Sprintf("shifted_%d", i)
		b.Export(n, n+"_right", image.Pt(w-1, sliceY(i)+lane4))
	}
	value := func(stage, p int) string {
		if stage == 0 {
			if p < 0 || p > 15 {
				return "zero"
			}
			return fmt.Sprintf("operand_a_%d", p)
		}
		if p < -1 || p > 16 {
			return "zero"
		}
		return fmt.Sprintf("by_%d_%s", 1<<(stage-1), pos(p))
	}
	zeros := 0
	for k := range 4 {
		s := 1 << k
		for p := -1; p <= 16; p++ {
			m := b.Place(mux, fmt.Sprintf("shift_by_%d_%s", s, pos(p)), left+k*stageW+60, shiftRowY(p))
			for _, pv := range [][2]string{{"keep", value(k, p)}, {"from_below", value(k, p-s)}, {"from_above", value(k, p+s)}} {
				port, v := pv[0], pv[1]
				if v == "zero" {
					// A constant 0 of its own, right at the port.
					v = fmt.Sprintf("zero_%d", zeros)
					zeros++
					b.Tie(v, bitmap.Ground)
					b.Quiet(v)
				}
				b.Connect(v, m, port)
			}
			b.Connect(fmt.Sprintf("keep_%d", k), m, "select_keep")
			b.Connect(fmt.Sprintf("left_%d", k), m, "select_from_below")
			b.Connect(fmt.Sprintf("right_%d", k), m, "select_from_above")
			out := fmt.Sprintf("by_%d_%s", s, pos(p))
			if k == 3 && p >= 0 && p <= 15 {
				out = fmt.Sprintf("shifted_%d", p)
			}
			b.Connect(out, m, "output")
		}
	}
	// Stage controls, from B[3:0] and the direction.
	ctl := b.Place(l.shiftControl(), "shift_control", left, shiftRowY(-1)+60)
	b.Export("shift_right", "shift_right", image.Pt(10, 0))
	b.Connect("shift_right", ctl, "shift_right")
	for k := range 4 {
		b.Connect(fmt.Sprintf("operand_b_%d", k), ctl, fmt.Sprintf("amount_%d", k))
		for _, n := range []string{"keep", "left", "right"} {
			b.Connect(fmt.Sprintf("%s_%d", n, k), ctl, fmt.Sprintf("%s_%d", n, k))
		}
	}
	// The carry: the bit shifted out last, above 15 for a left shift,
	// below 0 for a right shift.
	carry := b.Place(l.selector(), "carry_selector", left+3*stageW+60, 40)
	b.Connect("by_8_above_15", carry, "first")
	b.Connect("by_8_below_0", carry, "second")
	b.Connect("shift_right", carry, "choose")
	b.Export("carry", "carry", image.Pt(20, 0))
	b.Connect("carry", carry, "output")
	return b.Route()
}

// shiftControl turns B[3:0] and the direction into each stage's selects:
// keep when the bit is 0, else take from below (left) or above (right).
// Its ports are all on the top edge: it sits under the shifter.
func (l *Library) shiftControl() *Def {
	if d, ok := l.get("shift_control"); ok {
		return d
	}
	b := l.build("shift_control", 380, 50)
	b.Export("shift_right", "shift_right", image.Pt(6, 0))
	shiftLeft := b.Place(l.nand(1), "direction_inverter", 10, 30)
	b.Connect("shift_right", shiftLeft, "input")
	b.Connect("shift_left", shiftLeft, "output")
	for k := range 4 {
		x := 40 + 85*k
		amount := fmt.Sprintf("amount_%d", k)
		b.Export(amount, amount, image.Pt(x+2, 0))
		keep := b.Place(l.nand(1), fmt.Sprintf("keep_%d_inverter", k), x, 24)
		left := b.Place(l.andGate(2), fmt.Sprintf("left_%d_gate", k), x+14, 8)
		right := b.Place(l.andGate(2), fmt.Sprintf("right_%d_gate", k), x+14, 28)
		b.Connect(amount, keep, "input")
		b.Connect(amount, left, "input_1")
		b.Connect("shift_left", left, "input_2")
		b.Connect(amount, right, "input_1")
		b.Connect("shift_right", right, "input_2")
		for j, n := range []string{"keep", "left", "right"} {
			name := fmt.Sprintf("%s_%d", n, k)
			b.Export(name, name, image.Pt(x+50+8*j, 0))
		}
		b.Connect(fmt.Sprintf("keep_%d", k), keep, "output")
		b.Connect(fmt.Sprintf("left_%d", k), left, "output")
		b.Connect(fmt.Sprintf("right_%d", k), right, "output")
	}
	return b.Route()
}
