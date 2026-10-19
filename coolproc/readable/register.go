package main

import (
	"fmt"
	"image"
)

// Datapath geometry. Every datapath unit is a column of 16 bit slices, bit
// 15 at the top, and every slice is bitPitch cells tall. Data runs across
// the slices on horizontal lanes at the top of each slice, at the same
// height in every unit, so the lanes are straight lines from unit to unit.
// The slice's components sit below the lanes.
const (
	bitPitch = 100
	sliceH   = bitPitch - 4 // slices are stacked with a gap
	busLane  = 2            // the bus, through every unit
	lane1    = 4            // A to the ALU; PC to the incrementer
	lane2    = 6            // B to the ALU; PC + 1 to the temporary register
	lane3    = 8            // the ALU result to X and Y
	lane4    = 10           // the shifted value to the ALU
	parts    = 14           // components start here
)

// slice describes one bit slice of a register unit.
type slice struct {
	name      string // component name
	fromLane  int    // data from this lane instead of the bus (0: the bus)
	selectAlt int    // lane of a second source, picked by "select" (0: none)
	driver    bool   // drives the bus when "read" is high
	output    int    // lane the value goes out on (0: none)
	pass      []int  // lanes passing straight through
	whole     bool   // one write line for all 16 bits (no separate halves)
}

func (s slice) passes(lane int) bool {
	for _, p := range s.pass {
		if p == lane {
			return true
		}
	}
	return false
}

// selector picks second when choose is high, else first.
func (l *Library) selector() *Def {
	if d, ok := l.get("selector"); ok {
		return d
	}
	b := l.build("selector", 36, 28)
	notChoose := b.Place(l.nand(1), "choose_inverter", 6, 18)
	takeFirst := b.Place(l.nand(2), "take_first", 16, 3)
	takeSecond := b.Place(l.nand(2), "take_second", 16, 15)
	combine := b.Place(l.nand(2), "combine", 27, 8)
	b.Export("first", "first", image.Pt(0, 5))
	b.Export("second", "second", image.Pt(0, 13))
	b.Export("choose", "choose", image.Pt(0, 24))
	b.Export("output", "output", image.Pt(35, 9))
	b.Connect("first", takeFirst, "input_1")
	b.Connect("not_choose", takeFirst, "input_2")
	b.Connect("choose", notChoose, "input")
	b.Connect("not_choose", notChoose, "output")
	b.Connect("second", takeSecond, "input_1")
	b.Connect("choose", takeSecond, "input_2")
	b.Connect("first_inverted", takeFirst, "output")
	b.Connect("second_inverted", takeSecond, "output")
	b.Connect("first_inverted", combine, "input_1")
	b.Connect("second_inverted", combine, "input_2")
	b.Connect("output", combine, "output")
	return b.Route()
}

// registerSlice builds one bit of a register: a latch, written from the bus
// (or a lane, or one of two sources), and optionally a bus driver.
func (l *Library) registerSlice(s slice) *Def {
	if d, ok := l.get(s.name); ok {
		return d
	}
	const w = 140
	b := l.build(s.name, w, sliceH)
	latch := b.Place(l.latch(), "latch", 64, parts)
	// Ports: lanes on both sides, controls on the left below the lanes.
	b.Export("bus", "bus_left", image.Pt(0, busLane))
	b.Export("bus", "bus_right", image.Pt(w-1, busLane))
	b.Export("write", "write", image.Pt(0, 36))
	b.Export("not_reset", "not_reset", image.Pt(0, 52))
	for _, lane := range s.pass {
		name := fmt.Sprintf("lane_%d", lane)
		b.Export(name, name+"_left", image.Pt(0, lane))
		b.Export(name, name+"_right", image.Pt(w-1, lane))
	}
	data := "bus"
	if s.fromLane > 0 {
		data = "data"
		b.Export(data, "data", image.Pt(0, s.fromLane))
	}
	if s.selectAlt > 0 {
		sel := b.Place(l.selector(), "source_selector", 16, parts)
		alt := "alternative"
		if s.passes(s.selectAlt) {
			alt = fmt.Sprintf("lane_%d", s.selectAlt) // tapped from the lane passing through
		} else {
			b.Export(alt, alt, image.Pt(0, s.selectAlt))
		}
		b.Export("select", "select", image.Pt(0, 46))
		b.Connect(data, sel, "first")
		b.Connect(alt, sel, "second")
		b.Connect("select", sel, "choose")
		data = "selected"
		b.Connect(data, sel, "output")
	}
	b.Connect(data, latch, "data")
	b.Connect("write", latch, "write")
	b.Connect("not_reset", latch, "not_reset")
	if s.driver {
		drv := b.Place(l.busDriver(), "bus_driver", 120, parts)
		b.Export("read", "read", image.Pt(0, 41))
		b.Connect("read", drv, "enable")
		b.Connect("value_inverted", latch, "value_inverted")
		b.Connect("value_inverted", drv, "data_inverted")
		b.Connect("bus", drv, "bus")
	}
	b.Connect("value", latch, "value")
	out := s.output
	if out == 0 {
		out = 24 // not on a lane: the value is only named, or tapped by a device
	}
	b.Export("value", "value", image.Pt(w-1, out))
	return b.Route()
}

// registerUnit stacks 16 slices into a 16-bit register. Controls come in
// from the top: write_low (bits 0-7), write_high (bits 8-15), read, select,
// not_reset.
func (l *Library) registerUnit(name string, s slice) *Def {
	same := func(c string) func(int) string { return func(int) string { return c } }
	return l.stackUnit(stack{
		name:  name,
		slice: func(int) *Def { return l.registerSlice(s) },
		left:  40,
		right: 12,
		control: map[string]func(int) string{
			"write": func(i int) string {
				if s.whole {
					return "write"
				}
				if i < 8 {
					return "write_low"
				}
				return "write_high"
			},
			"read":      same("read"),
			"select":    same("select"),
			"not_reset": same("not_reset"),
		},
	})
}
