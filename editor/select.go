package editor

import (
	"fmt"
	"image"
	"slices"
	"strings"

	"github.com/Castux/mipsim2/doc"
)

func canonRect(a, b image.Point) image.Rectangle {
	return image.Rect(min(a.X, b.X), min(a.Y, b.Y), max(a.X, b.X)+1, max(a.Y, b.Y)+1)
}

// selWorld returns the selection rectangle in world coordinates.
func (e *Editor) selWorld() image.Rectangle {
	if e.sel == nil {
		return image.Rectangle{}
	}
	return e.sel.toW.ApplyRect(e.sel.rect)
}

// Selected returns the selection rectangle in world coordinates, or an
// empty rectangle.
func (e *Editor) Selected() image.Rectangle { return e.selWorld() }

// selectArea selects the rectangle from a to b in the definition under a.
// A click without a drag selects the top-level instance under it, if any.
func (e *Editor) selectArea(a, b image.Point) {
	if a == b {
		e.clickSelect(a)
		return
	}
	loc := e.Doc.Locate(a)
	r := canonRect(a, b)
	def := e.Doc.Defs[loc.Def]
	if loc.Def != e.Doc.Root {
		r = r.Intersect(loc.Map.ApplyRect(def.Rect()))
	}
	local := loc.Map.Inverse().ApplyRect(r)
	if local.Empty() {
		e.sel = nil
		return
	}
	e.sel = &selection{path: loc.Path, def: loc.Def, toW: loc.Map, rect: local}
	where := "top level"
	if loc.Def != e.Doc.Root {
		where = "definition " + string(loc.Def)
	}
	e.Status = fmt.Sprintf("selected %dx%d in %s", local.Dx(), local.Dy(), where)
}

// clickSelect selects the deepest instance under p, in its parent's
// definition, so moving it rearranges the component that contains it.
// Clicking again inside the selected instance selects its parent instance,
// one level up, wrapping back to the deepest after the top level.
func (e *Editor) clickSelect(p image.Point) {
	path := e.Doc.Locate(p).Path
	if len(path) == 0 {
		e.sel = nil
		e.Status = ""
		return
	}
	depth := len(path) // 1-based depth of the instance to select
	if e.sel != nil && p.In(e.selWorld()) {
		if _, ok := e.selectedInstance(); ok && slices.Equal(e.sel.path, path[:len(e.sel.path)]) {
			if d := len(e.sel.path) + 1; d > 1 {
				depth = d - 1
			}
		}
	}

	// Walk down to the parent of the instance at that depth.
	def, toW := e.Doc.RootDef(), doc.IdentityAffine
	var names []string
	for _, i := range path[:depth-1] {
		inst := def.Instances[i]
		toW = e.Doc.Placement(inst).Then(toW)
		names = append(names, inst.Name)
		def = e.Doc.Defs[inst.Def]
	}
	inst := def.Instances[path[depth-1]]
	e.sel = &selection{path: slices.Clone(path[:depth-1]), def: def.ID, toW: toW, rect: e.Doc.PlacedRect(inst)}
	names = append(names, inst.Name)
	where := ""
	if depth > 1 {
		where = fmt.Sprintf(" inside %s (moving it edits %s everywhere)", e.Doc.Defs[def.ID].Name, e.Doc.Defs[def.ID].Name)
	}
	e.Status = fmt.Sprintf("selected %s (%s)%s; click again for the outer component", strings.Join(names, "."), e.Doc.Defs[inst.Def].Name, where)
}

// selectionClip extracts the selected content.
func (e *Editor) selectionClip() *clip {
	return extract(e.Doc, e.Doc.Defs[e.sel.def], e.sel.rect)
}

func (e *Editor) copySelection() bool {
	if e.sel == nil {
		e.Status = "nothing selected"
		return false
	}
	e.clip = e.selectionClip()
	e.Status = fmt.Sprintf("copied %dx%d: %d pixels, %d labels, %d instances",
		e.clip.w, e.clip.h, e.clip.pixels.Count(), len(e.clip.labels), len(e.clip.insts))
	return true
}

