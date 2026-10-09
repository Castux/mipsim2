package editor

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Castux/mipsim2/devices"
	"github.com/Castux/mipsim2/internal/mipfile"
	"github.com/Castux/mipsim2/sim"
)

// TestRAMInEditor is the M8 completion check in the editor: with select and
// write enable pinned by clicking, buses set as the Watch panel does, a byte
// is written, read back, and a device error stops the simulation with a
// message.
func TestRAMInEditor(t *testing.T) {
	d, err := mipfile.Load(filepath.Join("..", "testdata", "runner", "ram.mip"))
	if err != nil {
		t.Fatal(err)
	}
	e := New(d)
	e.Do(ActToggleSimulate)
	if e.Mode() != SimulateMode {
		t.Fatalf("not simulating: %s", e.Status)
	}
	at := func(name string) (int, int) {
		for _, l := range d.RootDef().Labels {
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

// TestDeviceEditing configures a memory from scratch the way the Devices tab
// does: add, rename its buses to match the circuit, undo, and reload an init
// file while simulating.
func TestDeviceEditing(t *testing.T) {
	d, err := mipfile.Load(filepath.Join("..", "testdata", "runner", "ram.mip"))
	if err != nil {
		t.Fatal(err)
	}
	d.Devices = nil
	files := map[string][]byte{"prog.bin": {1, 2, 3}}
	e := New(d)
	e.ReadFile = func(name string) ([]byte, error) {
		if b, ok := files[name]; ok {
			return b, nil
		}
		return nil, fmt.Errorf("no file %s", name)
	}

	e.AddMemory()
	e.AddMemory()
	infos := e.Devices()
	if len(infos) != 2 || infos[0].Name != "ram" || infos[1].Name != "ram1" {
		t.Fatalf("devices %+v", infos)
	}
	// ram1 drives the same bus as ram: allowed, the runner reports it only if
	// both drive at once. Delete it anyway.
	e.DeleteDevice(1)
	if !e.SetMemoryField(0, "select", "cs") {
		t.Fatalf("set select: %s", e.Status)
	}
	if p := e.Devices()[0].Problem; !strings.Contains(p, "cs") || strings.Contains(p, "addr") {
		t.Errorf("problem %q", p)
	}
	if !e.SetMemoryField(0, "select", "sel") {
		t.Fatalf("set select: %s", e.Status)
	}
	if info := e.Devices()[0]; info.Problem != "" || !strings.Contains(info.Summary, "addr_0..7") {
		t.Errorf("after fixing select: %+v", info)
	}

	for _, bad := range [][2]string{{"words", "0"}, {"width", "65"}, {"addr", ""}, {"readonly", "maybe"}, {"colour", "red"}} {
		if e.SetMemoryField(0, bad[0], bad[1]) {
			t.Errorf("accepted %s=%q", bad[0], bad[1])
		}
	}
	e.SetMemoryField(0, "width", "4")
	if p := e.Devices()[0].Problem; p != "" {
		t.Errorf("4-bit memory on an 8-bit bus: %q (extra bits are allowed)", p)
	}
	e.Undo()
	if c := e.Devices()[0].Memory; c == nil || c.Width != 8 {
		t.Errorf("undo width: %+v", c)
	}

	e.SetMemoryField(0, "init", "missing.bin")
	if p := e.Devices()[0].Problem; !strings.Contains(p, "missing.bin") {
		t.Errorf("missing init: %q", p)
	}
	// Every problem is listed, without repeating the device's name.
	e.SetMemoryField(0, "write", "wr")
	if p := e.Devices()[0].Problem; !strings.Contains(p, "missing.bin") || !strings.Contains(p, "missing nets wr") || strings.Contains(p, `"ram"`) {
		t.Errorf("both problems: %q", p)
	}
	e.Undo()
	e.SetMemoryField(0, "init", "prog.bin")
	e.Do(ActToggleSimulate)
	if e.Mode() != SimulateMode {
		t.Fatalf("simulate: %s", e.Status)
	}
	mem := e.Runner().Devices()[0].(*devices.Memory)
	if mem.Words()[2] != 3 {
		t.Fatalf("init not loaded: %v", mem.Words()[:4])
	}
	files["prog.bin"] = []byte{9}
	e.ReloadInit("ram")
	if mem.Words()[0] != 9 || mem.Words()[2] != 0 || !strings.Contains(e.Status, "reloaded") {
		t.Errorf("reload: %v, %q", mem.Words()[:4], e.Status)
	}
}
