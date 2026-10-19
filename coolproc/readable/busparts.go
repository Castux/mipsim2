package main

import (
	"fmt"
	"image"

	"github.com/Castux/mipsim2/bitmap"
)

// immediateSlice puts the instruction's argument byte on the bus: setl
// drives bits 0-7 with it and bits 8-15 with zeros, seth drives bits 8-15
// with it. Bus drivers take the inverted data (they pull the bus low for a
// 0).
func (l *Library) immediateSlice(high bool) *Def {
	id := "immediate_low_bit"
	if high {
		id = "immediate_high_bit"
	}
	if d, ok := l.get(id); ok {
		return d
	}
	const w = 70
	b := l.build(id, w, sliceH)
	b.Export("bus", "bus_left", image.Pt(0, busLane))
	b.Export("bus", "bus_right", image.Pt(w-1, busLane))
	b.Export("immediate_low", "immediate_low", image.Pt(0, 30))
	b.Export("argument_inverted", "argument_inverted", image.Pt(0, 50))
	low := b.Place(l.busDriver(), "low_byte_driver", 40, 24)
	b.Connect("immediate_low", low, "enable")
	b.Connect("bus", low, "bus")
	if !high {
		b.Connect("argument_inverted", low, "data_inverted")
		return b.Route()
	}
	// setl writes zeros up here: the data is always 0.
	b.Connect("always_zero_inverted", low, "data_inverted")
	b.Tie("always_zero_inverted", bitmap.Power)
	b.Export("immediate_high", "immediate_high", image.Pt(0, 40))
	hi := b.Place(l.busDriver(), "high_byte_driver", 40, 44)
	b.Connect("immediate_high", hi, "enable")
	b.Connect("argument_inverted", hi, "data_inverted")
	b.Connect("bus", hi, "bus")
	return b.Route()
}

// immediate is the unit that puts the instruction argument on the bus.
func (l *Library) immediate() *Def {
	return l.stackUnit(stack{
		name:  "immediate",
		slice: func(i int) *Def { return l.immediateSlice(i >= 8) },
		left:  60,
		right: 12,
		control: map[string]func(int) string{
			"immediate_low":  func(int) string { return "immediate_low_to_bus" },
			"immediate_high": func(int) string { return "immediate_high_to_bus" },
			"argument_inverted": func(i int) string {
				return fmt.Sprintf("argument_inverted_%d", i%8)
			},
		},
	})
}

// busEnd is where the bus ends: a pull-up on every line (the bus reads 1
// when nothing drives it), and the zero detector for the jumps.
func (l *Library) busEnd() *Def {
	if d, ok := l.get("bus_end"); ok {
		return d
	}
	const w = 160
	b := l.build("bus_end", w, unitHeight)
	zero := b.Place(l.andGate(4), "all_groups_zero", 110, unitTop-60)
	for k := range 4 {
		// Each NOR sits next to the four bus bits it reads.
		g := b.Place(l.nor(4), fmt.Sprintf("bits_%d_to_%d_zero", 4*k, 4*k+3), 60, sliceY(4*k+2)+20)
		for j := range 4 {
			i := 4*k + j
			name := fmt.Sprintf("bus_%d", i)
			b.Export(name, name+"_left", image.Pt(0, sliceY(i)+busLane))
			b.Connect(name, g, inputName(j, 4))
			pull := b.Place(l.pullUp(), fmt.Sprintf("pull_up_%d", i), 20, sliceY(i)+20)
			b.Connect(name, pull, "line")
		}
		group := fmt.Sprintf("bits_%d_to_%d_zero", 4*k, 4*k+3)
		b.Connect(group, g, "output")
		b.Connect(group, zero, inputName(k, 4))
	}
	b.Export("bus_is_zero", "bus_is_zero", image.Pt(10, 0))
	b.Connect("bus_is_zero", zero, "output")
	b.Export("bus_15", "bus_is_negative", image.Pt(20, 0))
	return b.Route()
}

// pullUp is a power cell on a line: it reads 1 unless something pulls it
// low.
func (l *Library) pullUp() *Def {
	if d, ok := l.get("pull_up"); ok {
		return d
	}
	d := fromRows("pull_up", []string{"...", "#H.", "..."})
	d.port("line", image.Pt(0, 1), false)
	return l.add(d)
}
