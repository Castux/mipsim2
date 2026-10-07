// Package editor is the editor state machine: tools, selection, clipboard,
// commands and undo. It has no graphics dependency, so every tool is tested
// with synthetic input events. The ui package renders its state and feeds it
// events.
package editor

import (
	"fmt"
	"image"
	"strings"
	"time"

	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/netlist"
	"github.com/Castux/mipsim2/runner"
	"github.com/Castux/mipsim2/sim"
)

// Mode is edit or simulate.
type Mode int

const (
	EditMode Mode = iota
	SimulateMode
)

func (m Mode) String() string {
	if m == SimulateMode {
		return "simulate"
	}
	return "edit"
}

// Tool is the active edit tool.
type Tool int

const (
	Pencil Tool = iota
)

func (t Tool) String() string { return [...]string{"pencil"}[t] }

// Button is a pointer button.
type Button int

const (
	Left Button = iota
	Right
	Middle
)

// Mods are the modifier keys held during a pointer event.
type Mods struct {
	Shift, Ctrl, Alt bool
}

// Action is a keyboard command, bound to keys by the ui's keymap.
type Action int

const (
	ActPencil Action = iota
	ActToggleSimulate
	ActTick
	ActHalfTick
	ActResetSim
	ActUndo
	ActRedo
)

// cheapCompile is the compile time below which strokes recompile on every
// pointer move, so roles and nets update live; above it, only on release.
const cheapCompile = 8 * time.Millisecond

// Editor holds the document and everything the user is doing with it.
type Editor struct {
	Doc *doc.Document

	mode Mode
	tool Tool

	nl          *netlist.Netlist
	dirty       bool
	lastCompile time.Duration
	compiles    int // increments on every compile, so renderers can cache

	run *runner.Runner

	undo, redo []Command
	stroke     *stroke

	hover image.Point

	// Status is a one-line message for the status bar.
	Status string
}

type stroke struct {
	value  bool // what the stroke paints
	start  image.Point
	last   image.Point
	axis   int // 0 free, 1 horizontal, 2 vertical (alt line lock)
	edit   *pixelEdit
	done   map[image.Point]bool // world cells visited by this stroke (lookups only)
	cells  []image.Point        // world cells changed by this stroke, in order
	locked bool
}

// New returns an editor on d in edit mode.
func New(d *doc.Document) *Editor {
	e := &Editor{Doc: d, dirty: true}
	e.Netlist()
	return e
}

// Mode returns the current mode.
func (e *Editor) Mode() Mode { return e.mode }

// Tool returns the current edit tool.
func (e *Editor) Tool() Tool { return e.tool }

// Compiles counts compilations; it changes whenever Netlist returns a new value.
func (e *Editor) Compiles() int { return e.compiles }

// Stale reports whether the document changed since the last compile (during
// a stroke on a large circuit), so renderers should overlay Stroke().
func (e *Editor) Stale() bool { return e.dirty }

// Runner returns the simulation runner in simulate mode, else nil.
func (e *Editor) Runner() *runner.Runner { return e.run }

// Netlist returns the compiled document, compiling it first if it changed.
func (e *Editor) Netlist() *netlist.Netlist {
	if e.dirty {
		start := time.Now()
		e.nl = netlist.CompileDoc(e.Doc, netlist.Options{})
		e.lastCompile = time.Since(start)
		e.dirty = false
		e.compiles++
	}
	return e.nl
}

// Stroke returns the pixels changed by the stroke in progress, in world
// coordinates, so a renderer can show them before the next compile.
func (e *Editor) Stroke() (cells []image.Point, value bool) {
	if e.stroke == nil {
		return nil, false
	}
	return e.stroke.cells, e.stroke.value
}