func (e *Editor) deleteSelection(name string) {
	if e.sel == nil {
		e.Status = "nothing selected"
		return
	}
	s := e.sel
	e.change(name, []doc.DefID{s.def}, func() error {
		clearRect(e.Doc, e.Doc.Defs[s.def], s.rect, true)
		return nil
	})
	e.sel = s
}

// moveSelection moves the selected content by the drag, in the selection's
// own definition.
func (e *Editor) moveSelection(d *dragState) {
	s := e.sel
	inv := s.toW.Inverse()
	delta := inv.Apply(d.cur).Sub(inv.Apply(d.start))
	if delta == (image.Point{}) {
		e.selectArea(d.start, d.cur) // a click inside the selection
		return
	}
	ok := e.change("move", []doc.DefID{s.def}, func() error {
		def := e.Doc.Defs[s.def]
		c := extract(e.Doc, def, s.rect)
		clearRect(e.Doc, def, s.rect, true)
		insert(e.Doc, def, c, s.rect.Min.Add(delta))
		return nil
	})
	if ok {
		s.rect = s.rect.Add(delta)
	}
	e.sel = s
}

// transformSelection rotates or mirrors the paste preview, or else the
// selected content in place, keeping the top-left corner fixed.
func (e *Editor) transformSelection(o doc.Orient, name string) {
	if e.paste && e.clip != nil {
		e.clip = e.clip.transform(e.Doc, o)
		e.Status = name + " paste"
		return
	}
	if e.sel == nil {
		e.Status = "nothing selected"
		return
	}
	s := e.sel
	var size image.Point
	ok := e.change(name, []doc.DefID{s.def}, func() error {
		def := e.Doc.Defs[s.def]
		c := extract(e.Doc, def, s.rect).transform(e.Doc, o)
		clearRect(e.Doc, def, s.rect, true)
		insert(e.Doc, def, c, s.rect.Min)
		size = image.Pt(c.w, c.h)
		return nil
	})
	if ok {
		s.rect = image.Rectangle{Min: s.rect.Min, Max: s.rect.Min.Add(size)}
	}
	e.sel = s
}

func (e *Editor) startPaste() {
	if e.clip == nil || e.clip.empty() {
		e.Status = "clipboard is empty"
		return
	}
	e.paste = true
	e.Status = "paste: click to place, right click or escape to cancel, m/r to mirror/rotate"
}

func (e *Editor) placePaste(p image.Point) {
	loc := e.Doc.Locate(p)
	c := e.clip
	ok := e.change("paste", []doc.DefID{loc.Def}, func() error {
		insert(e.Doc, e.Doc.Defs[loc.Def], c, loc.Local)
		return nil
	})
	if ok {
		e.paste = false
		e.tool = Select
		e.sel = &selection{path: loc.Path, def: loc.Def, toW: loc.Map,
			rect: image.Rectangle{Min: loc.Local, Max: loc.Local.Add(image.Pt(c.w, c.h))}}
	}
}

// SetClipboardRows replaces the clipboard with ASCII art ('#' and '.' rows),
// so pixel art can be pasted from outside.
func (e *Editor) SetClipboardRows(text string) error {
	b, err := clipFromRows(text)
	if err != nil {
		return err
	}
	e.clip = b
	return nil
}

// ClipboardRows returns the clipboard's pixels as ASCII art.
func (e *Editor) ClipboardRows() string {
	if e.clip == nil {
		return ""
	}
	return strings.Join(e.clip.pixels.EncodeRows(image.Rect(0, 0, e.clip.w, e.clip.h)), "\n")
}

func clipFromRows(text string) (*clip, error) {
	rows := strings.Split(strings.ReplaceAll(strings.TrimSpace(text), "\r", ""), "\n")
	for i := range rows {
		rows[i] = strings.TrimSpace(rows[i])
	}
	c := &clip{}
	var err error
	if c.pixels, err = bitmapFromRows(rows); err != nil {
		return nil, err
	}
	c.h = len(rows)
	for _, r := range rows {
		c.w = max(c.w, len(r))
	}
	return c, nil
}

