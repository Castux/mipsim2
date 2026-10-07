package editor

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Castux/mipsim2/devices"
	"github.com/Castux/mipsim2/internal/fixture"
	"github.com/Castux/mipsim2/sim"
)

// TestRAMInEditor is the M8 completion check in the editor: with select and
// write enable pinned by clicking, buses set as the Watch panel does, a byte
// is written, read back, and a device error stops the simulation with a
// message.
func TestRAMInEditor(t *testing.T) {
	fx, err := fixture.ParseFile(filepath.Join("..", "testdata", "runner", "ram.fix"))
	if err != nil {
		t.Fatal(err)
	}
	e := New(fx.Doc)
	e.Do(ActToggleSimulate)
	if e.Mode() != SimulateMode {
		t.Fatalf("not simulating: %s", e.Status)
	}
	at := func(name string) (int, int) {
		for _, l := range fx.Doc.RootDef().Labels {
			if l.Name == name {
				return l.X, l.Y
			}
		}
		t.Fatalf("no label %q", name)
		return 0, 0
	}
	click := func(name string, b Button) {
		x, y := at(name)
		e.PointerDown(pt(x, y), b, Mods{})
		e.PointerUp(pt(x, y), b)
	}
	set := func(name, value string) {
		if err := e.Runner().Set(name, value); err != nil {
			t.Fatal(err)
		}
		if !e.Settle() {
			t.Fatalf("set %s=%s: %s", name, value, e.Status)
		}
	}
	mem := e.Runner().Devices()[0].(*devices.Memory)

	set("addr", "200")
	click("we", Left)
	click("sel", Left)
	set("data", "0x5c")
	if mem.Words()[200] != 0x5c || e.Runner().Format("q") != "163" {
		t.Fatalf("write: word %#x, q=%s, status %q", mem.Words()[200], e.Runner().Format("q"), e.Status)
	}

	click("we", Right)
	set("data", "float")
	if got := e.Runner().Format("data"); got != "92" {
		t.Errorf("read back data=%s, status %q", got, e.Status)
	}
	nl := e.Netlist()
	sel, _ := nl.Lookup("sel")
	if e.Value(sel) != sim.High {
		t.Errorf("sel = %v", e.Value(sel))
	}

	// Releasing an address bit while selected is a device error.
	x, y := at("addr_3")
	e.PointerDown(pt(x, y), Middle, Mods{})
	if !strings.Contains(e.Status, "stopped") || !strings.Contains(e.Status, "floating") {
		t.Errorf("status %q", e.Status)
	}
}
