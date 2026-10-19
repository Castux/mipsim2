package main

import (
	"fmt"
	"image"
	"strings"

	"github.com/Castux/mipsim2/bitmap"
)

// Every datapath unit has the same frame, so that slices of every unit sit
// at the same height and lanes run straight from unit to unit.
const (
	unitTop    = 2*bitPitch + 30 // control ports on the top edge; room for header parts
	unitBottom = 2*bitPitch + 10 // room for footer parts
	unitHeight = unitTop + 16*bitPitch + unitBottom
)

// sliceY is the top of bit i's slice in a unit.
func sliceY(i int) int { return unitTop + (15-i)*bitPitch }

// stack describes a datapath unit made of 16 slices.
type stack struct {
	name  string
	slice func(i int) *Def
	left  int // x of the slices: room on their left for control lines
	right int // room on their right
	// control maps a slice port to the unit's control net for bit i
	// (controls enter through the top edge).
	control map[string]func(i int) string
	// chain connects a port of slice i to a port of slice i+1 (a carry):
	// {out, in}. Bit 0's in and bit 15's out become unit ports.
	chain [][2]string
	// carryOne ties bit 0's chain input high instead of making it a port.
	carryOne bool
	// extra adds parts and wiring of the unit's own.
	extra func(b *Builder, bits []*Inst)
}

// stackUnit builds a stack. Lanes (port pairs name_left and name_right,
// and single ports on the left or right edge at lane heights) become unit
// ports name_i; other ports of the slices become named nets.
func (l *Library) stackUnit(s stack) *Def {
	if d, ok := l.get(s.name); ok {
		return d
	}
	sl := s.slice(0)
	w := s.left + sl.W + s.right
	b := l.build(s.name, w, unitHeight)
	var bits []*Inst
	controls := map[string]bool{}
	var controlOrder []string
	chained := map[string]bool{}
	for _, c := range s.chain {
		chained[c[0]], chained[c[1]] = true, true
	}
	for i := range 16 {
		d := s.slice(i)
		in := b.Place(d, fmt.Sprintf("bit_%d", i), s.left, sliceY(i))
		bits = append(bits, in)
		for _, port := range d.order {
			p := d.Port(port)
			if chained[port] {
				continue
			}
			if c, ok := s.control[port]; ok {
				name := c(i)
				if name == "" {
					continue
				}
				if !controls[name] {
					controls[name] = true
					controlOrder = append(controlOrder, name)
				}
				b.Connect(name, in, port)
				continue
			}
			base, side := port, ""
			if strings.HasSuffix(port, "_left") {
				base, side = strings.TrimSuffix(port, "_left"), "left"
			} else if strings.HasSuffix(port, "_right") {
				base, side = strings.TrimSuffix(port, "_right"), "right"
			}
			name := fmt.Sprintf("%s_%d", base, i)
			switch {
			case side == "left" && hasPort(d, base+"_right"):
				b.Through(name, in, port, base+"_right") // a lane through the slice
			case side == "right" && hasPort(d, base+"_left"):
				// joined with its left side
			default:
				b.Connect(name, in, port)
			}
			y := sliceY(i) + p.Y
			switch {
			case side == "left" || side == "" && p.X == 0 && p.Y < parts:
				b.Export(name, name+"_left", image.Pt(0, y))
			case side == "right" || side == "" && p.X == d.W-1 && p.Y < parts:
				b.Export(name, name+"_right", image.Pt(w-1, y))
			}
		}
	}
	for k, name := range controlOrder {
		b.Export(name, name, image.Pt(4+6*k, 0))
	}
	for k, c := range s.chain {
		for i := range 15 {
			name := fmt.Sprintf("%s_%d", c[0], i)
			b.Connect(name, bits[i], c[0])
			b.Connect(name, bits[i+1], c[1])
		}
		x := 4 + 6*(len(controlOrder)+k)
		b.Connect(c[1]+"_0", bits[0], c[1])
		if s.carryOne {
			b.Tie(c[1]+"_0", bitmap.Power)
		} else {
			b.Export(c[1]+"_0", c[1]+"_0", image.Pt(x, unitHeight-1))
		}
		b.Export(c[0]+"_15", c[0]+"_15", image.Pt(x, 0))
		b.Connect(c[0]+"_15", bits[15], c[0])
	}
	if s.extra != nil {
		s.extra(b, bits)
	}
	return b.Route()
}

func hasPort(d *Def, name string) bool {
	_, ok := d.ports[name]
	return ok
}
