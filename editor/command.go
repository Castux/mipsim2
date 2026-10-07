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

// pixelEdit is a set of pixel changes made by one stroke.
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
