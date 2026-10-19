package ui

import (
	"image"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
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
// A label being retyped is left out: the overlay shows the text typed so far.
func (a *app) drawLabels(dst *ebiten.Image, o editor.Overlay) {
	if a.view.Scale < 6 {
		return
	}
	bounds := dst.Bounds()
	n := 0
	for _, l := range a.ed.Flat().Labels {
		x, y := a.view.ToScreen(l.Pos)
		if !image.Pt(int(x), int(y)).In(bounds) || o.Typing && l.Pos == o.Label {
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
	w, h := a.textWidth(s, 0), a.lineHeight()
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
	vector.FillRect(dst, float32(px-2*a.scale), float32(py-1*a.scale), float32(w+4*a.scale), float32(h+2*a.scale), labelBg, false)
	a.drawText(dst, s, px, py, 0, labelFg)
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
	instColor   = color.RGBA{208, 161, 208, 255} // v1's drain-source purple
	cueColor    = color.RGBA{128, 0, 128, 255}   // v1's transistor purple
	handleColor = color.RGBA{255, 140, 0, 255}   // resize handles: orange, used nowhere else
	tagColor    = color.RGBA{150, 105, 150, 255} // instance names written in their box
	minOutline  = 12                             // screen pixels: smaller instances are not outlined
	minTag      = 90                             // screen pixels: wider instances show their name
	maxOutline  = 4000                           // plain outlines drawn per frame, at most
)

// drawInstances outlines component instances. The instance under the
// pointer is labelled, and every other instance of the same definition is
// highlighted, so an edit that changes them all is visible.
//
// Instances big enough on screen are outlined, and the biggest show their
// instance name in their top-left corner, so a large design reads as named
// boxes at any zoom. In simulate mode only the names are drawn.
func (a *app) drawInstances(dst *ebiten.Image) {
	cues := a.ed.Instances()
	simulating := a.ed.Mode() == editor.SimulateMode
	b := dst.Bounds()
	view := image.Rectangle{Min: a.view.ToWorld(float64(b.Min.X), float64(b.Min.Y)), Max: a.view.ToWorld(float64(b.Max.X), float64(b.Max.Y)).Add(image.Pt(1, 1))}
	drawn := 0
	for _, c := range cues {
		if !c.Rect.Overlaps(view) {
			continue
		}
		sw, sh := float64(c.Rect.Dx())*a.view.Scale, float64(c.Rect.Dy())*a.view.Scale
		switch {
		case simulating:
		case c.Hovered || c.Sibling:
			strokeWorldRect(dst, &a.view, c.Rect, float32(2*a.scale), cueColor)
		case sw >= float64(minOutline)*a.scale && sh >= float64(minOutline)*a.scale && drawn < maxOutline:
			strokeWorldRect(dst, &a.view, c.Rect, 1, instColor)
			drawn++
		}
		if sw >= float64(minTag)*a.scale && sh >= 2*a.lineHeight() && !(c.Hovered && !simulating) {
			name := c.Path[strings.LastIndexByte(c.Path, '.')+1:]
			x, y := a.view.ToScreen(c.Rect.Min)
			a.drawText(dst, a.fitText(name, sw-6*a.scale), x+3*a.scale, y+2*a.scale, 0, tagColor)
		}
	}
	if simulating {
		return
	}
	for _, c := range cues {
		if !c.Hovered {
			continue
		}
		x, y := a.view.ToScreen(c.Rect.Min)
		s := c.Path + " : " + c.Name
		w, h := a.textWidth(s, 0), a.lineHeight()
		vector.FillRect(dst, float32(x), float32(y-h-4*a.scale), float32(w+6*a.scale), float32(h+4*a.scale), cueColor, false)
		a.drawText(dst, s, x+3*a.scale, y-h-2*a.scale, 0, color.White)
	}
}

// drawHandles draws resize handles as filled squares.
func (a *app) drawHandles(dst *ebiten.Image, hs []image.Point) {
	s := max(a.view.Scale, 6*a.scale)
	for _, h := range hs {
		x, y := a.view.ToScreen(h)
		cx, cy := x+a.view.Scale/2, y+a.view.Scale/2
		vector.FillRect(dst, float32(cx-s/2), float32(cy-s/2), float32(s), float32(s), handleColor, false)
	}
}
