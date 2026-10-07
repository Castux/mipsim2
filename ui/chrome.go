package ui

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/Castux/mipsim2/editor"
)

// Chrome colours: a light theme matching MiPSim v1's white canvas.
var (
	chromeBg     = color.RGBA{240, 240, 240, 255}
	chromeLine   = color.RGBA{200, 200, 200, 255}
	chromeText   = color.RGBA{32, 32, 32, 255}
	chromeDim    = color.RGBA{112, 112, 112, 255}
	chromeOff    = color.RGBA{176, 176, 176, 255}
	chromeHover  = color.RGBA{226, 226, 226, 255}
	chromeActive = color.RGBA{192, 192, 192, 255} // v1 silver
	simPink      = color.RGBA{255, 192, 203, 255} // v1 pink
)

// layout is the screen split into chrome regions, recomputed every frame.
type layout struct {
	top, left, right, bottom, canvas image.Rectangle
	buttons                          []button
	tabs                             []tab
}

type button struct {
	rect    image.Rectangle
	b       binding
	enabled bool
	active  bool
	icon    string
	label   string
	segment bool // part of the mode toggle
}

type tab struct {
	rect image.Rectangle
	id   panelTab
	text string
}

func (a *app) u(x float64) int { return int(x * a.scale) }

// computeLayout splits the screen and places every button.
func (a *app) computeLayout() layout {
	var l layout
	ed := a.ed
	mode := ed.Mode()
	topH, leftW, bottomH := a.u(40), a.u(132), a.statusHeight()
	rightW := 0
	if a.panelVisible() {
		rightW = a.u(300)
	}
	l.top = image.Rect(0, 0, a.w, topH)
	l.bottom = image.Rect(0, a.h-bottomH, a.w, a.h)
	l.left = image.Rect(0, topH, leftW, a.h-bottomH)
	l.right = image.Rect(a.w-rightW, topH, a.w, a.h-bottomH)
	l.canvas = image.Rect(leftW, topH, a.w-rightW, a.h-bottomH)

	// Mode toggle: two segments at the top left.
	pad := a.u(6)
	segW := a.u(104)
	x := pad
	for i, m := range []editor.Mode{editor.EditMode, editor.SimulateMode} {
		r := image.Rect(x, pad, x+segW, topH-pad)
		label := [...]string{"Edit", "Simulate"}[i]
		l.buttons = append(l.buttons, button{rect: r, b: keymap[0], enabled: true, active: mode == m, label: label, segment: true})
		x += segW
	}

	// File and history buttons at the top right.
	bw := a.u(64)
	x = a.w - pad
	top := buttonsAt(topBar, mode)
	for i := len(top) - 1; i >= 0; i-- {
		b := top[i]
		r := image.Rect(x-bw, pad, x, topH-pad)
		l.buttons = append(l.buttons, a.makeButton(r, b))
		x -= bw + a.u(2)
	}

	// Left column: tools and actions for the mode.
	y := l.left.Min.Y + pad
	rowH := a.u(30)
	addCol := func(p place) {
		for _, b := range buttonsAt(p, mode) {
			r := image.Rect(l.left.Min.X+pad, y, l.left.Max.X-pad, y+rowH)
			l.buttons = append(l.buttons, a.makeButton(r, b))
			y += rowH + a.u(2)
		}
		y += a.u(10)
	}
	if mode == editor.EditMode {
		addCol(toolCol)
		addCol(editCol)
	} else {
		addCol(simCol)
	}

	// Right panel tabs.
	if rightW > 0 {
		tx := l.right.Min.X + pad
		for _, t := range a.panelTabs() {
			w := a.u(140)
			l.tabs = append(l.tabs, tab{rect: image.Rect(tx, l.right.Min.Y+pad, tx+w, l.right.Min.Y+pad+a.u(26)), id: t.id, text: t.text})
			tx += w + a.u(4)
		}
	}
	return l
}

