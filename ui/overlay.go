package ui

import (
	"image"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/Castux/mipsim2/editor"
	"github.com/Castux/mipsim2/netlist"
)

var (
	selColor   = color.RGBA{169, 169, 169, 255} // v1 selectRect: darkgrey
	ghostColor = color.RGBA{96, 96, 96, 140}
	rectColor  = color.RGBA{128, 0, 128, 255}
	errColor   = color.RGBA{220, 0, 0, 255}
	warnColor  = color.RGBA{230, 140, 0, 255}
	labelBg    = color.RGBA{255, 255, 255, 210}
	labelFg    = color.RGBA{0, 0, 0, 255}
)

func screenRect(v *editor.View, r image.Rectangle) (x, y, w, h float32) {
	x0, y0 := v.ToScreen(r.Min)
	x1, y1 := v.ToScreen(r.Max)
	return float32(x0), float32(y0), float32(x1 - x0), float32(y1 - y0)
}

func strokeWorldRect(dst *ebiten.Image, v *editor.View, r image.Rectangle, width float32, clr color.Color) {
	if r.Empty() {
		return
	}
	x, y, w, h := screenRect(v, r)
	vector.StrokeRect(dst, x, y, w, h, width, clr, false)
}

// drawOverlay draws the editor's selection, previews and label entry.
func (a *app) drawOverlay(dst *ebiten.Image, o editor.Overlay) {
	v := &a.view
	lw := float32(2 * a.scale)
	s := float32(max(v.Scale, 1))
	for _, p := range o.Ghost {
		x, y := v.ToScreen(p)
		vector.FillRect(dst, float32(x), float32(y), s, s, ghostColor, false)
	}
	for _, r := range o.GhostRects {
		strokeWorldRect(dst, v, r, lw, rectColor)
	}
	strokeWorldRect(dst, v, o.Selection, lw, selColor)
	a.drawHandles(dst, o.Handles)
	if o.Typing {
		strokeWorldRect(dst, v, image.Rectangle{Min: o.Label, Max: o.Label.Add(image.Pt(1, 1))}, lw, selColor)
		buf, _ := a.ed.Typing()
		a.drawLabel(dst, o.Label, buf+"_")
	}
}

// drawLabels writes label names next to their pixels when zoomed in enough.
func (a *app) drawLabels(dst *ebiten.Image) {
	if a.view.Scale < 6 {
		return
	}
	bounds := dst.Bounds()
	n := 0
	for _, l := range a.ed.Flat().Labels {
		x, y := a.view.ToScreen(l.Pos)
		if !image.Pt(int(x), int(y)).In(bounds) {
			continue
		}
		if n++; n > 400 {
			return
		}
		name := l.Name
		if i := strings.LastIndexByte(name, '.'); i >= 0 {
			name = name[i+1:] // the full name is in the hover text
		}
		a.drawLabel(dst, l.Pos, name)
	}
}

func (a *app) drawLabel(dst *ebiten.Image, p image.Point, s string) {
	x, y := a.view.ToScreen(p)
	a.face.Size = 12 * a.scale
	w, h := text.Measure(s, a.face, 0)
	sc := a.view.Scale
	gap := 2 * a.scale
	// Put the label on a side whose neighbouring pixel is off, so it covers
	// as little of the circuit as possible: right, left, above, below.
	px, py := x+sc+gap, y+sc/2-h/2
	if on := a.ed.Flat().Pixels; on.Get(p.X+1, p.Y) {
		switch {
		case !on.Get(p.X-1, p.Y):
			px = x - w - gap
		case !on.Get(p.X, p.Y-1):
			px, py = x+sc/2-w/2, y-h-gap
		case !on.Get(p.X, p.Y+1):
			px, py = x+sc/2-w/2, y+sc+gap
		}
	}
	vector.FillRect(dst, float32(px-2*a.scale), float32(py), float32(w+4*a.scale), float32(h), labelBg, false)
	op := &text.DrawOptions{}
	op.GeoM.Translate(px, py)
	op.ColorScale.ScaleWithColor(labelFg)
	text.Draw(dst, s, a.face, op)
}

// drawDiagnosticMarkers outlines every diagnostic's pixel on the canvas.
func (a *app) drawDiagnosticMarkers(dst *ebiten.Image, nl *netlist.Netlist) {
	size := max(a.view.Scale, 8*a.scale)
	for _, d := range nl.Diagnostics {
		clr := warnColor
		if d.Level == netlist.Error {
			clr = errColor
		}
		x, y := a.view.ToScreen(d.Pos)
		cx, cy := x+a.view.Scale/2, y+a.view.Scale/2
		vector.StrokeRect(dst, float32(cx-size/2-2), float32(cy-size/2-2), float32(size+4), float32(size+4), float32(1.5*a.scale), clr, false)
	}
}

var (
	instColor  = color.RGBA{208, 161, 208, 255} // v1's drain-source purple
	cueColor   = color.RGBA{128, 0, 128, 255}   // v1's transistor purple
	maxOutline = 3000                           // skip plain outlines beyond this many
)

// drawInstances outlines component instances. The instance under the
// pointer is labelled, and every other instance of the same definition is
// highlighted, so an edit that changes them all is visible.
func (a *app) drawInstances(dst *ebiten.Image) {
	cues := a.ed.Instances()
	if a.ed.Mode() == editor.SimulateMode {
		return
	}
	plain := len(cues) <= maxOutline && a.view.Scale >= 0.5
	for _, c := range cues {
		switch {
		case c.Hovered || c.Sibling:
			strokeWorldRect(dst, &a.view, c.Rect, float32(2*a.scale), cueColor)
		case plain:
			strokeWorldRect(dst, &a.view, c.Rect, 1, instColor)
		}
	}
	for _, c := range cues {
		if !c.Hovered {
			continue
		}
		x, y := a.view.ToScreen(c.Rect.Min)
		s := c.Path + " : " + c.Name
		a.face.Size = 12 * a.scale
		w, h := text.Measure(s, a.face, 0)
		vector.FillRect(dst, float32(x), float32(y-h-2*a.scale), float32(w+6*a.scale), float32(h+2*a.scale), cueColor, false)
		op := &text.DrawOptions{}
		op.GeoM.Translate(x+3*a.scale, y-h-1*a.scale)
		op.ColorScale.ScaleWithColor(color.White)
		text.Draw(dst, s, a.face, op)
	}
}

// drawHandles draws resize handles as filled squares.
func (a *app) drawHandles(dst *ebiten.Image, hs []image.Point) {
	s := max(a.view.Scale, 6*a.scale)
	for _, h := range hs {
		x, y := a.view.ToScreen(h)
		cx, cy := x+a.view.Scale/2, y+a.view.Scale/2
		vector.FillRect(dst, float32(cx-s/2), float32(cy-s/2), float32(s), float32(s), cueColor, false)
	}
}
