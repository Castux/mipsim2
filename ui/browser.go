package ui

import (
	"image"
	"image/color"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/platform"
	"github.com/Castux/mipsim2/ui/filebrowser"
)

// The file dialog is a modal box drawn over the canvas, in the same pixel
// style as the rest of the ui. Its logic lives in ui/filebrowser.

func lister(dir string, done func([]filebrowser.Entry, error)) {
	platform.ListDir(dir, func(des []platform.DirEntry, err error) {
		out := make([]filebrowser.Entry, len(des))
		for i, d := range des {
			out[i] = filebrowser.Entry{Name: d.Name, Dir: d.Dir}
		}
		done(out, err)
	})
}

func (a *app) openBrowser(mode filebrowser.Mode) {
	start := a.opts.Path
	if start == "" {
		start = "."
		if mode == filebrowser.Save {
			start = "circuit.mip"
		}
	}
	a.fb = filebrowser.New(mode, start, lister, platform.Exists)
}

// cancelBrowser closes the dialog, dropping any action waiting on a save.
func (a *app) cancelBrowser() {
	a.fb = nil
	a.afterSave = nil
	a.importing = false
	a.ed.Status = "cancelled"
}

// finishBrowser acts on the chosen path.
func (a *app) finishBrowser(path string) {
	mode := a.fb.Mode
	a.fb = nil
	if a.importing {
		a.importing = false
		a.finishImportPick(path)
		return
	}
	if mode == filebrowser.Pick {
		a.finishPick(path)
		return
	}
	if mode == filebrowser.Save {
		a.save(path)
		return
	}
	platform.ReadFile(path, func(data []byte, err error) {
		if err != nil {
			a.ed.Status = "open failed: " + err.Error()
			return
		}
		d, err := doc.Load(data)
		if err != nil {
			a.ed.Status = "open failed: " + firstLine(err.Error())
			return
		}
		a.ed.ReplaceDocument(d)
		a.setPath(path)
		a.fit()
		a.ed.Status = "opened " + path
	})
}

// browserLayout places the dialog's parts.
type browserLayout struct {
	box, list, name, up, cancel, ok image.Rectangle
	rowH, rows                      int
}

func (a *app) browserLayout(l layout) browserLayout {
	var bl browserLayout
	c := l.canvas
	w, h := min(c.Dx()-a.u(40), a.u(760)), min(c.Dy()-a.u(40), a.u(560))
	x0, y0 := c.Min.X+(c.Dx()-w)/2, c.Min.Y+(c.Dy()-h)/2
	bl.box = image.Rect(x0, y0, x0+w, y0+h)
	lh := int(a.lineHeight())
	pad := a.u(10)
	bl.rowH = lh + a.u(8)
	btnH := lh + a.u(10)
	top := y0 + pad + lh + pad + lh + pad // title, folder
	bottom := y0 + h - pad - btnH - pad - btnH - pad
	bl.list = image.Rect(x0+pad, top, x0+w-pad, bottom)
	bl.rows = max(1, bl.list.Dy()/bl.rowH)
	bl.name = image.Rect(x0+pad, bottom+pad, x0+w-pad, bottom+pad+btnH)
	by := y0 + h - pad - btnH
	bw := int(a.textWidth("Cancel  esc", 0)) + a.u(20)
	bl.ok = image.Rect(x0+w-pad-bw, by, x0+w-pad, by+btnH)
	bl.cancel = image.Rect(bl.ok.Min.X-a.u(6)-bw, by, bl.ok.Min.X-a.u(6), by+btnH)
	uw := int(a.textWidth("Up  alt+up", 0)) + a.u(20)
	bl.up = image.Rect(x0+pad, by, x0+pad+uw, by+btnH)
	return bl
}