func (a *app) makeButton(r image.Rectangle, b binding) button {
	bt := button{rect: r, b: b, enabled: true, icon: b.icon, label: b.label}
	if b.ui == uiNone {
		bt.enabled = a.ed.Enabled(b.action)
	}
	switch b.action {
	case editor.ActPencil:
		bt.active = a.ed.Tool() == editor.Pencil && a.ed.Mode() == editor.EditMode
	case editor.ActSelect:
		bt.active = a.ed.Tool() == editor.Select
	case editor.ActLabel:
		bt.active = a.ed.Tool() == editor.LabelTool
	case editor.ActRunPause:
		if a.ed.Running() {
			bt.icon, bt.label, bt.active = "pause", "Pause", true
		}
	}
	if b.ui == uiSave {
		bt.enabled = true
	}
	return bt
}

// clickChrome handles a click at p on the chrome and reports whether it
// hit any.
func (a *app) clickChrome(l layout, p image.Point, mb ebiten.MouseButton) bool {
	for _, bt := range l.buttons {
		if !p.In(bt.rect) || mb != ebiten.MouseButtonLeft {
			continue
		}
		if bt.segment {
			want := editor.EditMode
			if bt.label == "Simulate" {
				want = editor.SimulateMode
			}
			if a.ed.Mode() != want {
				a.ed.Do(editor.ActToggleSimulate)
			}
			return true
		}
		if bt.enabled {
			a.trigger(bt.b)
		}
		return true
	}
	for _, t := range l.tabs {
		if p.In(t.rect) {
			a.tab = t.id
			return true
		}
	}
	if p.In(l.right) {
		a.panelClick(l, p, mb)
		return true
	}
	return p.In(l.top) || p.In(l.left) || p.In(l.bottom)
}

// trigger performs a binding's action, from a key or a button.
func (a *app) trigger(b binding) {
	switch b.ui {
	case uiNone:
		a.ed.Do(b.action)
	case uiSave:
		if a.opts.Path == "" {
			a.prompt = &prompt{kind: uiSaveAs, buffer: "circuit.mip"}
		} else {
			a.save(a.opts.Path)
		}
	case uiSaveAs:
		a.prompt = &prompt{kind: uiSaveAs, buffer: a.opts.Path}
	case uiOpen:
		a.prompt = &prompt{kind: uiOpen, buffer: a.opts.Path}
	case uiFit:
		a.fit()
	case uiFilter:
		a.canvas.filter = (a.canvas.filter + 1) % 3
		a.ed.Status = "zoomed-out filter: " + [...]string{"average", "contrast boost", "any-on"}[a.canvas.filter]
	case uiScreenshot:
		a.shotPath, a.shotAt = fmt.Sprintf("mipsim-%d.png", a.frames), a.frames+1
	}
}

func (a *app) drawText(dst *ebiten.Image, s string, x, y, size float64, clr color.Color) float64 {
	a.face.Size = size * a.scale
	op := &text.DrawOptions{}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(clr)
	text.Draw(dst, s, a.face, op)
	w, _ := text.Measure(s, a.face, 0)
	return w
}

func (a *app) textWidth(s string, size float64) float64 {
	a.face.Size = size * a.scale
	w, _ := text.Measure(s, a.face, 0)
	return w
}

func fillRect(dst *ebiten.Image, r image.Rectangle, clr color.Color) {
	vector.FillRect(dst, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), clr, false)
}

