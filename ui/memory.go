package ui

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/Castux/mipsim2/devices"
)

// The Memory tab shows every memory device as a foldable section: a header,
// then a window of its words as a hex grid, with the last access
// highlighted (light blue read, pink write, as in the canvas palette). The
// open sections share the panel's height, each scrolls on its own, and as
// many words go on a row as fit (a power of two, at most 16). While the
// clock is paused, clicking a word edits it; clicking an address jumps to
// another one.

func (a *app) memories() []*devices.Memory {
	r := a.ed.Runner()
	if r == nil {
		return nil
	}
	var ms []*devices.Memory
	for _, d := range r.Devices() {
		if m, ok := d.(*devices.Memory); ok {
			ms = append(ms, m)
		}
	}
	return ms
}

// memSection is where a memory sits in the tab.
type memSection struct {
	mem    *devices.Memory
	header image.Rectangle
	open   bool
	perRow int     // words per row
	rows   int     // rows shown
	total  int     // rows in all
	first  int     // first row shown
	y0     float64 // top of the first row
	x0     float64 // left of the first word
	cellW  float64
	digits int
}

// words returns the area of the word rows.
func (s memSection) words(body image.Rectangle, rowH float64) image.Rectangle {
	return image.Rect(body.Min.X, int(s.y0), body.Max.X, int(s.y0+float64(s.rows)*rowH))
}

func addrDigits(m *devices.Memory) int {
	return len(fmt.Sprintf("%x", max(1, len(m.Words())-1)))
}

// memSections lays out the tab.
func (a *app) memSections(body image.Rectangle) []memSection {
	ms := a.memories()
	if a.memFolded == nil {
		a.memFolded, a.memScroll = map[string]bool{}, map[string]int{}
	}
	rowH := a.rowHeight()
	open := 0
	for _, m := range ms {
		if !a.memFolded[m.Name()] {
			open++
		}
	}
	// The open sections share what the headers leave.
	share := 0
	if open > 0 {
		share = max(3, (int(float64(body.Dy())/rowH)-len(ms))/open)
	}
	pad := 10 * a.scale
	y := float64(body.Min.Y)
	var out []memSection
	for _, m := range ms {
		s := memSection{mem: m, open: !a.memFolded[m.Name()]}
		s.header = image.Rect(body.Min.X, int(y), body.Max.X, int(y+rowH))
		y += rowH
		s.digits = (m.Config().Width + 3) / 4
		s.x0 = float64(body.Min.X) + pad + a.textWidth(strings.Repeat("0", addrDigits(m))+" ", 0)
		s.cellW = a.textWidth(strings.Repeat("0", s.digits)+" ", 0)
		fit := int((float64(body.Max.X) - pad - s.x0) / s.cellW)
		s.perRow = 1
		for s.perRow*2 <= min(fit, 16) {
			s.perRow *= 2
		}
		s.total = (len(m.Words()) + s.perRow - 1) / s.perRow
		if s.open {
			s.rows = min(share, s.total)
			s.first = max(0, min(a.memScroll[m.Name()], s.total-s.rows))
			a.memScroll[m.Name()] = s.first
		}
		s.y0 = y
		y += float64(s.rows) * rowH
		out = append(out, s)
	}
	return out
}

// reloadRect is the header's reload link, when the memory has an init file.
func (a *app) reloadRect(s memSection) image.Rectangle {
	if s.mem.Config().Init == "" {
		return image.Rectangle{}
	}
	w := int(a.textWidth("reload", 0) + 16*a.scale)
	return image.Rect(s.header.Max.X-w, s.header.Min.Y, s.header.Max.X, s.header.Max.Y)
}

