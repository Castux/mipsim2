package ui

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/Castux/mipsim2/editor"
	"github.com/Castux/mipsim2/netlist"
	"github.com/Castux/mipsim2/sim"
)

// The right panel has tabs. Edit mode: Components (the palette) and Devices
// (memory configuration). Simulate mode: Watch (every labelled net and bus
// with its value) and Memory (contents of memory devices). Both: Diagnostics
// (compile errors and warnings), when there are any.

type panelTab int

const (
	tabDiagnostics panelTab = iota
	tabWatch
	tabComponents
	tabMemory
	tabDevices
)

type tabInfo struct {
	id   panelTab
	text string
}

func (a *app) panelTabs() []tabInfo {
	var ts []tabInfo
	if a.ed.Mode() == editor.SimulateMode {
		ts = append(ts, tabInfo{tabWatch, "Watch"})
		if len(a.memories()) > 0 {
			ts = append(ts, tabInfo{tabMemory, "Memory"})
		}
	} else {
		ts = append(ts, tabInfo{tabComponents, fmt.Sprintf("Components (%d)", len(a.ed.Definitions()))})
		ts = append(ts, tabInfo{tabDevices, fmt.Sprintf("Devices (%d)", len(a.ed.Doc.Devices))})
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
	return image.Rect(l.right.Min.X, l.right.Min.Y+int(a.lineHeight())+a.u(26), l.right.Max.X, l.right.Max.Y)
}

func (a *app) rowHeight() float64 {
	if t := a.currentTab(); t == tabWatch || t == tabComponents || t == tabMemory || t == tabDevices {
		return a.lineHeight() + 10*a.scale
	}
	return 2*a.lineHeight() + 14*a.scale
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
	case tabMemory:
		a.memoryClick(l, p)
	case tabDevices:
		a.devicesClick(l, p, mb)
	case tabDiagnostics:
		diags := a.ed.Netlist().Diagnostics
		if i < 0 || i >= len(diags) || mb != ebiten.MouseButtonLeft {
			return
		}
		a.view.CenterOn(diags[i].Pos, l.canvas)
		a.ed.Status = diags[i].String()
	case tabComponents:
		defs := a.ed.Definitions()
		if i == importRow(defs) {
			if mb == ebiten.MouseButtonLeft {
				a.startImport()
			}
			return
		}
		if i < 0 || i >= len(defs) {
			return
		}
		switch mb {
		case ebiten.MouseButtonLeft:
			a.ed.PlaceDefinition(defs[i].ID)
		case ebiten.MouseButtonRight:
			a.ed.StartRenameDefinition(defs[i].ID)
		case ebiten.MouseButtonMiddle:
			a.ed.DeleteDefinition(defs[i].ID)
		}
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
			a.ed.SetName(name, v)
			return
		}
		if mb == ebiten.MouseButtonMiddle {
			a.ed.SetName(name, "float")
			return
		}
		a.editWatch(name)
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
		a.box(screen, t.rect, bg)
		a.drawText(screen, t.text, float64(t.rect.Min.X)+8*a.scale, float64(t.rect.Min.Y+t.rect.Max.Y)/2-a.lineHeight()/2, 0, chromeText)
	}

	body := a.panelBody(l)
	bodyImg := screen.SubImage(body).(*ebiten.Image)
	x := float64(body.Min.X) + 10*a.scale
	y := float64(body.Min.Y)
	switch cur {
	case tabMemory:
		a.drawMemory(bodyImg, body)
	case tabDevices:
		a.drawDevices(bodyImg, body)
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
			a.drawText(bodyImg, fmt.Sprintf("%s at %d,%d", d.Code, d.Pos.X, d.Pos.Y), x, y, 0, clr)
			a.drawText(bodyImg, a.fitText(d.Msg, float64(body.Max.X)-x-18*a.scale), x+8*a.scale, y+a.lineHeight()+3*a.scale, 0, chromeDim)
			y += a.rowHeight()
		}
	case tabComponents:
		defs := a.ed.Definitions()
		a.panelScroll = max(0, min(a.panelScroll, len(defs)-1))
		if len(defs) == 0 {
			a.drawText(bodyImg, "none yet: select an area and press k", x, y, 0, chromeDim)
		}
		for _, d := range defs[a.panelScroll:] {
			if y > float64(body.Max.Y) {
				break
			}
			info := fmt.Sprintf("%dx%d  ×%d", d.W, d.H, d.Instances)
			a.drawText(bodyImg, a.fitText(d.Name, float64(body.Max.X)-x-a.textWidth(info, 0)-24*a.scale), x, y+5*a.scale, 0, chromeText)
			clr := chromeDim
			if d.Instances == 0 {
				clr = chromeOff
			}
			a.drawText(bodyImg, info, float64(body.Max.X)-a.textWidth(info, 12)-10*a.scale, y+5*a.scale, 0, clr)
			y += a.rowHeight()
		}
		if r := importRow(defs) - a.panelScroll; r >= 0 {
			by := float64(body.Min.Y) + float64(r)*a.rowHeight()
			btn := image.Rect(int(x), int(by+1*a.scale), body.Max.X-a.u(10), int(by+a.lineHeight()+9*a.scale))
			cx, cy := ebiten.CursorPosition()
			a.smallButton(bodyImg, btn, "Import components...", image.Pt(cx, cy))
		}
	case tabWatch:
		r := a.ed.Runner()
		if r == nil {
			return
		}
		names := r.Names()
		a.panelScroll = max(0, min(a.panelScroll, len(names)-1))
		if len(names) == 0 {
			a.drawText(bodyImg, "no labelled nets: label wires with n", x, y, 0, chromeDim)
		}
		valueX := float64(body.Max.X) - 90*a.scale
		for _, name := range names[a.panelScroll:] {
			if y > float64(body.Max.Y) {
				break
			}
			a.drawText(bodyImg, a.fitText(name, valueX-x-10*a.scale), x, y+5*a.scale, 0, chromeText)
			if _, isBus := r.Bus(name); isBus {
				a.drawWatchField(bodyImg, name, valueX, y, float64(body.Max.X)-34*a.scale)
			} else {
				a.drawText(bodyImg, r.Format(name), valueX, y+5*a.scale, 0, a.valueColor(name))
			}
			if pin := a.pinOf(name); pin != sim.Floating {
				clr := color.RGBA{255, 125, 125, 255} // v1 pinned high
				if pin == sim.Low {
					clr = color.RGBA{109, 109, 255, 255} // v1 pinned low
				} else if pin == sim.Unstable {
					clr = chromeDim // some bits of a bus
				}
				sq := a.lineHeight() / 2
				fillRect(bodyImg, image.Rect(int(float64(body.Max.X)-24*a.scale), int(y+5*a.scale+sq/2), int(float64(body.Max.X)-24*a.scale+sq), int(y+5*a.scale+sq*1.5)), clr)
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

// panelHint explains the mouse buttons on the panel's rows.
func (a *app) panelHint(l layout, p image.Point) string {
	if !p.In(a.panelBody(l)) {
		return ""
	}
	switch a.currentTab() {
	case tabDevices:
		r, _, ok := a.devRowAt(a.panelBody(l), p)
		if !ok {
			return ""
		}
		if r.field == "status" {
			if info := a.ed.Devices()[r.dev]; info.Problem != "" {
				return info.Problem
			}
		}
		return devicesHint(r)
	case tabMemory:
		if i := a.rowAt(l, p); i >= 0 && i < len(a.memLines()) && a.memLines()[i].start < 0 {
			return "click to reload the init file (after rebuilding it)"
		}
		return "click a word to type a new value (while paused) · wheel scroll · highlighted: last access (blue read, pink write)"
	case tabComponents:
		if a.rowAt(l, p) == importRow(a.ed.Definitions()) {
			return "copy components (and the ones they use) from another document"
		}
		return "left click place a copy · right click rename · middle click delete (only if unused) · wheel scroll"
	case tabWatch:
		return "wire: left pin high · right pin low · middle release · number box: click and type a value (up/down steps), middle click releases · wheel scroll"
	case tabDiagnostics:
		return "click show it on the canvas · wheel scroll"
	}
	return ""
}

// editWatch starts editing a bus value in its Watch row. The field starts
// with the current value, replaced by the first character typed.
func (a *app) editWatch(name string) {
	r := a.ed.Runner()
	value := r.Format(name)
	if value == "?" {
		value = ""
	}
	a.prompt = &prompt{kind: uiSetWatch, target: name, buffer: value, fresh: true}
}

// stepWatch adds delta to the bus being edited, wrapping at its width, and
// applies it at once.
func (a *app) stepWatch(delta int) {
	p := a.prompt
	r := a.ed.Runner()
	if r == nil {
		return
	}
	bits, ok := r.Bus(p.target)
	if !ok {
		return
	}
	p.edited = true
	n, err := strconv.ParseUint(strings.TrimSpace(p.buffer), 0, 64)
	if err != nil {
		if v, err2 := r.Number(p.target); err2 == nil {
			n = v
		}
	}
	mask := uint64(1)<<len(bits) - 1
	if len(bits) >= 64 {
		mask = ^uint64(0)
	}
	n = (n + uint64(delta)) & mask
	p.buffer, p.fresh = strconv.FormatUint(n, 10), true
	a.ed.SetName(p.target, p.buffer)
}

// drawWatchField draws a bus value as an editable box from x0 to x1.
func (a *app) drawWatchField(dst *ebiten.Image, name string, x0, y, x1 float64) {
	r := a.ed.Runner()
	box := image.Rect(int(x0-5*a.scale), int(y+1*a.scale), int(x1), int(y+a.lineHeight()+9*a.scale))
	editing := a.prompt != nil && a.prompt.kind == uiSetWatch && a.prompt.target == name
	border := chromeLine
	if editing {
		border = cueColor
	}
	a.box(dst, box, color.White)
	if editing {
		vector.StrokeRect(dst, float32(box.Min.X), float32(box.Min.Y), float32(box.Dx()), float32(box.Dy()), float32(2*max(1, int(a.scale+0.5))), border, false)
	}
	if editing {
		a.drawText(dst, a.prompt.buffer+"_", x0, y+5*a.scale, 0, chromeText)
		return
	}
	a.drawText(dst, r.Format(name), x0, y+5*a.scale, 0, a.valueColor(name))
}

// fitText shortens s with ".." so it is at most w pixels wide.
func (a *app) fitText(s string, w float64) string {
	if a.textWidth(s, 0) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && a.textWidth(string(r)+"..", 0) > w {
		r = r[:len(r)-1]
	}
	return string(r) + ".."
}

// importRow is the Components tab row holding the Import button: after the
// components, or after the "none yet" line.
func importRow(defs []editor.DefInfo) int { return max(len(defs), 1) }