// Do performs a keyboard action.
func (e *Editor) Do(a Action) {
	switch a {
	case ActPencil:
		e.tool = Pencil
		e.setMode(EditMode)
	case ActToggleSimulate:
		if e.mode == SimulateMode {
			e.setMode(EditMode)
		} else {
			e.setMode(SimulateMode)
		}
	case ActTick, ActHalfTick:
		if e.run == nil {
			e.Status = "ticks need simulate mode (e)"
			return
		}
		var err error
		if a == ActTick {
			err = e.run.Tick(1)
		} else {
			err = e.run.HalfTick()
		}
		if err != nil {
			e.Status = err.Error()
		} else {
			e.Status = fmt.Sprintf("tick %d", e.run.Ticks())
		}
	case ActResetSim:
		if e.run != nil {
			e.startSim()
			e.Status = "simulation reset"
		}
	case ActUndo:
		e.Undo()
	case ActRedo:
		e.Redo()
	}
}

func (e *Editor) setMode(m Mode) {
	if m == e.mode {
		return
	}
	if m == EditMode {
		e.mode = EditMode
		e.run = nil
		e.Status = "edit mode"
		return
	}
	nl := e.Netlist()
	if nl.HasErrors() {
		n := 0
		for _, d := range nl.Diagnostics {
			if d.Level == netlist.Error {
				n++
			}
		}
		e.Status = fmt.Sprintf("cannot simulate: %d error(s); first: %v", n, firstError(nl))
		return
	}
	e.mode = SimulateMode
	e.startSim()
	e.Status = "simulate mode: left click pins high, right pins low, middle releases"
}

func firstError(nl *netlist.Netlist) netlist.Diagnostic {
	for _, d := range nl.Diagnostics {
		if d.Level == netlist.Error {
			return d
		}
	}
	return netlist.Diagnostic{}
}

func (e *Editor) startSim() {
	r, err := runner.New(e.Netlist(), runner.Options{})
	if err != nil {
		e.Status = err.Error()
		e.mode = EditMode
		e.run = nil
		return
	}
	e.run = r
}

// Value returns the simulated value of a net, or Floating in edit mode.
func (e *Editor) Value(n netlist.NetID) sim.Value {
	if e.run == nil || n == netlist.NoNet {
		return sim.Floating
	}
	return e.run.Sim().Value(n)
}

// PointerDown starts a stroke (edit mode) or pins a net (simulate mode).
func (e *Editor) PointerDown(p image.Point, b Button, m Mods) {
	e.hover = p
	if e.mode == SimulateMode {
		e.pinAt(p, b)
		return
	}
	if b != Left {
		return
	}
	loc := e.Doc.Locate(p)
	value := !e.Doc.Defs[loc.Def].Pixels.Get(loc.Local.X, loc.Local.Y)
	e.stroke = &stroke{value: value, start: p, last: p, edit: &pixelEdit{}, done: map[image.Point]bool{}}
	e.paint(p)
	e.afterPaint()
}

// PointerMove extends the stroke and updates the hover position.
func (e *Editor) PointerMove(p image.Point, m Mods) {
	e.hover = p
	s := e.stroke
	if s == nil {
		return
	}
	if m.Alt {
		p = s.lockTo(p)
	} else {
		s.axis, s.locked = 0, false
	}
	if p == s.last {
		return
	}
	for _, q := range line4(s.last, p)[1:] {
		e.paint(q)
	}
	s.last = p
	e.afterPaint()
}

// lockTo constrains p to a horizontal or vertical line through the stroke
// start, choosing the axis of the dominant movement once the cursor has moved
// 2 pixels, and keeping it until release (or until alt is let go).
func (s *stroke) lockTo(p image.Point) image.Point {
	if !s.locked {
		dx, dy := abs(p.X-s.start.X), abs(p.Y-s.start.Y)
		if max(dx, dy) < 2 {
			return s.last
		}
		s.locked = true
		if dx >= dy {
			s.axis = 1
		} else {
			s.axis = 2
		}
		// Continue from the start so the locked line is straight.
		if s.last != s.start {
			s.last = s.start
		}
	}
	if s.axis == 1 {
		return image.Pt(p.X, s.start.Y)
	}
	return image.Pt(s.start.X, p.Y)
}

