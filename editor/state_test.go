package editor

import (
	"strings"
	"testing"
	"time"

	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/sim"
)

// clocked is the inverter with its input labelled clock.
const clocked = `{
  "format": "mipsim", "version": 3, "root": "top",
  "defs": {"top": {
    "rows": [".H#", "#T", ".L"],
    "labels": [{"x": 0, "y": 1, "name": "clock"}, {"x": 2, "y": 0, "name": "out"}]
  }}
}`

func loadClocked(t *testing.T) *doc.Document {
	t.Helper()
	d, err := doc.LoadString(clocked)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestRunningClock(t *testing.T) {
	e := New(loadClocked(t))
	if e.Enabled(ActRunPause) {
		t.Error("run enabled in edit mode")
	}
	e.Do(ActToggleSimulate)
	out, _ := e.Netlist().Lookup("out")
	if e.Value(out) != sim.High {
		t.Fatalf("out = %v at start (clock low)", e.Value(out))
	}
	e.Do(ActRunPause)
	if !e.Running() || e.Hz() != 4 {
		t.Fatalf("running %v at %v Hz", e.Running(), e.Hz())
	}
	e.Advance(125 * time.Millisecond) // 4 Hz = 8 half ticks per second: one half tick
	if e.Value(out) != sim.Low {
		t.Errorf("after one half tick out = %v", e.Value(out))
	}
	e.Advance(125 * time.Millisecond)
	if e.Value(out) != sim.High || e.Runner().Ticks() != 1 {
		t.Errorf("after a full tick out = %v, ticks %d", e.Value(out), e.Runner().Ticks())
	}
	e.Do(ActFaster)
	if e.Hz() != 8 {
		t.Errorf("faster: %v Hz", e.Hz())
	}
	e.Do(ActRunPause)
	e.Advance(time.Second)
	if e.Running() || e.Runner().Ticks() != 1 {
		t.Error("paused clock still ticks")
	}
	e.Do(ActRunPause)
	e.Advance(time.Hour) // a huge backlog is capped, not run
	if e.Runner().Ticks() > 1+maxHalfTicksPerAdvance/2 {
		t.Errorf("backlog not capped: %d ticks", e.Runner().Ticks())
	}
	e.Do(ActToggleSimulate)
	if e.Running() {
		t.Error("leaving simulate mode did not stop the clock")
	}
}

func TestStepShowsPropagation(t *testing.T) {
	e := New(loadClocked(t))
	e.Do(ActToggleSimulate)
	out, _ := e.Netlist().Lookup("out")
	e.Do(ActStep) // toggles the clock high and evaluates one net
	if e.Value(out) != sim.High {
		t.Fatalf("one step already changed out: %v", e.Value(out))
	}
	for i := 0; i < 10 && e.Runner().Sim().Pending() > 0; i++ {
		e.Do(ActStep)
	}
	if e.Value(out) != sim.Low {
		t.Errorf("after stepping through, out = %v", e.Value(out))
	}
	if !strings.HasPrefix(e.Status, "step") {
		t.Errorf("status %q", e.Status)
	}
}

func TestModifiedAndEnabled(t *testing.T) {
	e := New(doc.New())
	if e.Modified() || e.Enabled(ActUndo) || e.Enabled(ActCopy) || e.Enabled(ActPaste) {
		t.Fatal("fresh editor looks modified or has things to undo or copy")
	}
	drag(e, Mods{}, pt(0, 0), pt(3, 0))
	if !e.Modified() || !e.Enabled(ActUndo) {
		t.Error("a stroke did not mark the document modified")
	}
	e.MarkSaved()
	if e.Modified() {
		t.Error("still modified after saving")
	}
	e.Do(ActUndo)
	if !e.Modified() || !e.Enabled(ActRedo) {
		t.Error("undo after saving should be a modification")
	}
	selectRect(e, pt(0, 0), pt(1, 1))
	if !e.Enabled(ActCopy) || !e.Enabled(ActRotate) {
		t.Error("selection actions disabled with a selection")
	}
	e.Do(ActCopy)
	if !e.Enabled(ActPaste) {
		t.Error("paste disabled with a clipboard")
	}
	if !strings.Contains(e.Hint(), "drag select") {
		t.Errorf("select hint %q", e.Hint())
	}
}
