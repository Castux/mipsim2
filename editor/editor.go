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

	"github.com/Castux/mipsim2/devices"
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
	Select
	LabelTool
)

func (t Tool) String() string { return [...]string{"pencil", "select", "label"}[t] }

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
	ActSelect
	ActLabel
	ActToggleSimulate
	ActTick
	ActHalfTick
	ActResetSim
	ActUndo
	ActRedo
	ActCopy
	ActCut
	ActPaste
	ActDelete
	ActMirror
	ActRotate
	ActEscape
	ActEnter     // while typing
	ActBackspace // while typing
	ActRunPause  // run or pause the clock
	ActStep      // slow motion: one simulator step
	ActSlower    // halve the clock rate
	ActFaster    // double the clock rate
	ActMakeComponent
	ActExplode
	ActRename // the selected instance
)

// cheapCompile is the compile time below which strokes recompile on every
// pointer move, so roles and nets update live; above it, only on release.
const cheapCompile = 8 * time.Millisecond

// Editor holds the document and everything the user is doing with it.
type Editor struct {
	Doc *doc.Document

	mode Mode
	tool Tool

	flat        *doc.Flat
	nl          *netlist.Netlist
	dirty       bool
	lastCompile time.Duration
	compiles    int // increments on every compile, so renderers can cache

	run *runner.Runner

	undo, redo []Command

	stroke *stroke
	sel    *selection
	drag   *dragState
	clip   *clip
	paste  bool // the clipboard follows the pointer until a click
	typing *typing

	hover image.Point

	running bool    // the clock runs on its own (simulate mode)
	hz      float64 // clock rate in full periods per second
	due     float64 // half ticks owed by Advance

	version, savedVersion int // document changes, and the version last saved

	// Status is a one-line message for the status bar.
	Status string

	// ReadFile loads files the document's devices refer to (memory init
	// data), relative to the document. Set by the ui; nil means none can be
	// read.
	ReadFile devices.ReadFile
}

type stroke struct {
	value  bool // what the stroke paints
	start  image.Point
	last   image.Point
	axis   int // 0 free, 1 horizontal, 2 vertical (alt line lock)
	locked bool
	edit   *pixelEdit
	done   map[image.Point]bool // world cells visited by this stroke (lookups only)
	cells  []image.Point        // world cells changed by this stroke, in order
}

// selection is a rectangle in one definition, its context.
type selection struct {
	path []int           // instance indices from the root to the context
	def  doc.DefID       // the context definition
	toW  doc.Affine      // context local -> world
	rect image.Rectangle // local coordinates
}

type dragState struct {
	start, cur image.Point // world
	moving     bool        // dragging the selection, not drawing a new one
	resizing   bool        // dragging a resize handle of the selected instance
	edge       int
}

type typing struct {
	kind   typingKind
	inst   doc.InstID // for an instance name
	def    doc.DefID
	at     image.Point // local coordinates of the label
	world  image.Point // where it was clicked
	old    string      // existing label name, "" for a new label
	buffer string
}

// New returns an editor on d in edit mode.
func New(d *doc.Document) *Editor {
	e := &Editor{Doc: d, dirty: true, hz: 4}
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
		e.flat = e.Doc.Flatten()
		e.nl = netlist.Compile(e.flat, netlist.Options{})
		e.lastCompile = time.Since(start)
		e.dirty = false
		e.compiles++
	}
	return e.nl
}

// Flat returns the flattened document of the last compile (for drawing labels).
func (e *Editor) Flat() *doc.Flat {
	e.Netlist()
	return e.flat
}

// Stroke returns the pixels changed by the stroke in progress, in world
// coordinates, so a renderer can show them before the next compile.
func (e *Editor) Stroke() (cells []image.Point, value bool) {
	if e.stroke == nil {
		return nil, false
	}
	return e.stroke.cells, e.stroke.value
}

// Typing returns the label being typed, if any.
func (e *Editor) Typing() (string, bool) {
	if e.typing == nil {
		return "", false
	}
	return e.typing.buffer, true
}

// TypeRune adds a character to the label being typed.
func (e *Editor) TypeRune(r rune) {
	if e.typing != nil && r >= ' ' && r != 127 {
		e.typing.buffer += string(r)
		e.Status = e.typing.kind.String() + ": " + e.typing.buffer + "_   (enter to apply, escape to cancel)"
	}
}