// Overlay is what the renderer draws on top of the circuit, in world
// coordinates.
type Overlay struct {
	Selection  image.Rectangle   // the selection, or the rectangle being dragged out
	Ghost      []image.Point     // pixels of a move or paste preview
	GhostRects []image.Rectangle // the preview's outline and its instances' rectangles
	Label      image.Point       // where a label is being typed
	Typing     bool
	Handles    []image.Point // resize handles of a selected instance
}

// Overlay describes selection, previews and label entry for the renderer.
func (e *Editor) Overlay() Overlay {
	var o Overlay
	if e.typing != nil && e.typing.kind == typeLabel {
		o.Typing = true
		o.Label = e.typing.world
	}
	switch {
	case e.drag != nil && e.drag.resizing:
		o.Selection = e.drag.resizedRect(e.selWorld())
	case e.drag != nil && !e.drag.moving:
		o.Selection = canonRect(e.drag.start, e.drag.cur)
	case e.drag != nil && e.drag.moving:
		s := e.sel
		inv := s.toW.Inverse()
		delta := inv.Apply(e.drag.cur).Sub(inv.Apply(e.drag.start))
		o.Selection = s.toW.ApplyRect(s.rect.Add(delta))
		e.ghost(&o, e.selectionClip(), s.toW, s.rect.Min.Add(delta))
	case e.paste && e.clip != nil:
		loc := e.Doc.Locate(e.hover)
		e.ghost(&o, e.clip, loc.Map, loc.Local)
	default:
		o.Selection = e.selWorld()
		if _, ok := e.selectedInstance(); ok && e.mode == EditMode && e.tool == Select {
			h := handles(o.Selection)
			o.Handles = h[:]
		}
	}
	return o
}

// ghost adds clip c placed at local point at, mapped to world by toW.
func (e *Editor) ghost(o *Overlay, c *clip, toW doc.Affine, at image.Point) {
	place := doc.Translate(at.X, at.Y).Then(toW)
	c.pixels.ForEach(func(x, y int) { o.Ghost = append(o.Ghost, place.Apply(image.Pt(x, y))) })
	o.GhostRects = append(o.GhostRects, place.ApplyRect(image.Rect(0, 0, c.w, c.h)))
	for _, inst := range c.insts {
		o.GhostRects = append(o.GhostRects, place.ApplyRect(e.Doc.PlacedRect(inst)))
	}
}

func (e *Editor) startLabel(p image.Point) {
	loc := e.Doc.Locate(p)
	def := e.Doc.Defs[loc.Def]
	t := &typing{def: loc.Def, at: loc.Local, world: p}
	for _, l := range def.Labels {
		if l.Pos() == loc.Local {
			t.old, t.buffer = l.Name, l.Name
		}
	}
	e.typing = t
	e.Status = "label: " + t.buffer + "_   (enter to apply, escape to cancel; empty removes)"
}

func (e *Editor) doTyping(a Action) {
	t := e.typing
	switch a {
	case ActEnter:
		e.commitTyping()
	case ActEscape:
		e.typing = nil
		e.Status = t.kind.String() + " cancelled"
	case ActBackspace:
		if r := []rune(t.buffer); len(r) > 0 {
			t.buffer = string(r[:len(r)-1])
		}
		e.Status = t.kind.String() + ": " + t.buffer + "_"
	}
}

func (e *Editor) commitTyping() {
	t := e.typing
	e.typing = nil
	name := strings.TrimSpace(t.buffer)
	if name == t.old {
		e.Status = ""
		return
	}
	switch t.kind {
	case typeDefName:
		e.renameDefinition(t.def, name)
		return
	case typeInstName:
		e.renameInstance(t.def, t.inst, name)
		return
	}
	if name != "" && !doc.ValidName(name) {
		e.Status = fmt.Sprintf("invalid label %q: use letters, digits and underscores", name)
		return
	}
	what := "label"
	if name == "" {
		what = "remove label"
	}
	e.change(what, []doc.DefID{t.def}, func() error {
		def := e.Doc.Defs[t.def]
		labels := def.Labels[:0]
		for _, l := range def.Labels {
			if l.Pos() != t.at {
				labels = append(labels, l)
			}
		}
		def.Labels = labels
		if name != "" {
			def.Labels = append(def.Labels, doc.Label{X: t.at.X, Y: t.at.Y, Name: name})
		}
		return nil
	})
}
