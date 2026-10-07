package editor

import (
	"image"

	"github.com/Castux/mipsim2/doc"
)

// Command is one undoable change to the document. Every command records the
// definitions it changes, so undo works across instances.
type Command interface {
	Do(d *doc.Document)
	Undo(d *doc.Document)
	Name() string
}

// pixelChange sets one pixel of one definition.
type pixelChange struct {
	def      doc.DefID
	p        image.Point // local coordinates
	old, new bool
}

// pixelEdit is a set of pixel changes made by one stroke. It is cheaper than
// a snapshot for large definitions.
type pixelEdit struct {
	changes []pixelChange
}

func (c *pixelEdit) Name() string { return "draw" }

func (c *pixelEdit) Do(d *doc.Document) {
	for _, ch := range c.changes {
		d.Defs[ch.def].Pixels.Set(ch.p.X, ch.p.Y, ch.new)
	}
}

func (c *pixelEdit) Undo(d *doc.Document) {
	for i := len(c.changes) - 1; i >= 0; i-- {
		ch := c.changes[i]
		d.Defs[ch.def].Pixels.Set(ch.p.X, ch.p.Y, ch.old)
	}
}

// defState is a definition's content before and after a command. A nil
// side means the definition did not exist.
type defState struct {
	id            doc.DefID
	before, after *doc.Definition
}

// snapshot is a command that replaces whole definitions.
type snapshot struct {
	name string
	defs []defState
}

func (c *snapshot) Name() string { return c.name }

func (c *snapshot) Do(d *doc.Document) {
	for _, s := range c.defs {
		set(d, s.id, s.after)
	}
}

func (c *snapshot) Undo(d *doc.Document) {
	for i := len(c.defs) - 1; i >= 0; i-- {
		set(d, c.defs[i].id, c.defs[i].before)
	}
}

func set(d *doc.Document, id doc.DefID, def *doc.Definition) {
	if def == nil {
		delete(d.Defs, id)
		return
	}
	d.Defs[id] = def.Clone()
}

// change runs fn on clones of the given definitions, validates the document,
// and either records the result as one undoable command or reverts it and
// reports why. It returns whether the change was kept.
func (e *Editor) change(name string, ids []doc.DefID, fn func() error) bool {
	c := &snapshot{name: name}
	for _, id := range ids {
		var before *doc.Definition
		if def := e.Doc.Defs[id]; def != nil {
			before = def.Clone()
		}
		c.defs = append(c.defs, defState{id: id, before: before})
	}
	err := fn()
	if err == nil {
		err = e.Doc.Validate()
	}
	if err != nil {
		c.Undo(e.Doc)
		e.Status = name + " rejected: " + firstLine(err.Error())
		return false
	}
	for i := range c.defs {
		if def := e.Doc.Defs[c.defs[i].id]; def != nil {
			c.defs[i].after = def.Clone()
		}
	}
	e.undo = append(e.undo, c)
	e.redo = nil
	e.changed(name)
	return true
}

func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' {
			return s[:i]
		}
	}
	return s
}
