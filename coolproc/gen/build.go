package main

import (
	"fmt"
	"image"
	"sort"

	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/netlist"
)

// Document turns a layout into a document whose root holds every cell and
// label.
func (l *Layout) Document() *doc.Document {
	d := doc.New()
	root := d.RootDef()
	for p, kind := range l.Cells {
		root.Pixels.Put(p.X, p.Y, kind)
	}
	var positions []image.Point
	for p := range l.Labels {
		positions = append(positions, p)
	}
	sort.Slice(positions, func(i, j int) bool { // raster order, for a stable file
		if positions[i].Y != positions[j].Y {
			return positions[i].Y < positions[j].Y
		}
		return positions[i].X < positions[j].X
	})
	for _, p := range positions {
		root.Labels = append(root.Labels, doc.Label{X: p.X, Y: p.Y, Name: l.Labels[p]})
	}
	return d
}

// Verify compiles the document and checks that the circuit is the design:
// no errors, one net per signal, every transistor where a gate input
// should be, gated by that input, between the gate's output and ground,
// and every output pulled up.
func (d *Design) Verify(document *doc.Document, layout *Layout) (*netlist.Netlist, error) {
	nets := netlist.CompileDoc(document, netlist.Options{})
	for _, diagnostic := range nets.Diagnostics {
		if diagnostic.Level == netlist.Error {
			return nil, fmt.Errorf("layout error: %v", diagnostic)
		}
	}
	netOf := map[*Signal]netlist.NetID{}
	signalOf := map[netlist.NetID]*Signal{}
	for _, s := range d.signals {
		id, ok := nets.Lookup(s.Name)
		if !ok {
			return nil, fmt.Errorf("signal %s has no net", s.Name)
		}
		if other, shared := signalOf[id]; shared {
			return nil, fmt.Errorf("signals %s and %s are one net", other.Name, s.Name)
		}
		signalOf[id] = s
		netOf[s] = id
	}
	transistorAt := map[image.Point]netlist.Transistor{}
	for _, t := range nets.Transistors {
		transistorAt[t.Pos] = t
	}
	checked := 0
	for _, g := range d.gates {
		rowY := layout.gateRowY[g.Block][g.row]
		transistorY := rowY + 1
		if g.row%2 == 1 {
			transistorY = rowY - 1
		}
		for k, input := range g.Inputs {
			p := image.Pt(trackColumn(g.inputLines[k].track)+1, transistorY)
			t, ok := transistorAt[p]
			if !ok {
				return nil, fmt.Errorf("gate %s: no transistor for input %s at %v", g.Output.Name, input.Name, p)
			}
			if t.Gate != netOf[input] {
				return nil, fmt.Errorf("gate %s: the transistor at %v is not gated by %s", g.Output.Name, p, input.Name)
			}
			output, ground := t.A, t.B
			if output != netOf[g.Output] {
				output, ground = ground, output
			}
			if output != netOf[g.Output] || nets.Nets[ground].Drive != netlist.DriveLow || signalOf[ground] != nil {
				return nil, fmt.Errorf("gate %s: the transistor at %v is not between the output and ground", g.Output.Name, p)
			}
			checked++
		}
		if nets.Nets[netOf[g.Output]].Drive != netlist.DriveHigh {
			return nil, fmt.Errorf("gate %s: the output is not pulled up", g.Output.Name)
		}
	}
	if checked != len(nets.Transistors) {
		return nil, fmt.Errorf("%d transistors, the design has %d", len(nets.Transistors), checked)
	}
	return nets, nil
}