// handleBrowser takes keyboard and mouse input while the dialog is open.
func (a *app) handleBrowser(l layout) {
	b := a.fb
	bl := a.browserLayout(l)
	pressed := inpututil.IsKeyJustPressed
	repeat := func(k ebiten.Key) bool {
		d := inpututil.KeyPressDuration(k)
		return d == 1 || d > 20 && d%3 == 0
	}
	for _, r := range ebiten.AppendInputChars(nil) {
		b.TypeRune(r)
	}
	alt := ebiten.IsKeyPressed(ebiten.KeyAlt)
	switch {
	case pressed(ebiten.KeyEscape):
		a.cancelBrowser()
		return
	case pressed(ebiten.KeyEnter) || pressed(ebiten.KeyNumpadEnter):
		if path, done := b.Confirm(); done {
			a.finishBrowser(path)
			return
		}
	case alt && repeat(ebiten.KeyArrowUp):
		b.Parent()
	case repeat(ebiten.KeyArrowUp):
		b.Move(-1)
	case repeat(ebiten.KeyArrowDown):
		b.Move(1)
	case repeat(ebiten.KeyPageUp):
		b.Move(-bl.rows)
	case repeat(ebiten.KeyPageDown):
		b.Move(bl.rows)
	case repeat(ebiten.KeyBackspace):
		if b.Name == "" {
			b.Parent()
		} else {
			b.Backspace()
		}
	}
	// Keep the selection visible.
	if b.Selected >= 0 {
		b.Scroll = max(min(b.Scroll, b.Selected), b.Selected-bl.rows+1)
	}

	cx, cy := ebiten.CursorPosition()
	p := image.Pt(cx, cy)
	if _, wy := ebiten.Wheel(); wy != 0 && p.In(bl.list) {
		b.Scroll -= int(wy * 3)
	}
	b.Scroll = max(0, min(b.Scroll, len(b.Entries)-bl.rows))
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	switch {
	case p.In(bl.list):
		if path, done := b.Click(b.Scroll + (p.Y-bl.list.Min.Y)/bl.rowH); done {
			a.finishBrowser(path)
		}
	case p.In(bl.up):
		b.Parent()
	case p.In(bl.cancel):
		a.cancelBrowser()
	case p.In(bl.ok):
		if path, done := b.Confirm(); done {
			a.finishBrowser(path)
		}
	case !p.In(bl.box):
		a.cancelBrowser() // a click outside the dialog cancels it
	}
}

func (a *app) drawBrowser(screen *ebiten.Image, l layout) {
	b := a.fb
	bl := a.browserLayout(l)
	fillRect(screen, l.canvas, color.NRGBA{255, 255, 255, 170}) // dim the canvas (RGBA is premultiplied)
	a.box(screen, bl.box, chromeBg)
	lh := a.lineHeight()
	pad := float64(a.u(10))
	x := float64(bl.box.Min.X) + pad
	y := float64(bl.box.Min.Y) + pad
	a.drawText(screen, b.Mode.String(), x, y, 0, chromeText)
	y += lh + pad
	a.drawText(screen, a.fitText(b.Dir, float64(bl.box.Dx())-2*pad), x, y, 0, chromeDim)

	a.box(screen, bl.list, color.White)
	list := screen.SubImage(bl.list.Inset(a.textScale())).(*ebiten.Image)
	for i := 0; i < bl.rows && b.Scroll+i < len(b.Entries); i++ {
		e := b.Entries[b.Scroll+i]
		row := image.Rect(bl.list.Min.X, bl.list.Min.Y+i*bl.rowH, bl.list.Max.X, bl.list.Min.Y+(i+1)*bl.rowH)
		if b.Scroll+i == b.Selected {
			fillRect(list, row, chromeActive)
		}
		name, clr := e.Name, chromeText
		if e.Dir {
			name += string(filepath.Separator)
			clr = color.RGBA{40, 70, 160, 255}
		}
		a.drawText(list, name, float64(row.Min.X)+pad, float64(row.Min.Y)+float64(bl.rowH)/2-lh/2, 0, clr)
	}
	if len(b.Entries) == 0 {
		a.drawText(list, "(empty)", float64(bl.list.Min.X)+pad, float64(bl.list.Min.Y)+pad, 0, chromeDim)
	}

	a.box(screen, bl.name, color.White)
	ny := float64(bl.name.Min.Y+bl.name.Max.Y)/2 - lh/2
	a.drawText(screen, a.fitText(b.Name, float64(bl.name.Dx())-2*pad)+"_", float64(bl.name.Min.X)+pad, ny, 0, chromeText)

	cx, cy := ebiten.CursorPosition()
	cur := image.Pt(cx, cy)
	okLabel := map[filebrowser.Mode]string{filebrowser.Open: "Open", filebrowser.Save: "Save", filebrowser.Pick: "Choose"}[b.Mode]
	for _, btn := range []struct {
		r          image.Rectangle
		label, key string
	}{{bl.up, "Up", "alt+up"}, {bl.cancel, "Cancel", "esc"}, {bl.ok, okLabel, "enter"}} {
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
	if b.Err != "" {
		ex := float64(bl.up.Max.X) + pad
		a.drawText(screen, a.fitText(b.Err, float64(bl.cancel.Min.X-bl.up.Max.X)-2*pad), ex, float64(bl.up.Min.Y+bl.up.Max.Y)/2-lh/2, 0, errColor)
	}
}

// browserHint explains the dialog's keys on the hint line.
func browserHint() string {
	return "click, then click again or enter: choose · type a name or folder · alt+up: parent · esc: cancel"
}
