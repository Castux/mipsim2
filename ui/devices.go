package ui

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/Castux/mipsim2/devices"
	"github.com/Castux/mipsim2/editor"
	"github.com/Castux/mipsim2/platform"
	"github.com/Castux/mipsim2/ui/filebrowser"
)

// The Devices tab (edit mode) configures the document's memories: one block
// per device with a header (name, delete), a row per field, and a status row
// saying what is missing in the circuit. Edits are undoable document edits.

// devRow is one row of the Devices tab.
type devRow struct {
	dev   int    // index into the document's devices; -1 for the add row
	field string // "" header, a memory field, "status", or "add"
}

func (a *app) devRows() []devRow {
	var rows []devRow
	for i, info := range a.ed.Devices() {
		rows = append(rows, devRow{dev: i})
		if info.Memory != nil {
			for _, f := range editor.MemoryFields {
				rows = append(rows, devRow{dev: i, field: f})
			}
		}
		rows = append(rows, devRow{dev: i, field: "status"})
	}
	return append(rows, devRow{dev: -1, field: "add"})
}

// fieldValue is a memory field as shown and as first put in the edit box.
func fieldValue(c *devices.MemoryConfig, field string) string {
	switch field {
	case "name":
		return c.Name
	case "addr":
		return c.Addr
	case "data":
		return c.Data
	case "select":
		return c.Select
	case "write":
		return c.Write
	case "words":
		return strconv.Itoa(c.Words)
	case "width":
		return strconv.Itoa(c.Width)
	case "readonly":
		if c.ReadOnly {
			return "yes"
		}
		return "no"
	case "init":
		return c.Init
	}
	return ""
}

var fieldLabels = map[string]string{
	"name": "name", "addr": "address bus", "data": "data bus", "select": "select net",
	"write": "write net", "words": "words", "width": "bits per word", "readonly": "read-only", "init": "init file",
}

// devGeometry places a row's parts: the value box, and the small button at
// the right end (delete on headers, browse on init).
func (a *app) devGeometry(body image.Rectangle, y float64) (value, button image.Rectangle) {
	bw := int(a.textWidth("delete", 0)) + a.u(12)
	right := body.Max.X - a.u(10)
	button = image.Rect(right-bw, int(y+1*a.scale), right, int(y+a.lineHeight()+9*a.scale))
	value = image.Rect(body.Min.X+int(a.textWidth("bits per word  ", 0))+a.u(10), button.Min.Y, button.Min.X-a.u(6), button.Max.Y)
	return value, button
}

func deviceTarget(i int, field string) string { return fmt.Sprintf("%d:%s", i, field) }

func (a *app) drawDevices(dst *ebiten.Image, body image.Rectangle) {
	infos := a.ed.Devices()
	rows := a.devRows()
	a.panelScroll = max(0, min(a.panelScroll, len(rows)-1))
	x := float64(body.Min.X) + 10*a.scale
	y := float64(body.Min.Y)
	cx, cy := ebiten.CursorPosition()
	cur := image.Pt(cx, cy)
	for _, r := range rows[a.panelScroll:] {
		if y > float64(body.Max.Y) {
			break
		}
		ty := y + 5*a.scale
		value, button := a.devGeometry(body, y)
		switch r.field {
		case "add":
			if len(infos) == 0 {
				a.drawText(dst, "no devices", x, ty, 0, chromeDim)
			}
			a.smallButton(dst, button.Union(value), "Add memory", cur)
		case "":
			info := infos[r.dev]
			a.drawText(dst, a.fitText(info.Kind+" "+info.Name, float64(button.Min.X)-x-6*a.scale), x, ty, 0, chromeText)
			a.smallButton(dst, button, "delete", cur)
		case "status":
			info := infos[r.dev]
			text, clr := "ok: "+info.Summary, color.Color(color.RGBA{40, 130, 60, 255})
			if info.Problem != "" {
				text, clr = info.Problem, errColor
			}
			a.drawText(dst, a.fitText(text, float64(body.Max.X)-x-10*a.scale), x, ty, 0, clr)
			y += a.rowHeight() / 2 // a gap before the next device
		default:
			c := infos[r.dev].Memory
			a.drawText(dst, fieldLabels[r.field], x, ty, 0, chromeDim)
			target := deviceTarget(r.dev, r.field)
			editing := a.prompt != nil && a.prompt.kind == uiSetDevice && a.prompt.target == target
			a.box(dst, value, color.White)
			vx := float64(value.Min.X) + 5*a.scale
			switch {
			case editing:
				vector.StrokeRect(dst, float32(value.Min.X), float32(value.Min.Y), float32(value.Dx()), float32(value.Dy()), float32(2*max(1, int(a.scale+0.5))), cueColor, false)
				a.drawText(dst, a.prompt.buffer+"_", vx, ty, 0, chromeText)
			case r.field == "init" && c.Init == "":
				a.drawText(dst, "(none)", vx, ty, 0, chromeOff)
			default:
				a.drawText(dst, a.fitText(fieldValue(c, r.field), float64(value.Dx())-10*a.scale), vx, ty, 0, chromeText)
			}
			if r.field == "init" {
				a.smallButton(dst, button, "...", cur)
			}
		}
		y += a.rowHeight()
	}
}

