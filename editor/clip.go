package editor

import (
	"fmt"
	"image"
	"strings"

	"github.com/Castux/mipsim2/bitmap"
	"github.com/Castux/mipsim2/doc"
)

// clip is a rectangle of content: pixels, labels and whole instances, in
// coordinates relative to its top-left corner. It is the clipboard format and
// the unit of move, delete and transform.
type clip struct {
	w, h   int
	pixels *bitmap.Bitmap
	labels []doc.Label
	insts  []doc.Instance
}

func (c *clip) empty() bool {
	return c.pixels.IsEmpty() && len(c.labels) == 0 && len(c.insts) == 0
}

// extract copies the content of r (local coordinates of def): its pixels and
// labels, and the instances lying entirely inside it.
func extract(d *doc.Document, def *doc.Definition, r image.Rectangle) *clip {
	c := &clip{w: r.Dx(), h: r.Dy(), pixels: bitmap.New()}
	def.Pixels.ForEachIn(r, func(x, y int) { c.pixels.Set(x-r.Min.X, y-r.Min.Y, true) })
	for _, l := range def.Labels {
		if l.Pos().In(r) {
			c.labels = append(c.labels, doc.Label{X: l.X - r.Min.X, Y: l.Y - r.Min.Y, Name: l.Name})
		}
	}
	for _, inst := range def.Instances {
		if d.PlacedRect(inst).In(r) {
			inst.X -= r.Min.X
			inst.Y -= r.Min.Y
			c.insts = append(c.insts, inst)
		}
	}
	return c
}

// clearRect removes the pixels and labels inside r from def and, if
// instances is set, the instances entirely inside it.
func clearRect(d *doc.Document, def *doc.Definition, r image.Rectangle, instances bool) {
	def.Pixels.ClearRect(r)
	labels := def.Labels[:0]
	for _, l := range def.Labels {
		if !l.Pos().In(r) {
			labels = append(labels, l)
		}
	}
	def.Labels = labels
	if !instances {
		return
	}
	insts := def.Instances[:0]
	for _, inst := range def.Instances {
		if !d.PlacedRect(inst).In(r) {
			insts = append(insts, inst)
		}
	}
	def.Instances = insts
}

// insert pastes c into def with its top-left at p, replacing the pixels and
// labels in that rectangle (paste is opaque). It never removes instances:
// pasting onto one breaks an invariant and the edit is rejected instead.
// Pasted instances get fresh IDs, and new names where theirs are taken.
func insert(d *doc.Document, def *doc.Definition, c *clip, p image.Point) {
	clearRect(d, def, image.Rect(p.X, p.Y, p.X+c.w, p.Y+c.h), false)
	c.pixels.ForEach(func(x, y int) { def.Pixels.Set(x+p.X, y+p.Y, true) })
	for _, l := range c.labels {
		def.Labels = append(def.Labels, doc.Label{X: l.X + p.X, Y: l.Y + p.Y, Name: l.Name})
	}
	for _, inst := range c.insts {
		inst.X += p.X
		inst.Y += p.Y
		inst.ID = freeID(def)
		inst.Name = freeName(def, inst.Name)
		def.Instances = append(def.Instances, inst)
	}
}

func freeID(def *doc.Definition) doc.InstID {
	for n := len(def.Instances) + 1; ; n++ {
		id := doc.InstID(fmt.Sprintf("i%d", n))
		taken := false
		for _, inst := range def.Instances {
			if inst.ID == id {
				taken = true
			}
		}
		if !taken {
			return id
		}
	}
}

func freeName(def *doc.Definition, name string) string {
	taken := func(s string) bool {
		for _, inst := range def.Instances {
			if inst.Name == s {
				return true
			}
		}
		return false
	}
	if !taken(name) {
		return name
	}
	base := name
	if i := strings.LastIndexByte(name, '_'); i > 0 {
		var n int
		if _, err := fmt.Sscanf(name[i+1:], "%d", &n); err == nil {
			base = name[:i]
		}
	}
	for n := 2; ; n++ {
		if s := fmt.Sprintf("%s_%d", base, n); !taken(s) {
			return s
		}
	}
}

// transform returns c rotated or mirrored by o. Its rectangle's top-left
// stays at the origin; instances compose their orientation.
func (c *clip) transform(d *doc.Document, o doc.Orient) *clip {
	f := o.Affine(c.w, c.h)
	w, h := o.Size(c.w, c.h)
	n := &clip{w: w, h: h, pixels: bitmap.New()}
	c.pixels.ForEach(func(x, y int) {
		p := f.Apply(image.Pt(x, y))
		n.pixels.Set(p.X, p.Y, true)
	})
	for _, l := range c.labels {
		p := f.Apply(l.Pos())
		n.labels = append(n.labels, doc.Label{X: p.X, Y: p.Y, Name: l.Name})
	}
	for _, inst := range c.insts {
		r := f.ApplyRect(d.PlacedRect(inst))
		inst.Orient = inst.Orient.Then(o)
		inst.X, inst.Y = r.Min.X, r.Min.Y
		n.insts = append(n.insts, inst)
	}
	return n
}

func bitmapFromRows(rows []string) (*bitmap.Bitmap, error) {
	return bitmap.FromRows(rows...)
}
