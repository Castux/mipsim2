package editor

import (
	"fmt"
	"image"
	"slices"
	"strings"

	"github.com/Castux/mipsim2/bitmap"
	"github.com/Castux/mipsim2/doc"
)

// typingKind is what a text entry names.
type typingKind int

const (
	typeLabel typingKind = iota
	typeDefName
	typeInstName
)

func (k typingKind) String() string {
	return [...]string{"label", "component name", "instance name"}[k]
}

// DefInfo describes a definition for the components palette.
type DefInfo struct {
	ID        doc.DefID
	Name      string
	W, H      int
	Instances int // placements anywhere in the document
}

// Definitions lists every component definition (not the root), by name.
func (e *Editor) Definitions() []DefInfo {
	counts := map[doc.DefID]int{} // looked up only
	for _, id := range e.Doc.DefIDs() {
		for _, inst := range e.Doc.Defs[id].Instances {
			counts[inst.Def]++
		}
	}
	var out []DefInfo
	for _, id := range e.Doc.DefIDs() {
		if id == e.Doc.Root {
			continue
		}
		d := e.Doc.Defs[id]
		out = append(out, DefInfo{ID: id, Name: d.Name, W: d.W, H: d.H, Instances: counts[id]})
	}
	slices.SortStableFunc(out, func(a, b DefInfo) int {
		if a.Name < b.Name {
			return -1
		} else if a.Name > b.Name {
			return 1
		}
		return 0
	})
	return out
}

// freeDefID returns an unused definition ID "partN".
func (e *Editor) freeDefID() doc.DefID {
	for n := 1; ; n++ {
		id := doc.DefID(fmt.Sprintf("part%d", n))
		if e.Doc.Defs[id] == nil {
			return id
		}
	}
}

// makeComponent turns the selection into a new definition and replaces it
// with one instance, then asks for a name.
func (e *Editor) makeComponent() {
	if e.sel == nil || e.sel.rect.Empty() {
		e.Status = "select an area to make a component from"
		return
	}
	s := e.sel
	id := e.freeDefID()
	ok := e.change("make component", []doc.DefID{s.def, id}, func() error {
		parent := e.Doc.Defs[s.def]
		c := extract(e.Doc, parent, s.rect)
		def := doc.NewDefinition(id, c.w, c.h)
		insert(e.Doc, def, c, image.Point{})
		e.Doc.Defs[id] = def
		clearRect(e.Doc, parent, s.rect, true)
		parent.Instances = append(parent.Instances, doc.Instance{
			ID: freeID(parent), Def: id, X: s.rect.Min.X, Y: s.rect.Min.Y, // unnamed: named after the component
		})
		return nil
	})
	if !ok {
		return
	}
	e.sel = s
	e.typing = &typing{kind: typeDefName, def: id, buffer: string(id), old: string(id)}
	e.Status = "component name: " + string(id) + "_   (enter to apply)"
}

// selectedInstances returns the indices, in the selection's definition, of
// the instances entirely inside the selection.
func (e *Editor) selectedInstances() []int {
	if e.sel == nil {
		return nil
	}
	var out []int
	for i, inst := range e.Doc.Defs[e.sel.def].Instances {
		if e.Doc.PlacedRect(inst).In(e.sel.rect) {
			out = append(out, i)
		}
	}
	return out
}

// selectedInstance returns the instance the selection is exactly, if any.
func (e *Editor) selectedInstance() (doc.Instance, bool) {
	if e.sel == nil {
		return doc.Instance{}, false
	}
	for _, inst := range e.Doc.Defs[e.sel.def].Instances {
		if e.Doc.PlacedRect(inst) == e.sel.rect {
			return inst, true
		}
	}
	return doc.Instance{}, false
}

// explode replaces the selected instances by copies of their content.
func (e *Editor) explode() {
	idx := e.selectedInstances()
	if len(idx) == 0 {
		e.Status = "select a component to explode"
		return
	}
	s := e.sel
	e.change("explode", []doc.DefID{s.def}, func() error {
		parent := e.Doc.Defs[s.def]
		var keep []doc.Instance
		var clips []*clip
		var at []image.Point
		for i, inst := range parent.Instances {
			if !slices.Contains(idx, i) {
				keep = append(keep, inst)
				continue
			}
			child := e.Doc.Defs[inst.Def]
			clips = append(clips, extract(e.Doc, child, child.Rect()).transform(e.Doc, inst.Orient))
			at = append(at, image.Pt(inst.X, inst.Y))
		}
		parent.Instances = keep
		for i, c := range clips {
			insert(e.Doc, parent, c, at[i])
		}
		return nil
	})
	e.sel = s
}

