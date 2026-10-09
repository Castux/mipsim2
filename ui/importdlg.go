package ui

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/editor"
	"github.com/Castux/mipsim2/platform"
	"github.com/Castux/mipsim2/ui/filebrowser"
)

// Importing components: the Components tab's Import button opens the file
// dialog; the chosen document's components are listed with check boxes.
// Checking one also checks (greyed) every component it uses. The selection
// logic is editor.ImportSelection; this file only draws it.

type importDialog struct {
	file   string
	sel    *editor.ImportSelection
	scroll int
}

// startImport opens the file dialog to choose the document to import from.
func (a *app) startImport() {
	a.openBrowser(filebrowser.Open)
	a.importing = true
}

// finishImportPick loads the chosen document and shows its components.
func (a *app) finishImportPick(path string) {
	platform.ReadFile(path, func(data []byte, err error) {
		if err != nil {
			a.ed.Status = "import failed: " + err.Error()
			return
		}
		d, err := doc.Load(data)
		if err != nil {
			a.ed.Status = "import failed: " + firstLine(err.Error())
			return
		}
		sel := editor.NewImportSelection(d)
		if len(sel.Items) == 0 {
			a.ed.Status = filepath.Base(path) + " has no components to import"
			return
		}
		a.imp = &importDialog{file: filepath.Base(path), sel: sel}
	})
}

type importLayout struct {
	box, list, ok, cancel image.Rectangle
	rowH, rows            int
}

func (a *app) importLayout(l layout) importLayout {
	var il importLayout
	c := l.canvas
	lh := int(a.lineHeight())
	pad := a.u(10)
	il.rowH = lh + a.u(8)
	btnH := lh + a.u(10)
	n := len(a.imp.sel.Items)
	w := min(c.Dx()-a.u(40), a.u(560))
	listH := min(n*il.rowH, c.Dy()-a.u(40)-(pad+lh+pad+pad+btnH+pad))
	il.rows = max(1, listH/il.rowH)
	listH = il.rows * il.rowH
	h := pad + lh + pad + listH + pad + btnH + pad
	x0, y0 := c.Min.X+(c.Dx()-w)/2, c.Min.Y+(c.Dy()-h)/2
	il.box = image.Rect(x0, y0, x0+w, y0+h)
	il.list = image.Rect(x0+pad, y0+pad+lh+pad, x0+w-pad, y0+pad+lh+pad+listH)
	bw := int(a.textWidth("Import 999  enter", 0)) + a.u(20)
	by := y0 + h - pad - btnH
	il.ok = image.Rect(x0+w-pad-bw, by, x0+w-pad, by+btnH)
	il.cancel = image.Rect(il.ok.Min.X-a.u(6)-bw, by, il.ok.Min.X-a.u(6), by+btnH)
	return il
}

func (a *app) handleImport(l layout) {
	d := a.imp
	il := a.importLayout(l)
	pressed := inpututil.IsKeyJustPressed
	switch {
	case pressed(ebiten.KeyEscape):
		a.imp = nil
		a.ed.Status = "import cancelled"
		return
	case pressed(ebiten.KeyEnter) || pressed(ebiten.KeyNumpadEnter):
		a.doImport()
		return
	}
	cx, cy := ebiten.CursorPosition()
	p := image.Pt(cx, cy)
	if _, wy := ebiten.Wheel(); wy != 0 && p.In(il.list) {
		d.scroll -= int(wy * 3)
	}
	d.scroll = max(0, min(d.scroll, len(d.sel.Items)-il.rows))
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	switch {
	case p.In(il.list):
		if i := d.scroll + (p.Y-il.list.Min.Y)/il.rowH; i < len(d.sel.Items) {
			d.sel.Toggle(d.sel.Items[i].ID)
		}
	case p.In(il.ok):
		a.doImport()
	case p.In(il.cancel), !p.In(il.box):
		a.imp = nil
		a.ed.Status = "import cancelled"
	}
}

func (a *app) doImport() {
	d := a.imp
	if len(d.sel.Chosen()) == 0 {
		a.ed.Status = "check the components to import"
		return
	}
	a.imp = nil
	a.ed.Import(d.sel.Src, d.sel.Chosen())
}

func (a *app) drawImport(screen *ebiten.Image, l layout) {
	d := a.imp
	il := a.importLayout(l)
	fillRect(screen, l.canvas, color.NRGBA{255, 255, 255, 170}) // dim the canvas (RGBA is premultiplied)
	a.box(screen, il.box, chromeBg)
	pad := float64(a.u(10))
	lh := a.lineHeight()
	a.drawText(screen, a.fitText("Import components from "+d.file, float64(il.box.Dx())-2*pad), float64(il.box.Min.X)+pad, float64(il.box.Min.Y)+pad, 0, chromeText)

	a.box(screen, il.list, color.White)
	list := screen.SubImage(il.list.Inset(a.textScale())).(*ebiten.Image)
	sq := int(lh * 0.7)
	for i := 0; i < il.rows && d.scroll+i < len(d.sel.Items); i++ {
		it := d.sel.Items[d.scroll+i]
		row := image.Rect(il.list.Min.X, il.list.Min.Y+i*il.rowH, il.list.Max.X, il.list.Min.Y+(i+1)*il.rowH)
		cy := (row.Min.Y + row.Max.Y) / 2
		x := row.Min.X + int(pad)
		forced := d.sel.Forced(it.ID)
		// The check box: filled when checked; grey when forced by another.
		boxR := image.Rect(x, cy-sq/2, x+sq, cy+sq/2)
		fill, mark := color.Color(color.White), chromeText
		if forced {
			fill, mark = chromeBg, chromeOff
		}
		a.box(list, boxR, fill)
		if d.sel.Checked(it.ID) {
			fillRect(list, boxR.Inset(sq/4), mark)
		}
		clr := chromeText
		if forced {
			clr = chromeDim
		}
		tx := float64(boxR.Max.X) + pad
		ty := float64(cy) - lh/2
		info := fmt.Sprintf("%dx%d", it.W, it.H)
		if n := len(it.Uses); n > 0 {
			info += fmt.Sprintf(" · uses %d", n)
		}
		infoW := a.textWidth(info, 0)
		tx += a.drawText(list, a.fitText(it.Name, float64(row.Max.X)-tx-infoW-2*pad), tx, ty, 0, clr)
		a.drawText(list, info, float64(row.Max.X)-infoW-pad, ty, 0, chromeDim)
	}

	cx, cy := ebiten.CursorPosition()
	cur := image.Pt(cx, cy)
	n := len(d.sel.Chosen())
	okLabel := fmt.Sprintf("Import %d", n)
	for _, btn := range []struct {
		r          image.Rectangle
		label, key string
	}{{il.cancel, "Cancel", "esc"}, {il.ok, okLabel, "enter"}} {
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

// importHint explains the dialog, naming what a hovered component uses.
func (a *app) importHint(l layout) string {
	il := a.importLayout(l)
	cx, cy := ebiten.CursorPosition()
	p := image.Pt(cx, cy)
	d := a.imp
	if p.In(il.list) {
		if i := d.scroll + (p.Y-il.list.Min.Y)/il.rowH; i < len(d.sel.Items) {
			it := d.sel.Items[i]
			if len(it.Uses) == 0 {
				return it.Name + " uses no other component"
			}
			var names []string
			for _, u := range it.Uses {
				names = append(names, d.sel.Src.Defs[u].Name)
			}
			return it.Name + " uses " + strings.Join(names, ", ") + " (imported with it)"
		}
	}
	return "click to check · components they use come along (greyed) · name clashes get a suffix · enter imports"
}
