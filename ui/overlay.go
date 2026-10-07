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
	selColor   = color.RGBA{90, 200, 255, 255}
	ghostColor = color.RGBA{230, 230, 255, 150}
	rectColor  = color.RGBA{255, 210, 90, 255}
	errColor   = color.RGBA{255, 70, 70, 255}
	warnColor  = color.RGBA{255, 190, 60, 255}
	labelBg    = color.RGBA{20, 20, 26, 200}
	labelFg    = color.RGBA{220, 220, 235, 255}
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
	px := x + a.view.Scale + 2*a.scale
	py := y + a.view.Scale/2 - h/2
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