// PointerUp finishes a stroke as one undoable command.
func (e *Editor) PointerUp(p image.Point, b Button) {
	s := e.stroke
	if s == nil || b != Left {
		return
	}
	e.stroke = nil
	if len(s.edit.changes) > 0 {
		e.undo = append(e.undo, s.edit)
		e.redo = nil
	}
	e.dirty = true
	e.Netlist()
	e.reportDiagnostics()
}

func (e *Editor) paint(p image.Point) {
	s := e.stroke
	if s.done[p] {
		return
	}
	s.done[p] = true
	loc := e.Doc.Locate(p)
	px := e.Doc.Defs[loc.Def].Pixels
	old := px.Get(loc.Local.X, loc.Local.Y)
	if old == s.value {
		return
	}
	px.Set(loc.Local.X, loc.Local.Y, s.value)
	s.cells = append(s.cells, p)
	s.edit.changes = append(s.edit.changes, pixelChange{def: loc.Def, p: loc.Local, old: old, new: s.value})
}

// afterPaint recompiles during a stroke only while compiling is cheap.
func (e *Editor) afterPaint() {
	e.dirty = true
	if e.lastCompile < cheapCompile {
		e.Netlist()
	}
}

func (e *Editor) reportDiagnostics() {
	errs, warns := 0, 0
	for _, d := range e.nl.Diagnostics {
		if d.Level == netlist.Error {
			errs++
		} else {
			warns++
		}
	}
	if errs+warns == 0 {
		e.Status = ""
		return
	}
	e.Status = fmt.Sprintf("%d error(s), %d warning(s); first: %v", errs, warns, e.nl.Diagnostics[0])
}

// Undo reverts the last command.
func (e *Editor) Undo() {
	if len(e.undo) == 0 {
		e.Status = "nothing to undo"
		return
	}
	c := e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	c.Undo(e.Doc)
	e.redo = append(e.redo, c)
	e.changed("undo " + c.Name())
}

// Redo reapplies the last undone command.
func (e *Editor) Redo() {
	if len(e.redo) == 0 {
		e.Status = "nothing to redo"
		return
	}
	c := e.redo[len(e.redo)-1]
	e.redo = e.redo[:len(e.redo)-1]
	c.Do(e.Doc)
	e.undo = append(e.undo, c)
	e.changed("redo " + c.Name())
}

// changed handles a document change outside a stroke: leave simulate mode,
// recompile and report.
func (e *Editor) changed(what string) {
	e.setMode(EditMode)
	e.dirty = true
	e.Netlist()
	e.reportDiagnostics()
	if e.Status == "" {
		e.Status = what
	}
}

func (e *Editor) pinAt(p image.Point, b Button) {
	n := e.nl.NetAt(p.X, p.Y)
	if n == netlist.NoNet {
		return
	}
	s := e.run.Sim()
	switch b {
	case Left:
		s.Pin(n, sim.High)
	case Right:
		s.Pin(n, sim.Low)
	case Middle:
		s.Unpin(n)
	}
	e.run.Settle()
}

// Hover describes the pixel under the pointer for the status bar.
func (e *Editor) Hover() (image.Point, netlist.NetID, string) {
	p := e.hover
	nl := e.nl
	role := nl.RoleAt(p.X, p.Y)
	n := nl.NetAt(p.X, p.Y)
	var b strings.Builder
	fmt.Fprintf(&b, "%d,%d", p.X, p.Y)
	roleNames := [...]string{"off", "wire", "high source", "low source", "transistor", "bridge", "thick (error)"}
	if role != netlist.RoleOff {
		fmt.Fprintf(&b, " %s", roleNames[role])
	}
	if n != netlist.NoNet {
		fmt.Fprintf(&b, " net %d", n)
		if names := nl.Nets[n].Names; len(names) > 0 {
			fmt.Fprintf(&b, " %s", strings.Join(names, ", "))
		}
		if e.run != nil {
			fmt.Fprintf(&b, " = %v", e.Value(n))
			if pin := e.run.Sim().Pinned(n); pin != sim.Floating {
				fmt.Fprintf(&b, " (pinned %v)", pin)
			}
		}
	}
	return p, n, b.String()
}
