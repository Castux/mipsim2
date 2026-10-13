package main

import (
	"fmt"
	"image"
	"sort"

	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/netlist"
)

// Document turns a layout into a document whose root holds every cell,
// shifted by off.
func (l *Layout) Document(off image.Point) *doc.Document {
	d := doc.New()
	root := d.RootDef()
	for p, k := range l.Cells {
		root.Pixels.Put(p.X+off.X, p.Y+off.Y, k)
	}
	var pts []image.Point
	for p := range l.Labels {
		pts = append(pts, p)
	}
	sort.Slice(pts, func(i, j int) bool {
		if pts[i].Y != pts[j].Y {
			return pts[i].Y < pts[j].Y
		}
		return pts[i].X < pts[j].X
	})
	for _, p := range pts {
		root.Labels = append(root.Labels, doc.Label{X: p.X + off.X, Y: p.Y + off.Y, Name: l.Labels[p]})
	}
	return d
}

// Verify compiles the document and checks that the circuit is the design:
// no errors, one net per signal, and every transistor where a gate input
// should be, gated by that input, between the gate's output and ground.
func (d *Design) Verify(dc *doc.Document, l *Layout, off image.Point) (*netlist.Netlist, error) {
	nl := netlist.CompileDoc(dc, netlist.Options{})
	for _, dg := range nl.Diagnostics {
		if dg.Level == netlist.Error {
			return nil, fmt.Errorf("layout error: %v", dg)
		}
	}
	netOf := map[*Sig]netlist.NetID{}
	owner := map[netlist.NetID]*Sig{}
	for _, s := range d.sigs {
		id, ok := nl.Lookup(s.Name)
		if !ok {
			return nil, fmt.Errorf("signal %s has no net", s.Name)
		}
		if o, dup := owner[id]; dup {
			return nil, fmt.Errorf("signals %s and %s are one net", o.Name, s.Name)
		}
		owner[id] = s
		netOf[s] = id
	}
	byPos := map[image.Point]netlist.Transistor{}
	for _, t := range nl.Transistors {
		byPos[t.Pos] = t
	}
	n := 0
	for _, g := range d.gates {
		ry := l.rowY[g.Block][g.row]
		ty := ry + 1
		if g.row%2 == 1 {
			ty = ry - 1
		}
		for _, s := range g.In {
			p := image.Pt(trackX(s.col)+1+off.X, ty+off.Y)
			t, ok := byPos[p]
			if !ok {
				return nil, fmt.Errorf("gate %s: no transistor for input %s at %v", g.Out.Name, s.Name, p)
			}
			if t.Gate != netOf[s] {
				return nil, fmt.Errorf("gate %s: transistor at %v not gated by %s", g.Out.Name, p, s.Name)
			}
			out, gnd := t.A, t.B
			if out != netOf[g.Out] {
				out, gnd = gnd, out
			}
			if out != netOf[g.Out] || nl.Nets[gnd].Drive != netlist.DriveLow || owner[gnd] != nil {
				return nil, fmt.Errorf("gate %s: transistor at %v is not between the output and ground", g.Out.Name, p)
			}
			n++
		}
		if nl.Nets[netOf[g.Out]].Drive != netlist.DriveHigh {
			return nil, fmt.Errorf("gate %s: output is not pulled up", g.Out.Name)
		}
	}
	if n != len(nl.Transistors) {
		return nil, fmt.Errorf("%d transistors, the design has %d", len(nl.Transistors), n)
	}
	return nl, nil
}