// startRenameInstance asks for a new name for the selected instance.
func (e *Editor) startRenameInstance() {
	inst, ok := e.selectedInstance()
	if !ok {
		e.Status = "select one component to rename (click it)"
		return
	}
	e.typing = &typing{kind: typeInstName, def: e.sel.def, inst: inst.ID, buffer: inst.Name, old: inst.Name}
	auto := e.Doc.InstanceName(e.Doc.Defs[e.sel.def], slices.IndexFunc(e.Doc.Defs[e.sel.def].Instances, func(i doc.Instance) bool { return i.ID == inst.ID }))
	if inst.Name == "" {
		e.Status = "instance name: _   (empty keeps the automatic name " + auto + ")"
	} else {
		e.Status = "instance name: " + inst.Name + "_   (empty: named after its component)"
	}
}

// StartRenameDefinition asks for a new display name for a definition.
func (e *Editor) StartRenameDefinition(id doc.DefID) {
	d := e.Doc.Defs[id]
	if d == nil {
		return
	}
	e.typing = &typing{kind: typeDefName, def: id, buffer: d.Name, old: d.Name}
	e.Status = "component name: " + d.Name + "_"
}

func (e *Editor) renameDefinition(id doc.DefID, name string) {
	if name == "" {
		e.Status = "a component needs a name"
		return
	}
	e.change("rename component", []doc.DefID{id}, func() error {
		e.Doc.Defs[id].Name = name
		return nil
	})
}

func (e *Editor) renameInstance(parent doc.DefID, inst doc.InstID, name string) {
	if name != "" && !doc.ValidName(name) {
		e.Status = fmt.Sprintf("invalid instance name %q: use letters, digits and underscores (empty: named after its component)", name)
		return
	}
	s := e.sel
	e.change("rename instance", []doc.DefID{parent}, func() error {
		p := e.Doc.Defs[parent]
		for i := range p.Instances {
			if p.Instances[i].ID == inst {
				p.Instances[i].Name = name
			}
		}
		return nil
	})
	e.sel = s
}

// PlaceDefinition starts a paste preview of one new instance of id.
func (e *Editor) PlaceDefinition(id doc.DefID) {
	d := e.Doc.Defs[id]
	if d == nil || id == e.Doc.Root {
		return
	}
	e.setMode(EditMode)
	e.clip = &clip{w: d.W, h: d.H, pixels: bitmap.New(),
		insts: []doc.Instance{{Def: id}}} // unnamed: named after the component
	e.paste = true
	e.Status = "place " + d.Name + ": click to place, m/r to mirror/rotate, esc to cancel"
}

// DeleteDefinition removes a definition that has no instances.
func (e *Editor) DeleteDefinition(id doc.DefID) {
	for _, info := range e.Definitions() {
		if info.ID != id {
			continue
		}
		if info.Instances > 0 {
			e.Status = fmt.Sprintf("%s is used %d time(s); explode or delete its instances first", info.Name, info.Instances)
			return
		}
		e.change("delete component", []doc.DefID{id}, func() error {
			delete(e.Doc.Defs, id)
			return nil
		})
	}
}

// Resize handles: one cell outside the middle of each edge of a selected
// instance. Dragging one resizes the instance's definition.
const (
	edgeRight = iota
	edgeLeft
	edgeBottom
	edgeTop
)

func handles(r image.Rectangle) [4]image.Point {
	mx, my := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	return [4]image.Point{{r.Max.X, my}, {r.Min.X - 1, my}, {mx, r.Max.Y}, {mx, r.Min.Y - 1}}
}

// handleAt returns the edge whose handle is at world point p, or -1.
func (e *Editor) handleAt(p image.Point) int {
	if _, ok := e.selectedInstance(); !ok {
		return -1
	}
	for i, h := range handles(e.selWorld()) {
		if h == p {
			return i
		}
	}
	return -1
}

// resizedRect is the world rectangle of the selected instance with the
// dragged edge moved to the pointer.
func (d *dragState) resizedRect(r image.Rectangle) image.Rectangle {
	switch d.edge {
	case edgeRight:
		r.Max.X = max(d.cur.X, r.Min.X+1)
	case edgeLeft:
		r.Min.X = min(d.cur.X+1, r.Max.X-1)
	case edgeBottom:
		r.Max.Y = max(d.cur.Y, r.Min.Y+1)
	case edgeTop:
		r.Min.Y = min(d.cur.Y+1, r.Max.Y-1)
	}
	return r
}