// Do performs a keyboard action.
func (e *Editor) Do(a Action) {
	if e.typing != nil {
		e.doTyping(a)
		return
	}
	switch a {
	case ActPencil:
		e.setTool(Pencil)
	case ActSelect:
		e.setTool(Select)
	case ActLabel:
		e.setTool(LabelTool)
	case ActToggleSimulate:
		if e.mode == SimulateMode {
			e.setMode(EditMode)
		} else {
			e.cancel()
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
		e.cancel()
		e.Undo()
	case ActRedo:
		e.cancel()
		e.Redo()
	case ActCopy:
		e.copySelection()
	case ActCut:
		if e.copySelection() {
			e.deleteSelection("cut")
		}
	case ActPaste:
		e.startPaste()
	case ActDelete:
		e.deleteSelection("delete")
	case ActMirror:
		e.transformSelection(doc.Orient{Flip: true}, "mirror")
	case ActRotate:
		e.transformSelection(doc.Orient{Rot: 1}, "rotate")
	case ActEscape:
		e.cancel()
		e.sel = nil
	case ActRunPause, ActStep, ActSlower, ActFaster:
		e.doClock(a)
	case ActMakeComponent:
		e.makeComponent()
	case ActExplode:
		e.explode()
	case ActRename:
		e.startRenameInstance()
	}
}

func (e *Editor) setTool(t Tool) {
	e.cancel()
	e.tool = t
	if t != Select {
		e.sel = nil
	}
	e.setMode(EditMode)
	e.Status = t.String()
}

// cancel abandons a paste preview or a drag in progress.
func (e *Editor) cancel() {
	e.paste = false
	e.drag = nil
}

func (e *Editor) setMode(m Mode) {
	if m == e.mode {
		return
	}
	if m == EditMode {
		e.mode = EditMode
		e.run = nil
		e.running = false
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
	devs, err := runner.DevicesFromDoc(e.Doc, e.ReadFile)
	var r *runner.Runner
	if err == nil {
		r, err = runner.New(e.Netlist(), runner.Options{Devices: devs})
	}
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

// PointerDown handles a button press at world pixel p.
func (e *Editor) PointerDown(p image.Point, b Button, m Mods) {
	e.hover = p
	if e.typing != nil {
		e.commitTyping()
	}
	if e.mode == SimulateMode {
		e.pinAt(p, b)
		return
	}
	if e.paste {
		if b == Left {
			e.placePaste(p)
		} else {
			e.paste = false
			e.Status = "paste cancelled"
		}
		return
	}
	if b != Left {
		return
	}
	switch e.tool {
	case Pencil:
		loc := e.Doc.Locate(p)
		value := !e.Doc.Defs[loc.Def].Pixels.Get(loc.Local.X, loc.Local.Y)
		e.stroke = &stroke{value: value, start: p, last: p, edit: &pixelEdit{}, done: map[image.Point]bool{}}
		e.paint(p)
		e.afterPaint()
	case Select:
		if edge := e.handleAt(p); edge >= 0 {
			e.drag = &dragState{start: p, cur: p, resizing: true, edge: edge}
			return
		}
		moving := e.sel != nil && p.In(e.selWorld())
		e.drag = &dragState{start: p, cur: p, moving: moving}
	case LabelTool:
		e.startLabel(p)
	}
}

// PointerMove extends a stroke or drag and updates the hover position.
func (e *Editor) PointerMove(p image.Point, m Mods) {
	e.hover = p
	if e.drag != nil {
		e.drag.cur = p
		return
	}
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
		s.last = s.start // continue from the start so the line is straight
	}
	if s.axis == 1 {
		return image.Pt(p.X, s.start.Y)
	}
	return image.Pt(s.start.X, p.Y)
}

// PointerUp finishes a stroke or a drag.
func (e *Editor) PointerUp(p image.Point, b Button) {
	if b != Left {
		return
	}
	if d := e.drag; d != nil {
		e.drag = nil
		d.cur = p
		if d.resizing {
			e.resizeSelected(d.resizedRect(e.selWorld()))
		} else if d.moving {
			e.moveSelection(d)
		} else {
			e.selectArea(d.start, d.cur)
		}
		return
	}
	s := e.stroke
	if s == nil {
		return
	}
	e.stroke = nil
	if len(s.edit.changes) > 0 {
		e.undo = append(e.undo, s.edit)
		e.redo = nil
		e.version++
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
	e.sel = nil
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
	e.sel = nil
	e.changed("redo " + c.Name())
}

// changed handles a document change outside a stroke: leave simulate mode,
// recompile and report.
func (e *Editor) changed(what string) {
	e.version++
	e.setMode(EditMode)
	e.dirty = true
	e.Netlist()
	e.reportDiagnostics()
	if e.Status == "" {
		e.Status = what
	}
}

// ReplaceDocument swaps in a new document (after opening a file), clearing
// history and selection.
func (e *Editor) ReplaceDocument(d *doc.Document) {
	// compiles carries over: renderers cache on it, and restarting it could
	// repeat the previous document's count and keep its stale textures.
	*e = Editor{Doc: d, dirty: true, tool: e.tool, hz: e.hz, ReadFile: e.ReadFile, compiles: e.compiles}
	e.Netlist()
}

func (e *Editor) pinAt(p image.Point, b Button) {
	n := e.nl.NetAt(p.X, p.Y)
	if n == netlist.NoNet {
		return
	}
	switch b {
	case Left:
		e.run.PinNet(n, sim.High)
	case Right:
		e.run.PinNet(n, sim.Low)
	case Middle:
		e.run.PinNet(n, sim.Floating)
	}
	e.Settle()
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

// Pinned returns High or Low if a net is pinned in simulate mode, else Floating.
func (e *Editor) Pinned(n netlist.NetID) sim.Value {
	if e.run == nil || n == netlist.NoNet {
		return sim.Floating
	}
	return e.run.Sim().Pinned(n)
}
