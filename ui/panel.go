package ui

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/Castux/mipsim2/netlist"
)

// The diagnostics panel lists compile errors and warnings on the right of
// the canvas while there are any. Clicking one centres the view on it.

func (a *app) panelWidth() int {
	if len(a.ed.Netlist().Diagnostics) == 0 {
		return 0
	}
	return int(380 * a.scale)
}

func (a *app) panelArea() image.Rectangle {
	return image.Rect(a.w-a.panelWidth(), 0, a.w, a.h-a.statusHeight())
}

func (a *app) panelLine() float64 { return 34 * a.scale }

// panelClick handles a click at screen position p inside the panel.
func (a *app) panelClick(p image.Point) {
	area := a.panelArea()
	i := a.panelScroll + int(float64(p.Y-area.Min.Y-int(28*a.scale))/a.panelLine())
	diags := a.ed.Netlist().Diagnostics
	if i < 0 || i >= len(diags) {
		return
	}
	c := a.canvasArea()
	a.view.CenterOn(diags[i].Pos, float64(c.Dx()), float64(c.Dy()))
	a.ed.Status = diags[i].String()
}

func (a *app) drawPanel(screen *ebiten.Image) {
	area := a.panelArea()
	if area.Empty() {
		return
	}
	panel := screen.SubImage(area).(*ebiten.Image)
	panel.Fill(color.RGBA{30, 30, 36, 255})
	diags := a.ed.Netlist().Diagnostics
	a.panelScroll = max(0, min(a.panelScroll, len(diags)-1))

	a.face.Size = 14 * a.scale
	x := float64(area.Min.X) + 10*a.scale
	op := &text.DrawOptions{}
	op.GeoM.Translate(x, float64(area.Min.Y)+6*a.scale)
	op.ColorScale.ScaleWithColor(labelFg)
	text.Draw(panel, fmt.Sprintf("Diagnostics (%d) — click to show", len(diags)), a.face, op)

	a.face.Size = 12 * a.scale
	y := float64(area.Min.Y) + 28*a.scale
	for _, d := range diags[a.panelScroll:] {
		if y > float64(area.Max.Y) {
			break
		}
		clr := warnColor
		if d.Level == netlist.Error {
			clr = errColor
		}
		head := &text.DrawOptions{}
		head.GeoM.Translate(x, y)
		head.ColorScale.ScaleWithColor(clr)
		text.Draw(panel, fmt.Sprintf("%s at %d,%d", d.Code, d.Pos.X, d.Pos.Y), a.face, head)
		msg := &text.DrawOptions{}
		msg.GeoM.Translate(x+8*a.scale, y+15*a.scale)
		msg.ColorScale.ScaleWithColor(color.RGBA{170, 170, 185, 255})
		text.Draw(panel, truncate(d.Msg, 52), a.face, msg)
		y += a.panelLine()
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
