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

// The Memory tab shows every memory device as a hex grid, 8 words per row,
// with the last access highlighted (light blue read, pink write, as in the
// canvas palette). While the clock is paused, clicking a word edits it.

const wordsPerRow = 8

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

// memLine is one line of the Memory tab: a memory's header, or a row of words.
type memLine struct {
	mem   int // index into memories()
	start int // first word of the row, or -1 for the header
}

func (a *app) memLines() []memLine {
	var lines []memLine
	for i, m := range a.memories() {
		lines = append(lines, memLine{mem: i, start: -1})
		for w := 0; w < len(m.Words()); w += wordsPerRow {
			lines = append(lines, memLine{mem: i, start: w})
		}
	}
	return lines
}

// memGeometry returns where word cells start and how wide they are.
func (a *app) memGeometry(body image.Rectangle, m *devices.Memory) (x0, cellW float64, digits int) {
	digits = (m.Config().Width + 3) / 4
	pad := 10 * a.scale
	addrW := a.textWidth(strings.Repeat("0", addrDigits(m))+" ", 0)
	return float64(body.Min.X) + pad + addrW, a.textWidth(strings.Repeat("0", digits)+" ", 0), digits
}

func addrDigits(m *devices.Memory) int {
	return len(fmt.Sprintf("%x", max(1, len(m.Words())-1)))
}

func (a *app) drawMemory(dst *ebiten.Image, body image.Rectangle) {
	ms := a.memories()
	lines := a.memLines()
	a.panelScroll = max(0, min(a.panelScroll, len(lines)-1))
	y := float64(body.Min.Y)
	pad := 10 * a.scale
	for _, ln := range lines[a.panelScroll:] {
		if y > float64(body.Max.Y) {
			break
		}
		m := ms[ln.mem]
		c := m.Config()
		if ln.start < 0 {
			kind := "RAM"
			if c.ReadOnly {
				kind = "ROM"
			}
			head := fmt.Sprintf("%s  %s %dx%d", c.Name, kind, c.Words, c.Width)
			if c.Init != "" {
				head += "  · " + c.Init + " (click: reload)"
			}
			a.drawText(dst, a.fitText(head, float64(body.Dx())-2*pad), float64(body.Min.X)+pad, y+5*a.scale, 0, chromeText)
			y += a.rowHeight()
			continue
		}
		x0, cellW, digits := a.memGeometry(body, m)
		a.drawText(dst, fmt.Sprintf("%0*x", addrDigits(m), ln.start), float64(body.Min.X)+pad, y+5*a.scale, 0, chromeDim)
		for j := ln.start; j < min(ln.start+wordsPerRow, len(m.Words())); j++ {
			x := x0 + float64(j-ln.start)*cellW
			cell := image.Rect(int(x-2*a.scale), int(y+2*a.scale), int(x+cellW-4*a.scale), int(y+a.rowHeight()-2*a.scale))
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
			a.drawText(dst, fmt.Sprintf("%0*x", digits, m.Words()[j]), x, y+5*a.scale, 0, clr)
		}
		y += a.rowHeight()
	}
}

func wordTarget(mem string, i int) string { return fmt.Sprintf("%s:%d", mem, i) }

// memoryClick starts editing the word under p.
func (a *app) memoryClick(l layout, p image.Point) {
	body := a.panelBody(l)
	lines := a.memLines()
	i := a.rowAt(l, p)
	if i < 0 || i >= len(lines) {
		return
	}
	if lines[i].start < 0 {
		a.ed.ReloadInit(a.memories()[lines[i].mem].Name())
		return
	}
	ln := lines[i]
	m := a.memories()[ln.mem]
	x0, cellW, _ := a.memGeometry(body, m)
	j := ln.start + int((float64(p.X)-x0+2*a.scale)/cellW)
	if float64(p.X) < x0-2*a.scale || j >= min(ln.start+wordsPerRow, len(m.Words())) {
		return
	}
	if a.ed.Running() {
		a.ed.Status = "pause the clock (space) to edit memory"
		return
	}
	a.prompt = &prompt{kind: uiSetWord, target: wordTarget(m.Name(), j), buffer: fmt.Sprintf("%x", m.Words()[j]), fresh: true}
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