func (a *app) drawChrome(screen *ebiten.Image, l layout) {
	cx, cy := ebiten.CursorPosition()
	cur := image.Pt(cx, cy)

	fillRect(screen, l.top, chromeBg)
	fillRect(screen, l.left, chromeBg)
	fillRect(screen, image.Rect(0, l.top.Max.Y-1, a.w, l.top.Max.Y), chromeLine)
	fillRect(screen, image.Rect(l.left.Max.X-1, l.left.Min.Y, l.left.Max.X, l.left.Max.Y), chromeLine)

	// Simulate mode frames the canvas in v1 pink.
	if a.ed.Mode() == editor.SimulateMode {
		c := l.canvas
		w := a.u(3)
		for _, r := range []image.Rectangle{
			image.Rect(c.Min.X, c.Min.Y, c.Max.X, c.Min.Y+w), image.Rect(c.Min.X, c.Max.Y-w, c.Max.X, c.Max.Y),
			image.Rect(c.Min.X, c.Min.Y, c.Min.X+w, c.Max.Y), image.Rect(c.Max.X-w, c.Min.Y, c.Max.X, c.Max.Y),
		} {
			fillRect(screen, r, simPink)
		}
	}

	for _, bt := range l.buttons {
		a.drawButton(screen, bt, cur.In(bt.rect))
	}

	// File name, with a dot when modified, after the mode toggle.
	name := a.opts.Path
	if name == "" {
		name = "untitled"
	}
	if a.ed.Modified() {
		name += "  •"
	}
	a.drawText(screen, name, float64(a.u(230)), float64(a.u(12)), 14, chromeText)

	// Clock rate and tick count under the simulate column.
	if a.ed.Mode() == editor.SimulateMode && a.ed.Runner() != nil {
		y := 0
		for _, bt := range l.buttons {
			if bt.b.place == simCol {
				y = max(y, bt.rect.Max.Y)
			}
		}
		x := float64(l.left.Min.X + a.u(10))
		a.drawText(screen, fmt.Sprintf("%g Hz", a.ed.Hz()), x, float64(y+a.u(8)), 13, chromeText)
		a.drawText(screen, fmt.Sprintf("tick %d", a.ed.Runner().Ticks()), x, float64(y+a.u(26)), 13, chromeDim)
		if !a.ed.Runner().HasClock() {
			a.drawText(screen, "no clock net", x, float64(y+a.u(44)), 12, chromeDim)
		}
	}
}

func (a *app) drawButton(dst *ebiten.Image, bt button, hover bool) {
	bg := chromeBg
	switch {
	case bt.active && bt.segment && bt.label == "Simulate":
		bg = simPink
	case bt.active:
		bg = chromeActive
	case hover && bt.enabled:
		bg = chromeHover
	}
	fillRect(dst, bt.rect, bg)
	vector.StrokeRect(dst, float32(bt.rect.Min.X), float32(bt.rect.Min.Y), float32(bt.rect.Dx()), float32(bt.rect.Dy()), 1, chromeLine, false)

	fg, keyClr := chromeText, chromeDim
	if !bt.enabled {
		fg, keyClr = chromeOff, chromeOff
	}
	r := bt.rect
	midY := float64(r.Min.Y+r.Max.Y) / 2
	key := bt.b.keyName

	if bt.segment {
		label := bt.label + "  " + key
		w := a.textWidth(label, 14)
		a.drawText(dst, label, float64(r.Min.X+r.Max.X)/2-w/2, midY-9*a.scale, 14, fg)
		return
	}
	s := float64(max(1, int(a.scale*1.5+0.5)))
	x := float64(r.Min.X) + 6*a.scale
	drawIcon(dst, bt.icon, x, midY-s*iconSize/2, s, fg)
	if bt.b.place == topBar {
		// Compact: icon and key only; the label is in the hover hint.
		a.drawText(dst, key, x+s*iconSize+5*a.scale, midY-8*a.scale, 12, keyClr)
		return
	}
	a.drawText(dst, bt.label, x+s*iconSize+8*a.scale, midY-9*a.scale, 14, fg)
	kw := a.textWidth(key, 12)
	a.drawText(dst, key, float64(r.Max.X)-kw-6*a.scale, midY-8*a.scale, 12, keyClr)
}

// buttonHint names the button under the pointer, for the hint line.
func (a *app) buttonHint(l layout, p image.Point) string {
	for _, bt := range l.buttons {
		if p.In(bt.rect) && !bt.segment {
			s := bt.label + " (" + bt.b.keyName + ")"
			if !bt.enabled {
				s += " — not available now"
			}
			return s
		}
	}
	return ""
}