func (a *app) drawMemory(dst *ebiten.Image, body image.Rectangle) {
	a.memBody = body
	rowH := a.rowHeight()
	pad := 10 * a.scale
	for _, s := range a.memSections(body) {
		m, c := s.mem, s.mem.Config()
		hy := float64(s.header.Min.Y)
		arrow := a.lineHeight() * 0.8
		a.drawFoldArrow(dst, float64(body.Min.X)+pad, hy+5*a.scale, arrow, s.open)
		kind := "RAM"
		if c.ReadOnly {
			kind = "ROM"
		}
		head := fmt.Sprintf("%s  %s %dx%d", c.Name, kind, c.Words, c.Width)
		if c.Init != "" {
			head += "  · " + c.Init
		}
		reload := a.reloadRect(s)
		right := float64(body.Max.X) - pad
		if !reload.Empty() {
			right = float64(reload.Min.X)
			a.drawText(dst, "reload", float64(reload.Min.X)+6*a.scale, hy+5*a.scale, 0, cueColor)
		}
		x := float64(body.Min.X) + pad + arrow
		a.drawText(dst, a.fitText(head, right-x-pad), x, hy+5*a.scale, 0, chromeText)
		for r := range s.rows {
			row := s.first + r
			start := row * s.perRow
			y := s.y0 + float64(r)*rowH
			goTo := a.prompt != nil && a.prompt.kind == uiGoToAddress && a.prompt.target == c.Name && r == 0
			if goTo {
				cell := image.Rect(int(float64(body.Min.X)+pad-2*a.scale), int(y+2*a.scale), int(s.x0-4*a.scale), int(y+rowH-2*a.scale))
				a.box(dst, cell, color.White)
				a.drawText(dst, a.prompt.buffer+"_", float64(body.Min.X)+pad, y+5*a.scale, 0, chromeText)
			} else {
				a.drawText(dst, fmt.Sprintf("%0*x", addrDigits(m), start), float64(body.Min.X)+pad, y+5*a.scale, 0, chromeDim)
			}
			for j := start; j < min(start+s.perRow, len(m.Words())); j++ {
				x := s.x0 + float64(j-start)*s.cellW
				cell := image.Rect(int(x-2*a.scale), int(y+2*a.scale), int(x+s.cellW-4*a.scale), int(y+rowH-2*a.scale))
				editing := a.prompt != nil && a.prompt.kind == uiSetWord && a.prompt.target == wordTarget(c.Name, j)
				switch {
				case editing:
					a.box(dst, cell, color.White)
					a.drawText(dst, a.prompt.buffer+"_", x, y+5*a.scale, 0, chromeText)
					continue
				case j == m.Last && m.LastWrite:
					fillRect(dst, cell, color.RGBA{255, 192, 203, 255})
				case j == m.Last:
					fillRect(dst, cell, color.RGBA{173, 216, 230, 255})
				}
				clr := chromeText
				if m.Words()[j] == 0 {
					clr = chromeDim
				}
				a.drawText(dst, fmt.Sprintf("%0*x", s.digits, m.Words()[j]), x, y+5*a.scale, 0, clr)
			}
		}
		// A scroll bar when the window shows part of the memory.
		if s.rows > 0 && s.rows < s.total {
			track := s.words(body, rowH)
			x := float64(body.Max.X) - 5*a.scale
			h := float64(track.Dy())
			top := float64(track.Min.Y) + h*float64(s.first)/float64(s.total)
			thumb := max(4*a.scale, h*float64(s.rows)/float64(s.total))
			fillRect(dst, image.Rect(int(x), track.Min.Y, int(x+3*a.scale), track.Max.Y), chromeLine)
			fillRect(dst, image.Rect(int(x), int(top), int(x+3*a.scale), int(top+thumb)), chromeDim)
		}
	}
}

func wordTarget(mem string, i int) string { return fmt.Sprintf("%s:%d", mem, i) }

