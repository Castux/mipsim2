// Command readable builds the coolproc processor (proc.md) as a readable
// mipsim document: small components (gates, latches, registers, adders...)
// built from each other, each one a component of the document, wired by a
// router and labelled with plain names.
package main

import (
	"fmt"
	"image"
	"sort"

	"github.com/Castux/mipsim2/bitmap"
	"github.com/Castux/mipsim2/doc"
)

// Port is a connection point of a component: a wire cell on its edge.
type Port struct {
	Name string
	P    image.Point
}

// Def is a component: cells, labels, child instances and ports.
type Def struct {
	ID     string
	W, H   int
	cells  map[image.Point]bitmap.Kind
	labels map[image.Point]string
	insts  []*Inst
	ports  map[string]image.Point
	order  []string // port names in creation order
}

func newDef(id string, w, h int) *Def {
	return &Def{ID: id, W: w, H: h, cells: map[image.Point]bitmap.Kind{}, labels: map[image.Point]string{}, ports: map[string]image.Point{}}
}

// Port returns the position of a port, panicking if there is none.
func (d *Def) Port(name string) image.Point {
	p, ok := d.ports[name]
	if !ok {
		panic(fmt.Sprintf("%s has no port %q", d.ID, name))
	}
	return p
}

func (d *Def) addPort(name string, p image.Point) { d.port(name, p, true) }

// port adds a port, labelled or not (leaf gates leave theirs unlabelled to
// keep drawings readable: the gate's name says what the ports are).
func (d *Def) port(name string, p image.Point, label bool) {
	if _, dup := d.ports[name]; dup {
		panic(fmt.Sprintf("%s: port %q twice", d.ID, name))
	}
	if p.X != 0 && p.Y != 0 && p.X != d.W-1 && p.Y != d.H-1 {
		panic(fmt.Sprintf("%s: port %q at %v is not on the edge", d.ID, name, p))
	}
	d.ports[name] = p
	d.order = append(d.order, name)
	if label {
		d.labels[p] = name
	}
}

// Inst is a placed component.
type Inst struct {
	Name string
	Def  *Def
	X, Y int
}

func (i *Inst) Rect() image.Rectangle { return image.Rect(i.X, i.Y, i.X+i.Def.W, i.Y+i.Def.H) }

// Port returns a port's cell in the parent's coordinates.
func (i *Inst) Port(name string) image.Point { return i.Def.Port(name).Add(image.Pt(i.X, i.Y)) }

// outside returns the parent cell that connects to a port: its neighbour
// outside the instance.
func (i *Inst) outside(name string) image.Point {
	p := i.Def.Port(name)
	q := p.Add(image.Pt(i.X, i.Y))
	switch {
	case p.X == 0:
		return q.Add(image.Pt(-1, 0))
	case p.X == i.Def.W-1:
		return q.Add(image.Pt(1, 0))
	case p.Y == 0:
		return q.Add(image.Pt(0, -1))
	default:
		return q.Add(image.Pt(0, 1))
	}
}

// Library collects components and writes them as a document.
type Library struct {
	defs map[string]*Def
	list []*Def
}

func newLibrary() *Library { return &Library{defs: map[string]*Def{}} }

// add registers a component, or returns the one already built under its ID.
func (l *Library) get(id string) (*Def, bool) {
	d, ok := l.defs[id]
	return d, ok
}

func (l *Library) add(d *Def) *Def {
	if _, dup := l.defs[d.ID]; dup {
		panic("component built twice: " + d.ID)
	}
	l.defs[d.ID] = d
	l.list = append(l.list, d)
	return d
}

// Document writes the library with root as the top level.
func (l *Library) Document(root *Def) *doc.Document {
	dc := doc.New()
	var walk func(d *Def, dd *doc.Definition)
	fill := func(d *Def, dd *doc.Definition) {
		for p, k := range d.cells {
			dd.Pixels.Put(p.X, p.Y, k)
		}
		var pts []image.Point
		for p := range d.labels {
			pts = append(pts, p)
		}
		sort.Slice(pts, func(a, b int) bool {
			if pts[a].Y != pts[b].Y {
				return pts[a].Y < pts[b].Y
			}
			return pts[a].X < pts[b].X
		})
		for _, p := range pts {
			dd.Labels = append(dd.Labels, doc.Label{X: p.X, Y: p.Y, Name: d.labels[p]})
		}
		for k, in := range d.insts {
			dd.Instances = append(dd.Instances, doc.Instance{ID: doc.InstID(fmt.Sprintf("i%d", k+1)), Def: doc.DefID(in.Def.ID), X: in.X, Y: in.Y, Name: in.Name})
		}
	}
	seen := map[string]bool{}
	walk = func(d *Def, dd *doc.Definition) {
		fill(d, dd)
		for _, in := range d.insts {
			if seen[in.Def.ID] {
				continue
			}
			seen[in.Def.ID] = true
			cd := doc.NewDefinition(doc.DefID(in.Def.ID), in.Def.W, in.Def.H)
			dc.Defs[cd.ID] = cd
			walk(in.Def, cd)
		}
	}
	walk(root, dc.RootDef())
	return dc
}
