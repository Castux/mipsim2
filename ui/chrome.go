package ui

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/Castux/mipsim2/editor"
	"github.com/Castux/mipsim2/ui/filebrowser"
)

// Chrome colours: a light theme matching MiPSim v1's white canvas.
var (
	chromeBg     = color.RGBA{240, 240, 240, 255}
	chromeLine   = color.RGBA{200, 200, 200, 255}
	chromeEdge   = color.RGBA{96, 96, 96, 255} // box and button borders
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
	lh := int(a.lineHeight())
	topH, bottomH := lh+a.u(20), a.statusHeight()
	// The left column fits its longest label and key.
	leftW := 0
	for _, b := range keymap {
		if b.place == toolCol || b.place == editCol || b.place == compCol || b.place == simCol {
			leftW = max(leftW, int(a.textWidth(b.label+"  "+b.keyName, 0))+a.u(30))
		}
	}
	rightW := 0
	if a.panelVisible() {
		rightW = max(a.u(300), int(a.textWidth("MMMMMMMMMMMMMMMMMMMMMMMMMMMMMM", 0)))
	}
	l.top = image.Rect(0, 0, a.w, topH)
	l.bottom = image.Rect(0, a.h-bottomH, a.w, a.h)
	l.left = image.Rect(0, topH, leftW, a.h-bottomH)
	l.right = image.Rect(a.w-rightW, topH, a.w, a.h-bottomH)
	l.canvas = image.Rect(leftW, topH, a.w-rightW, a.h-bottomH)

	// Mode toggle: two segments at the top left.
	pad := a.u(6)
	segW := int(a.textWidth("Simulate  e", 0)) + a.u(24)
	x := pad
	for i, m := range []editor.Mode{editor.EditMode, editor.SimulateMode} {
		r := image.Rect(x, pad, x+segW, topH-pad)
		label := [...]string{"Edit", "Simulate"}[i]
		l.buttons = append(l.buttons, button{rect: r, b: keymap[0], enabled: true, active: mode == m, label: label, segment: true})
		x += segW
	}

	a.fileX = x + a.u(14)

	// File and history buttons at the top right.
	x = a.w - pad
	top := buttonsAt(topBar, mode)
	for i := len(top) - 1; i >= 0; i-- {
		b := top[i]
		bw := int(a.textWidth(b.label+"  "+b.keyName, 0)) + a.u(20)
		r := image.Rect(x-bw, pad, x, topH-pad)
		l.buttons = append(l.buttons, a.makeButton(r, b))
		x -= bw + a.u(2)
	}

	// Left column: tools and actions for the mode.
	y := l.left.Min.Y + pad
	rowH := lh + a.u(10)
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
		addCol(compCol)
	} else {
		addCol(simCol)
	}

	// Right panel tabs.
	if rightW > 0 {
		tx := l.right.Min.X + pad
		for _, t := range a.panelTabs() {
			w := int(a.textWidth(t.text, 0)) + a.u(16)
			l.tabs = append(l.tabs, tab{rect: image.Rect(tx, l.right.Min.Y+pad, tx+w, l.right.Min.Y+pad+lh+a.u(10)), id: t.id, text: t.text})
			tx += w + a.u(4)
		}
	}
	return l
}

func (a *app) makeButton(r image.Rectangle, b binding) button {
	bt := button{rect: r, b: b, enabled: true, label: b.label}
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
			bt.label, bt.active = "Pause", true
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
			a.openBrowser(filebrowser.Save)
		} else {
			a.save(a.opts.Path)
		}
	case uiSaveAs:
		a.openBrowser(filebrowser.Save)
	case uiOpen:
		a.openBrowser(filebrowser.Open)
	case uiFit:
		a.fit()
	case uiFilter:
		a.canvas.filter = (a.canvas.filter + 1) % 3
		a.ed.Status = "zoomed-out filter: " + [...]string{"average", "contrast boost", "any-on"}[a.canvas.filter]
	case uiScreenshot:
		a.shotPath, a.shotAt = fmt.Sprintf("mipsim-%d.png", a.frames), a.frames+1
	}
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
	// Just the file name; the window title has the full path.
	name := filepath.Base(a.opts.Path)
	if a.opts.Path == "" {
		name = "untitled"
	}
	if a.ed.Modified() {
		name += "  *"
	}
	right := a.w
	for _, bt := range l.buttons {
		if bt.b.place == topBar {
			right = min(right, bt.rect.Min.X)
		}
	}
	name = a.fitText(name, float64(right-a.fileX-a.u(10)))
	a.drawText(screen, name, float64(a.fileX), float64(l.top.Min.Y+l.top.Max.Y)/2-a.lineHeight()/2, 0, chromeText)

	// Clock rate and tick count under the simulate column.
	if a.ed.Mode() == editor.SimulateMode && a.ed.Runner() != nil {
		y := 0
		for _, bt := range l.buttons {
			if bt.b.place == simCol {
				y = max(y, bt.rect.Max.Y)
			}
		}
		x := float64(l.left.Min.X + a.u(10))
		lh := a.lineHeight()
		a.drawText(screen, fmt.Sprintf("%g Hz", a.ed.Hz()), x, float64(y+a.u(8)), 0, chromeText)
		a.drawText(screen, fmt.Sprintf("tick %d", a.ed.Runner().Ticks()), x, float64(y+a.u(12))+lh, 0, chromeDim)
		if !a.ed.Runner().HasClock() {
			a.drawText(screen, "no clock net", x, float64(y+a.u(16))+2*lh, 0, chromeDim)
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
	a.box(dst, bt.rect, bg)

	fg, keyClr := chromeText, chromeDim
	if !bt.enabled {
		fg, keyClr = chromeOff, chromeOff
	}
	r := bt.rect
	ty := float64(r.Min.Y+r.Max.Y)/2 - a.lineHeight()/2
	key := bt.b.keyName
	pad := float64(a.u(8))
	if bt.segment || bt.b.place == topBar {
		label := bt.label + "  " + key
		w := a.textWidth(label, 0)
		x := float64(r.Min.X+r.Max.X)/2 - w/2
		x += a.drawText(dst, bt.label+"  ", x, ty, 0, fg)
		a.drawText(dst, key, x, ty, 0, keyClr)
		return
	}
	a.drawText(dst, bt.label, float64(r.Min.X)+pad, ty, 0, fg)
	a.drawText(dst, key, float64(r.Max.X)-a.textWidth(key, 0)-pad, ty, 0, keyClr)
}

// box fills r and draws a single-pixel dark border (one pixel of the ui's
// pixel size, so it scales with the device like the text).
func (a *app) box(dst *ebiten.Image, r image.Rectangle, fill color.Color) {
	fillRect(dst, r, fill)
	w := a.textScale()
	for _, e := range []image.Rectangle{
		{r.Min, image.Pt(r.Max.X, r.Min.Y+w)}, {image.Pt(r.Min.X, r.Max.Y-w), r.Max},
		{r.Min, image.Pt(r.Min.X+w, r.Max.Y)}, {image.Pt(r.Max.X-w, r.Min.Y), r.Max},
	} {
		fillRect(dst, e, chromeEdge)
	}
}

// buttonHint names the button under the pointer, for the hint line.
func (a *app) buttonHint(l layout, p image.Point) string {
	for _, bt := range l.buttons {
		if p.In(bt.rect) && !bt.segment {
			s := bt.label + " (" + bt.b.keyName + ")"
			if !bt.enabled {
				s += " - not available now"
			}
			return s
		}
	}
	return ""
}
