package ui

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/Castux/mipsim2/editor"
	"github.com/Castux/mipsim2/netlist"
	"github.com/Castux/mipsim2/sim"
)

// The right panel has tabs: Watch (simulate mode) lists every labelled net
// and bus with its value; Diagnostics lists compile errors and warnings.
// Later milestones add Components (M7) and Memory (M8).

type panelTab int

const (
	tabDiagnostics panelTab = iota
	tabWatch
)

type tabInfo struct {
	id   panelTab
	text string
}

func (a *app) panelTabs() []tabInfo {
	var ts []tabInfo
	if a.ed.Mode() == editor.SimulateMode {
		ts = append(ts, tabInfo{tabWatch, "Watch"})
	}
	if n := len(a.ed.Netlist().Diagnostics); n > 0 {
		ts = append(ts, tabInfo{tabDiagnostics, fmt.Sprintf("Diagnostics (%d)", n)})
	}
	return ts
}

func (a *app) panelVisible() bool { return len(a.panelTabs()) > 0 }

// currentTab returns the selected tab, falling back to the first available.
func (a *app) currentTab() panelTab {
	ts := a.panelTabs()
	for _, t := range ts {
		if t.id == a.tab {
			return a.tab
		}
	}
	if len(ts) > 0 {
		return ts[0].id
	}
	return tabDiagnostics
}

func (a *app) panelBody(l layout) image.Rectangle {
	return image.Rect(l.right.Min.X, l.right.Min.Y+a.u(38), l.right.Max.X, l.right.Max.Y)
}

func (a *app) rowHeight() float64 {
	if a.currentTab() == tabWatch {
		return 22 * a.scale
	}
	return 34 * a.scale
}

// rowAt returns the index of the panel row under p.
func (a *app) rowAt(l layout, p image.Point) int {
	body := a.panelBody(l)
	return a.panelScroll + int(float64(p.Y-body.Min.Y)/a.rowHeight())
}

func (a *app) panelClick(l layout, p image.Point, mb ebiten.MouseButton) {
	if !p.In(a.panelBody(l)) {
		return
	}
	i := a.rowAt(l, p)
	switch a.currentTab() {
	case tabDiagnostics:
		diags := a.ed.Netlist().Diagnostics
		if i < 0 || i >= len(diags) || mb != ebiten.MouseButtonLeft {
			return
		}
		a.view.CenterOn(diags[i].Pos, l.canvas)
		a.ed.Status = diags[i].String()
	case tabWatch:
		r := a.ed.Runner()
		if r == nil {
			return
		}
		names := r.Names()
		if i < 0 || i >= len(names) {
			return
		}
		name := names[i]
		if _, isNet := r.Netlist().Lookup(name); isNet {
			// Same buttons as on the canvas.
			v := map[ebiten.MouseButton]string{ebiten.MouseButtonLeft: "high", ebiten.MouseButtonRight: "low", ebiten.MouseButtonMiddle: "float"}[mb]
			r.Set(name, v)
			r.Settle()
			return
		}
		if mb == ebiten.MouseButtonMiddle {
			r.Set(name, "float")
			r.Settle()
			return
		}
		a.prompt = &prompt{kind: uiSetWatch, target: name, buffer: r.Format(name)}
	}
}

func (a *app) drawPanel(screen *ebiten.Image, l layout) {
	if l.right.Empty() {
		return
	}
	panel := screen.SubImage(l.right).(*ebiten.Image)
	panel.Fill(color.RGBA{250, 250, 250, 255})
	fillRect(screen, image.Rect(l.right.Min.X, l.right.Min.Y, l.right.Min.X+1, l.right.Max.Y), chromeLine)
	cur := a.currentTab()
	for _, t := range l.tabs {
		bg := chromeBg
		if t.id == cur {
			bg = chromeActive
		}
		fillRect(screen, t.rect, bg)
		a.drawText(screen, t.text, float64(t.rect.Min.X)+8*a.scale, float64(t.rect.Min.Y)+5*a.scale, 13, chromeText)
	}

	body := a.panelBody(l)
	bodyImg := screen.SubImage(body).(*ebiten.Image)
	x := float64(body.Min.X) + 10*a.scale
	y := float64(body.Min.Y)
	switch cur {
	case tabDiagnostics:
		diags := a.ed.Netlist().Diagnostics
		a.panelScroll = max(0, min(a.panelScroll, len(diags)-1))
		for _, d := range diags[a.panelScroll:] {
			if y > float64(body.Max.Y) {
				break
			}
			clr := warnColor
			if d.Level == netlist.Error {
				clr = errColor
			}
			a.drawText(bodyImg, fmt.Sprintf("%s at %d,%d", d.Code, d.Pos.X, d.Pos.Y), x, y, 12, clr)
			a.drawText(bodyImg, truncate(d.Msg, 44), x+8*a.scale, y+15*a.scale, 12, chromeDim)
			y += a.rowHeight()
		}
	case tabWatch:
		r := a.ed.Runner()
		if r == nil {
			return
		}
		names := r.Names()
		a.panelScroll = max(0, min(a.panelScroll, len(names)-1))
		if len(names) == 0 {
			a.drawText(bodyImg, "no labelled nets: label wires with n", x, y, 12, chromeDim)
		}
		valueX := float64(body.Max.X) - 90*a.scale
		for _, name := range names[a.panelScroll:] {
			if y > float64(body.Max.Y) {
				break
			}
			a.drawText(bodyImg, truncate(name, 24), x, y+3*a.scale, 13, chromeText)
			a.drawText(bodyImg, r.Format(name), valueX, y+3*a.scale, 13, a.valueColor(name))
			if pin := a.pinOf(name); pin != sim.Floating {
				clr := color.RGBA{255, 125, 125, 255} // v1 pinned high
				if pin == sim.Low {
					clr = color.RGBA{109, 109, 255, 255} // v1 pinned low
				} else if pin == sim.Unstable {
					clr = chromeDim // some bits of a bus
				}
				drawIcon(bodyImg, "pin", float64(body.Max.X)-26*a.scale, y+3*a.scale, float64(max(1, int(a.scale*1.5+0.5))), clr)
			}
			y += a.rowHeight()
		}
	}
}

// valueColor shows a net's level in v1's colours (darkened for text).
func (a *app) valueColor(name string) color.Color {
	r := a.ed.Runner()
	id, ok := r.Netlist().Lookup(name)
	if !ok {
		return chromeText
	}
	switch r.Sim().Value(id) {
	case sim.High:
		return color.RGBA{214, 64, 110, 255}
	case sim.Low:
		return color.RGBA{40, 110, 170, 255}
	case sim.Unstable:
		return color.RGBA{165, 42, 42, 255}
	}
	return chromeDim
}

// pinOf returns a net's pin, or for a bus Unstable (shown as a grey marker)
// if any of its bits is pinned.
func (a *app) pinOf(name string) sim.Value {
	r := a.ed.Runner()
	if id, ok := r.Netlist().Lookup(name); ok {
		return r.Sim().Pinned(id)
	}
	if bits, ok := r.Bus(name); ok {
		for _, id := range bits {
			if id != netlist.NoNet && r.Sim().Pinned(id) != sim.Floating {
				return sim.Unstable
			}
		}
	}
	return sim.Floating
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