// resizeSelected changes the selected instance's definition so its placed
// rectangle becomes r (world), keeping every instance's content fixed in the
// world. It is rejected if content would be cut off or instances overlap.
func (e *Editor) resizeSelected(r image.Rectangle) {
	inst, ok := e.selectedInstance()
	if !ok {
		return
	}
	s := e.sel
	def := e.Doc.Defs[inst.Def]
	// The new rectangle in the definition's own (unrotated) frame.
	toWorld := e.Doc.Placement(inst).Then(s.toW)
	lr := toWorld.Inverse().ApplyRect(r)
	if lr == def.Rect() {
		return
	}
	// Every definition placing this one changes too.
	ids := []doc.DefID{inst.Def}
	for _, id := range e.Doc.DefIDs() {
		for _, in := range e.Doc.Defs[id].Instances {
			if in.Def == inst.Def && !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	ok = e.change("resize component", ids, func() error {
		d := e.Doc.Defs[inst.Def]
		if b := d.Pixels.Bounds(); !b.Empty() && !b.In(lr) {
			return fmt.Errorf("it would cut off pixels")
		}
		for _, l := range d.Labels {
			if !l.Pos().In(lr) {
				return fmt.Errorf("it would cut off label %s", l.Name)
			}
		}
		for _, c := range d.Instances {
			if !e.Doc.PlacedRect(c).In(lr) {
				return fmt.Errorf("it would cut off a %s component", e.Doc.Defs[c.Def].Name)
			}
		}
		shift := image.Pt(-lr.Min.X, -lr.Min.Y)
		oldW, oldH := d.W, d.H
		px := d.Pixels
		d.Pixels = px.Crop(lr)
		moved := bitmap.New()
		d.Pixels.ForEach(func(x, y int) { moved.Set(x+shift.X, y+shift.Y, true) })
		d.Pixels = moved
		for i := range d.Labels {
			d.Labels[i].X += shift.X
			d.Labels[i].Y += shift.Y
		}
		for i := range d.Instances {
			d.Instances[i].X += shift.X
			d.Instances[i].Y += shift.Y
		}
		d.W, d.H = lr.Dx(), lr.Dy()
		// Keep content fixed: local q (old) is q+shift (new).
		for _, id := range ids[1:] {
			p := e.Doc.Defs[id]
			for i, in := range p.Instances {
				if in.Def != inst.Def {
					continue
				}
				old := in.Orient.Affine(oldW, oldH).Then(doc.Translate(in.X, in.Y))
				w0 := old.Apply(image.Point{})
				a := in.Orient.Affine(d.W, d.H).Apply(shift)
				p.Instances[i].X, p.Instances[i].Y = w0.X-a.X, w0.Y-a.Y
			}
		}
		return nil
	})
	if ok {
		inst2, _ := e.findInstance(s.def, inst.ID)
		s.rect = e.Doc.PlacedRect(inst2)
	}
	e.sel = s
}

func (e *Editor) findInstance(parent doc.DefID, id doc.InstID) (doc.Instance, bool) {
	for _, in := range e.Doc.Defs[parent].Instances {
		if in.ID == id {
			return in, true
		}
	}
	return doc.Instance{}, false
}

// InstanceCue describes an instance rectangle for drawing outlines.
type InstanceCue struct {
	Rect    image.Rectangle // world
	Path    string          // e.g. "alu.add3"
	Def     doc.DefID
	Name    string // the definition's display name
	Hovered bool   // under the pointer (deepest)
	Sibling bool   // another instance of the definition under the pointer
}

// Instances describes every instance for the renderer: outlines, the one
// under the pointer, and the other instances sharing its definition, so a
// shared edit is visible.
func (e *Editor) Instances() []InstanceCue {
	flat := e.Flat()
	loc := e.Doc.Locate(e.hover)
	// The hovered instance's path, from the indices Locate walked through.
	var names []string
	def := e.Doc.RootDef()
	for _, i := range loc.Path {
		names = append(names, e.Doc.InstanceName(def, i))
		def = e.Doc.Defs[def.Instances[i].Def]
	}
	hoverPath := strings.Join(names, ".")
	var cues []InstanceCue
	for _, fi := range flat.Instances {
		c := InstanceCue{Rect: fi.Rect, Path: fi.Path, Def: fi.Def, Name: e.Doc.Defs[fi.Def].Name}
		if loc.Def != e.Doc.Root && fi.Def == loc.Def {
			if fi.Path == hoverPath {
				c.Hovered = true
			} else {
				c.Sibling = true
			}
		}
		cues = append(cues, c)
	}
	return cues
}
