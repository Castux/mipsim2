package editor

import (
	"fmt"
	"time"
)

// doClock handles the simulate-mode clock actions.
func (e *Editor) doClock(a Action) {
	if e.run == nil {
		e.Status = "the clock runs in simulate mode (e)"
		return
	}
	switch a {
	case ActRunPause:
		if !e.run.HasClock() {
			e.Status = "no net labelled clock to drive"
			return
		}
		e.running = !e.running
		e.due = 0
		if e.running {
			e.Status = fmt.Sprintf("running at %g Hz", e.hz)
		} else {
			e.Status = fmt.Sprintf("paused at tick %d", e.run.Ticks())
		}
	case ActStep:
		e.running = false
		s := e.run.Sim()
		if s.Pending() == 0 {
			if !e.run.HasClock() {
				e.Status = "nothing to step: pin a net or add a clock"
				return
			}
			e.run.ToggleClock()
		}
		s.Step()
		e.Status = fmt.Sprintf("step %d, %d queued", s.Steps(), s.Pending())
	case ActSlower, ActFaster:
		if a == ActSlower {
			e.hz = max(e.hz/2, 0.25)
		} else {
			e.hz = min(e.hz*2, 1024)
		}
		e.Status = fmt.Sprintf("clock %g Hz", e.hz)
	}
}

// maxHalfTicksPerAdvance bounds the work done in one frame, so a slow
// circuit at a high rate does not freeze the editor.
const maxHalfTicksPerAdvance = 64

// advanceBudget bounds the time spent ticking in one frame, for circuits
// where even a few half ticks take milliseconds.
const advanceBudget = 8 * time.Millisecond

// Advance runs the clock for dt of real time if it is running.
func (e *Editor) Advance(dt time.Duration) {
	if !e.running || e.run == nil {
		return
	}
	e.due += dt.Seconds() * e.hz * 2
	start := time.Now()
	n := 0
	for e.due >= 1 && n < maxHalfTicksPerAdvance {
		if err := e.run.HalfTick(); err != nil {
			e.running = false
			e.Status = "stopped: " + err.Error()
			return
		}
		e.due--
		n++
		if time.Since(start) > advanceBudget {
			break // a large circuit: keep the frame rate, run slower
		}
	}
	if e.due >= 1 {
		e.due = 0 // falling behind: drop the backlog
	}
}

// Running reports whether the clock runs on its own.
func (e *Editor) Running() bool { return e.running }

// Hz returns the clock rate in full periods per second.
func (e *Editor) Hz() float64 { return e.hz }

// Modified reports whether the document changed since it was last saved.
func (e *Editor) Modified() bool { return e.top() != e.savedTop }

func (e *Editor) top() Command {
	if len(e.undo) == 0 {
		return nil
	}
	return e.undo[len(e.undo)-1]
}

// MarkSaved records that the document as it is now has been saved.
func (e *Editor) MarkSaved() { e.savedTop = e.top() }

// SaveMark identifies the document's current state. A save that completes
// later (on the web, writes are asynchronous) passes it to MarkSavedAt, so
// edits made meanwhile still count as unsaved.
type SaveMark struct{ top Command }

// Mark returns the current state, for MarkSavedAt.
func (e *Editor) Mark() SaveMark { return SaveMark{e.top()} }

// MarkSavedAt records that the state m was saved.
func (e *Editor) MarkSavedAt(m SaveMark) { e.savedTop = m.top }

// Pasting reports whether a paste preview follows the pointer.
func (e *Editor) Pasting() bool { return e.paste }

// Enabled reports whether an action can do anything right now, for greying
// out buttons.
func (e *Editor) Enabled(a Action) bool {
	sim := e.mode == SimulateMode
	switch a {
	case ActCopy, ActCut, ActDelete:
		return !sim && e.sel != nil
	case ActMirror, ActRotate:
		return !sim && (e.sel != nil || e.paste)
	case ActPaste:
		return !sim && e.clip != nil
	case ActUndo:
		return len(e.undo) > 0
	case ActRedo:
		return len(e.redo) > 0
	case ActTick, ActHalfTick, ActRunPause:
		return sim && e.run != nil && e.run.HasClock()
	case ActStep, ActResetSim, ActSlower, ActFaster:
		return sim
	case ActMakeComponent:
		return !sim && e.sel != nil
	case ActExplode:
		return !sim && len(e.selectedInstances()) > 0
	case ActRename:
		_, ok := e.selectedInstance()
		return !sim && ok
	}
	return true
}

// Hint describes the mouse gestures and modifiers for the current state,
// which the keymap's buttons cannot show.
func (e *Editor) Hint() string {
	const view = "space+drag pan · wheel zoom"
	switch {
	case e.typing != nil && e.typing.kind == typeLabel:
		return "type a name · enter apply · esc cancel · empty name removes the label"
	case e.typing != nil:
		return "type the " + e.typing.kind.String() + " · enter apply · esc cancel"
	case e.mode == SimulateMode:
		return "left click pin high · right click pin low · middle click release · space run/pause · " + view
	case e.paste:
		return "click place · right click or esc cancel · m/r mirror/rotate the paste · " + view
	case e.tool == Select && e.Enabled(ActRename):
		return "drag a square handle to resize the component (all its instances) · drag the selection to move it · esc clear · " + view
	case e.tool == Select:
		return "drag select an area · click select the innermost component, click again for the one around it · drag the selection to move it · esc clear · " + view
	case e.tool == LabelTool:
		return "click a cell to name its wire · click a label to rename it · " + view
	}
	return "left: wire or erase · right: cycle kind · 1-5: set kind · alt+drag: line · " + view
}

// Settle runs the simulation's settle loop. A device error (a memory reading
// a floating bit, two devices driving one net, ...) pauses the clock and is
// shown in the status bar; it reports whether the settle succeeded.
func (e *Editor) Settle() bool {
	if e.run == nil {
		return false
	}
	if err := e.run.Settle(); err != nil {
		e.running = false
		e.Status = "stopped: " + err.Error()
		return false
	}
	return true
}

// SetName pins a labelled net or bus to a value, as the runner's Set reads
// it ("high", "low", "float", or a number for a bus), and settles. Errors,
// including device errors while settling, go to the status line and stop
// the clock. It reports whether the value was applied cleanly.
func (e *Editor) SetName(name, value string) bool {
	if e.run == nil {
		return false
	}
	if err := e.run.Set(name, value); err != nil {
		e.Status = err.Error()
		return false
	}
	return e.Settle()
}
