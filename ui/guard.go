package ui

import (
	"image"
	"image/color"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/ui/filebrowser"
)

// The unsaved-changes guard: before an action that replaces the document
// (Open, New), a modified document asks to save, discard or cancel. Saving
// may go through the file dialog first; the action runs only once the save
// has succeeded.

// guard is the open question, with the action waiting on it.
type guard struct {
	action string // what will happen, for the message: "opening", "a new document"
	then   func()
}

// guarded runs then at once if the document is saved, else asks first.
func (a *app) guarded(action string, then func()) {
	if !a.ed.Modified() {
		then()
		return
	}
	a.guard = &guard{action: action, then: then}
}

// guardSave saves (through the dialog if untitled) and continues on success.
func (a *app) guardSave() {
	g := a.guard
	a.guard = nil
	a.afterSave = g.then
	if a.opts.Path == "" {
		a.openBrowser(filebrowser.Save)
	} else {
		a.save(a.opts.Path)
	}
}

func (a *app) guardDiscard() {
	g := a.guard
	a.guard = nil
	g.then()
}

func (a *app) guardCancel() {
	a.guard = nil
	a.ed.Status = "cancelled"
}

// newDocument replaces the document with an empty untitled one.
func (a *app) newDocument() {
	a.ed.ReplaceDocument(doc.New())
	a.setPath("")
	a.fit()
	a.ed.Status = "new document"
}

type guardLayout struct {
	box, save, discard, cancel image.Rectangle
}

func (a *app) guardLayout(l layout) guardLayout {
	var gl guardLayout
	lh := int(a.lineHeight())
	pad := a.u(10)
	btnH := lh + a.u(10)
	labels := []string{"Save  enter", "Discard  d", "Cancel  esc"}
	bw := 0
	for _, s := range labels {
		bw = max(bw, int(a.textWidth(s, 0))+a.u(20))
	}
	l1, l2 := a.guardMessage()
	w := max(3*bw+2*a.u(6), int(a.textWidth(l1, 0)), int(a.textWidth(l2, 0))) + 2*pad
	c := l.canvas
	w = min(w, c.Dx()-a.u(20))
	h := pad + 2*lh + pad + btnH + pad
	x0, y0 := c.Min.X+(c.Dx()-w)/2, c.Min.Y+(c.Dy()-h)/2
	gl.box = image.Rect(x0, y0, x0+w, y0+h)
	by := y0 + h - pad - btnH
	x := x0 + w - pad
	for _, r := range []*image.Rectangle{&gl.cancel, &gl.discard, &gl.save} {
		*r = image.Rect(x-bw, by, x, by+btnH)
		x -= bw + a.u(6)
	}
	return gl
}

// guardMessage is the question, on two lines.
func (a *app) guardMessage() (string, string) {
	name := "untitled"
	if a.opts.Path != "" {
		name = filepath.Base(a.opts.Path)
	}
	return name + " has unsaved changes.", "Save before " + a.guard.action + "?"
}

// handleGuard takes input while the question is shown.
func (a *app) handleGuard(l layout) {
	pressed := inpututil.IsKeyJustPressed
	switch {
	case pressed(ebiten.KeyEnter) || pressed(ebiten.KeyNumpadEnter) || pressed(ebiten.KeyS):
		a.guardSave()
		return
	case pressed(ebiten.KeyD):
		a.guardDiscard()
		return
	case pressed(ebiten.KeyEscape):
		a.guardCancel()
		return
	}
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	cx, cy := ebiten.CursorPosition()
	p := image.Pt(cx, cy)
	gl := a.guardLayout(l)
	switch {
	case p.In(gl.save):
		a.guardSave()
	case p.In(gl.discard):
		a.guardDiscard()
	case p.In(gl.cancel), !p.In(gl.box):
		a.guardCancel()
	}
}

func (a *app) drawGuard(screen *ebiten.Image, l layout) {
	gl := a.guardLayout(l)
	fillRect(screen, l.canvas, color.NRGBA{255, 255, 255, 170}) // dim the canvas (RGBA is premultiplied)
	a.box(screen, gl.box, chromeBg)
	pad := float64(a.u(10))
	lh := a.lineHeight()
	l1, l2 := a.guardMessage()
	tw := float64(gl.box.Dx()) - 2*pad
	a.drawText(screen, a.fitText(l1, tw), float64(gl.box.Min.X)+pad, float64(gl.box.Min.Y)+pad, 0, chromeText)
	a.drawText(screen, a.fitText(l2, tw), float64(gl.box.Min.X)+pad, float64(gl.box.Min.Y)+pad+lh, 0, chromeText)
	cx, cy := ebiten.CursorPosition()
	cur := image.Pt(cx, cy)
	for _, btn := range []struct {
		r          image.Rectangle
		label, key string
	}{{gl.save, "Save", "enter"}, {gl.discard, "Discard", "d"}, {gl.cancel, "Cancel", "esc"}} {
		bg := chromeBg
		if cur.In(btn.r) {
			bg = chromeHover
		}
		a.box(screen, btn.r, bg)
		ty := float64(btn.r.Min.Y+btn.r.Max.Y)/2 - lh/2
		w := a.textWidth(btn.label+"  "+btn.key, 0)
		bx := float64(btn.r.Min.X+btn.r.Max.X)/2 - w/2
		bx += a.drawText(screen, btn.label+"  ", bx, ty, 0, chromeText)
		a.drawText(screen, btn.key, bx, ty, 0, chromeDim)
	}
}

func guardHint() string {
	return "unsaved changes · enter or s: save first · d: discard them · esc: cancel"
}
