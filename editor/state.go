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

// Advance runs the clock for dt of real time if it is running.
func (e *Editor) Advance(dt time.Duration) {
	if !e.running || e.run == nil {
		return
	}
	e.due += dt.Seconds() * e.hz * 2
	n := 0
	for e.due >= 1 && n < maxHalfTicksPerAdvance {
		if err := e.run.HalfTick(); err != nil {
			e.running = false
			e.Status = err.Error()
			return
		}
		e.due--
		n++
	}
	if n == maxHalfTicksPerAdvance {
		e.due = 0 // falling behind: drop the backlog
	}
}

// Running reports whether the clock runs on its own.
func (e *Editor) Running() bool { return e.running }

// Hz returns the clock rate in full periods per second.
func (e *Editor) Hz() float64 { return e.hz }

// Modified reports whether the document changed since it was last saved.
func (e *Editor) Modified() bool { return e.version != e.savedVersion }

// MarkSaved records that the document as it is now has been saved.
func (e *Editor) MarkSaved() { e.savedVersion = e.version }

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
	}
	return true
}

// Hint describes the mouse gestures and modifiers for the current state,
// which the keymap's buttons cannot show.
func (e *Editor) Hint() string {
	const view = "space+drag pan · wheel zoom"
	switch {
	case e.typing != nil:
		return "type a name · enter apply · esc cancel · empty name removes the label"
	case e.mode == SimulateMode:
		return "left click pin high · right click pin low · middle click release · space run/pause · " + view
	case e.paste:
		return "click place · right click or esc cancel · m/r mirror/rotate the paste · " + view
	case e.tool == Select:
		return "drag select an area · click select a component · drag the selection to move it · esc clear · " + view
	case e.tool == LabelTool:
		return "click a pixel to name its wire · click a label to rename it · " + view
	}
	return "click toggle a pixel · drag paint · alt+drag straight line · " + view
}