// memoryClick folds a section, reloads its init file, starts a jump to an
// address, or starts editing the word under p.
func (a *app) memoryClick(l layout, p image.Point) {
	body := a.panelBody(l)
	rowH := a.rowHeight()
	for _, s := range a.memSections(body) {
		m := s.mem
		if p.In(s.header) {
			if p.In(a.reloadRect(s)) {
				a.ed.ReloadInit(m.Name())
				return
			}
			a.memFolded[m.Name()] = s.open
			return
		}
		if !p.In(s.words(body, rowH)) {
			continue
		}
		if float64(p.X) < s.x0-2*a.scale {
			a.prompt = &prompt{kind: uiGoToAddress, target: m.Name(), buffer: "", fresh: true}
			return
		}
		row := s.first + int((float64(p.Y)-s.y0)/rowH)
		j := row*s.perRow + int((float64(p.X)-s.x0+2*a.scale)/s.cellW)
		if j >= min((row+1)*s.perRow, len(m.Words())) {
			return
		}
		if a.ed.Running() {
			a.ed.Status = "pause the clock (space) to edit memory"
			return
		}
		a.prompt = &prompt{kind: uiSetWord, target: wordTarget(m.Name(), j), buffer: fmt.Sprintf("%x", m.Words()[j]), fresh: true}
		return
	}
}

// memoryWheel scrolls the section under p by n notches.
func (a *app) memoryWheel(l layout, p image.Point, n int) {
	body := a.panelBody(l)
	for _, s := range a.memSections(body) {
		if p.In(s.header) || p.In(s.words(body, a.rowHeight())) {
			a.memScroll[s.mem.Name()] = s.first - 3*n
			return
		}
	}
}

// memoryHint explains the mouse over the tab.
func (a *app) memoryHint(l layout, p image.Point) string {
	body := a.panelBody(l)
	for _, s := range a.memSections(body) {
		switch {
		case p.In(a.reloadRect(s)):
			return "reload the init file (after rebuilding it)"
		case p.In(s.header):
			return "click to fold or unfold"
		case p.In(s.words(body, a.rowHeight())) && float64(p.X) < s.x0-2*a.scale:
			return "click an address to jump to another one (hex)"
		}
	}
	return "click a word to type a new value (while paused) · click an address to jump · wheel scroll · highlighted: last access (blue read, pink write)"
}

// goToAddress scrolls a memory so that the typed address (hex) is in its
// first row.
func (a *app) goToAddress(mem, value string) {
	v, err := parseWord(value)
	if err != nil {
		a.ed.Status = err.Error()
		return
	}
	for _, s := range a.memSections(a.memBody) {
		if s.mem.Name() != mem {
			continue
		}
		if v >= uint64(len(s.mem.Words())) {
			a.ed.Status = fmt.Sprintf("%s has %#x words", mem, len(s.mem.Words()))
			return
		}
		a.memScroll[mem] = int(v) / s.perRow
		return
	}
}

// setWord applies a typed value (hex, or 0x/0b/0d prefixed) to a memory word
// and settles, so a memory driving its bus shows the new value at once.
func (a *app) setWord(target, value string) {
	name, idx, _ := strings.Cut(target, ":")
	i, _ := strconv.Atoi(idx)
	for _, m := range a.memories() {
		if m.Name() != name || i >= len(m.Words()) {
			continue
		}
		v, err := parseWord(value)
		if err != nil {
			a.ed.Status = err.Error()
			return
		}
		if v&^m.Mask() != 0 {
			a.ed.Status = fmt.Sprintf("%#x does not fit in %d bits", v, m.Config().Width)
			return
		}
		m.Words()[i] = v
		if a.ed.Settle() {
			a.ed.Status = fmt.Sprintf("%s[%#x] = %#x", name, i, v)
		}
		return
	}
}

// parseWord reads a memory value: hex by default, as shown in the grid, or
// with an explicit 0x, 0b or 0d prefix.
func parseWord(s string) (uint64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	base := 16
	switch {
	case strings.HasPrefix(s, "0x"):
		s = s[2:]
	case strings.HasPrefix(s, "0b"):
		s, base = s[2:], 2
	case strings.HasPrefix(s, "0d"):
		s, base = s[2:], 10
	}
	v, err := strconv.ParseUint(s, base, 64)
	if err != nil {
		return 0, fmt.Errorf("not a number: %q (hex by default, or 0x, 0b, 0d)", s)
	}
	return v, nil
}