// devRowAt finds the row under p; rows after a device's status row are
// shifted by the half-row gap drawn there.
func (a *app) devRowAt(body image.Rectangle, p image.Point) (devRow, float64, bool) {
	rows := a.devRows()
	y := float64(body.Min.Y)
	for _, r := range rows[min(a.panelScroll, len(rows)-1):] {
		h := a.rowHeight()
		if float64(p.Y) >= y && float64(p.Y) < y+h {
			return r, y, true
		}
		if r.field == "status" {
			h += a.rowHeight() / 2
		}
		y += h
	}
	return devRow{}, 0, false
}

func (a *app) smallButton(dst *ebiten.Image, r image.Rectangle, label string, cur image.Point) {
	bg := chromeBg
	if cur.In(r) {
		bg = chromeHover
	}
	a.box(dst, r, bg)
	w := a.textWidth(label, 0)
	a.drawText(dst, label, float64(r.Min.X+r.Max.X)/2-w/2, float64(r.Min.Y+r.Max.Y)/2-a.lineHeight()/2, 0, chromeText)
}

func (a *app) devicesClick(l layout, p image.Point, mb ebiten.MouseButton) {
	body := a.panelBody(l)
	r, y, ok := a.devRowAt(body, p)
	if !ok || mb != ebiten.MouseButtonLeft {
		return
	}
	value, button := a.devGeometry(body, y)
	switch r.field {
	case "add":
		a.ed.AddMemory()
	case "":
		if p.In(button) {
			a.ed.DeleteDevice(r.dev)
		}
	case "status":
	case "readonly":
		if p.In(value) {
			c := a.ed.Devices()[r.dev].Memory
			a.ed.SetMemoryField(r.dev, "readonly", map[bool]string{true: "no", false: "yes"}[c.ReadOnly])
		}
	default:
		c := a.ed.Devices()[r.dev].Memory
		switch {
		case r.field == "init" && p.In(button):
			a.pickInit = r.dev
			start := a.docDir()
			if c.Init != "" {
				start = a.initPath(c.Init)
			}
			a.fb = filebrowser.New(filebrowser.Pick, start, lister, platform.Exists)
		case p.In(value):
			a.prompt = &prompt{kind: uiSetDevice, target: deviceTarget(r.dev, r.field), buffer: fieldValue(c, r.field), fresh: true}
		}
	}
}

// setDeviceField applies a typed device field ("index:field").
func (a *app) setDeviceField(target, value string) {
	idx, field, _ := strings.Cut(target, ":")
	i, _ := strconv.Atoi(idx)
	a.ed.SetMemoryField(i, field, value)
}

// docDir is the folder of the open document, or the current folder.
func (a *app) docDir() string {
	if a.opts.Path == "" {
		return "."
	}
	return filepath.Dir(a.opts.Path)
}

// initPath resolves an init file name as the runner does: relative to the
// document's folder.
func (a *app) initPath(name string) string {
	if filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(a.docDir(), name)
}

// finishPick stores a chosen init file, relative to the document's folder
// when it is inside it, so the document and its data can move together.
func (a *app) finishPick(path string) {
	i := a.pickInit
	name := path
	if abs, err := filepath.Abs(a.docDir()); err == nil {
		if rel, err := filepath.Rel(abs, path); err == nil && !strings.HasPrefix(rel, "..") {
			name = filepath.ToSlash(rel)
		}
	}
	a.ed.SetMemoryField(i, "init", name)
}

func devicesHint(r devRow) string {
	switch r.field {
	case "add":
		return "add a 256-byte RAM; then set its bus and net names to match labels in the circuit"
	case "":
		return "delete this device (undoable)"
	case "status":
		return "what the device attaches to, or what is missing in the circuit"
	case "addr", "data":
		return "click to type the bus base name: " + r.field + " means labels " + r.field + "_0, " + r.field + "_1, ..."
	case "readonly":
		return "click to toggle; a write to a read-only memory stops the simulation"
	case "init":
		return "click to type a file name (relative to the document), or ... to browse; empty for none"
	}
	return "click to type a new value, enter applies, esc cancels"
}
