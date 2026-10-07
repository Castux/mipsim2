package doc

import (
	"image"
	"strings"

	"github.com/Castux/mipsim2/bitmap"
)

// Flat is a document flattened to world coordinates, the input of the
// pattern compiler and the renderer.
type Flat struct {
	Pixels    *bitmap.Bitmap
	Labels    []FlatLabel
	Instances []FlatInstance
}

// FlatLabel is a label at its world position with its hierarchical name.
type FlatLabel struct {
	Pos   image.Point
	Name  string // full name, e.g. "alu.add3.sum_2"
	Scope string // instance path the label comes from, e.g. "alu.add3"; "" for the root
}

// FlatInstance is an instance's rectangle in world coordinates.
type FlatInstance struct {
	Path string // e.g. "alu.add3"
	Def  DefID
	Rect image.Rectangle
	Map  Affine // local coordinates of the definition -> world
}

// Flatten expands every instance into one world bitmap. The document must be
// valid. Output order is deterministic: root first, then instances depth-first
// in their slice order.
func (d *Document) Flatten() *Flat {
	f := &Flat{Pixels: bitmap.New()}
	d.flattenInto(f, d.RootDef(), IdentityAffine, nil)
	return f
}

func (d *Document) flattenInto(f *Flat, def *Definition, toWorld Affine, path []string) {
	scope := strings.Join(path, ".")
	prefix := ""
	if scope != "" {
		prefix = scope + "."
	}

	def.Pixels.ForEach(func(x, y int) {
		p := toWorld.Apply(image.Pt(x, y))
		f.Pixels.Set(p.X, p.Y, true)
	})
	for _, l := range def.Labels {
		f.Labels = append(f.Labels, FlatLabel{Pos: toWorld.Apply(l.Pos()), Name: prefix + l.Name, Scope: scope})
	}
	names := d.InstanceNames(def)
	for i, inst := range def.Instances {
		child := d.Defs[inst.Def]
		m := d.Placement(inst).Then(toWorld)
		childPath := append(path[:len(path):len(path)], names[i])
		f.Instances = append(f.Instances, FlatInstance{
			Path: strings.Join(childPath, "."),
			Def:  inst.Def,
			Rect: m.ApplyRect(child.Rect()),
			Map:  m,
		})
		d.flattenInto(f, child, m, childPath)
	}
}

// Location is where a world point resolves to for editing: the deepest
// definition whose instance rectangle contains it.
type Location struct {
	Path  []int // instance indices from the root down; empty for the root
	Def   DefID // the definition being edited
	Local image.Point
	Map   Affine // local coordinates of Def -> world
}

// Locate resolves a world point by descending into the deepest instance whose
// rectangle contains it. Instance rectangles are opaque, so a point inside one
// always belongs to that instance's definition, even where it has no pixel.
func (d *Document) Locate(p image.Point) Location {
	loc := Location{Def: d.Root, Local: p, Map: IdentityAffine}
	def := d.RootDef()
	for {
		found := false
		for i, inst := range def.Instances {
			if !loc.Local.In(d.PlacedRect(inst)) {
				continue
			}
			place := d.Placement(inst)
			loc.Path = append(loc.Path, i)
			loc.Local = place.Inverse().Apply(loc.Local)
			loc.Map = place.Then(loc.Map)
			loc.Def = inst.Def
			def = d.Defs[inst.Def]
			found = true
			break
		}
		if !found {
			return loc
		}
	}
}
