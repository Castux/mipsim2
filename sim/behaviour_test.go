package sim

import (
	"fmt"
	"image"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Castux/mipsim2/bitmap"
	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/internal/mipfile"
	"github.com/Castux/mipsim2/netlist"
)

// Behaviour fixtures in testdata/sim and testdata/runner are documents
// with a test script: each step pins the nets and buses it sets, settles,
// then checks the values it expects (see doc.Step).

type harness struct {
	t      *testing.T
	d      *doc.Document
	sim    *Sim
	prefix string // added to names when the circuit is wrapped in an instance
}

func (h *harness) net(name string) (netlist.NetID, bool) {
	return h.sim.Netlist().Lookup(h.prefix + name)
}

// bits returns the nets of bus name_0, name_1, ..., or nil if there is none.
func (h *harness) bits(name string) []netlist.NetID {
	var ids []netlist.NetID
	for i := 0; ; i++ {
		id, ok := h.net(fmt.Sprintf("%s_%d", name, i))
		if !ok {
			return ids
		}
		ids = append(ids, id)
	}
}

func parseValue(s doc.Value) (Value, bool) {
	switch s {
	case "high", "1":
		return High, true
	case "low", "0":
		return Low, true
	case "floating", "float":
		return Floating, true
	case "unstable":
		return Unstable, true
	}
	return 0, false
}

// run executes the test script and returns a trace of all value changes.
func (h *harness) run() []Change {
	var trace []Change
	h.sim.OnChange = func(c Change) { trace = append(trace, c) }
	for i, step := range h.d.Tests {
		where := fmt.Sprintf("step %d", i+1)
		for _, name := range slices.Sorted(maps.Keys(step.Set)) {
			h.pin(where, name, step.Set[name])
		}
		h.sim.Settle()
		for _, name := range slices.Sorted(maps.Keys(step.Expect)) {
			h.check(where, name, step.Expect[name])
		}
		if len(step.AnyUnstable) > 0 {
			found := false
			for _, name := range step.AnyUnstable {
				id, ok := h.net(name)
				if !ok {
					h.t.Fatalf("%s: no net %s", where, name)
				}
				found = found || h.sim.Value(id) == Unstable
			}
			if !found {
				h.t.Errorf("%s%s: none of %v is unstable", h.label(), where, step.AnyUnstable)
			}
		}
	}
	return trace
}

func (h *harness) pin(where, name string, val doc.Value) {
	if id, ok := h.net(name); ok {
		v, ok := parseValue(val)
		switch {
		case !ok || v == Unstable:
			h.t.Fatalf("%s: cannot pin %s to %q", where, name, val)
		case v == Floating:
			h.sim.Unpin(id)
		default:
			h.sim.Pin(id, v)
		}
		return
	}
	bits := h.bits(name)
	if bits == nil {
		h.t.Fatalf("%s: no net or bus %s", where, name)
	}
	n, err := strconv.ParseUint(string(val), 0, 64)
	if err != nil {
		h.t.Fatalf("%s: bus %s needs a number, got %q", where, name, val)
	}
	for i, id := range bits {
		h.sim.Pin(id, map[bool]Value{true: High, false: Low}[n>>i&1 == 1])
	}
}

func (h *harness) check(where, name string, val doc.Value) {
	if id, ok := h.net(name); ok {
		want, ok := parseValue(val)
		if !ok {
			h.t.Fatalf("%s: bad value %q for %s", where, val, name)
		}
		if got := h.sim.Value(id); got != want {
			h.t.Errorf("%s%s: %s = %v, want %v", h.label(), where, name, got, want)
		}
		return
	}
	bits := h.bits(name)
	if bits == nil {
		h.t.Fatalf("%s: no net or bus %s", where, name)
	}
	want, err := strconv.ParseUint(string(val), 0, 64)
	if err != nil {
		h.t.Fatalf("%s: bus %s needs a number, got %q", where, name, val)
	}
	var got uint64
	for i, id := range bits {
		switch h.sim.Value(id) {
		case High:
			got |= 1 << i
		case Low:
		default:
			h.t.Errorf("%s%s: %s_%d is %v", h.label(), where, name, i, h.sim.Value(id))
			return
		}
	}
	if got != want {
		h.t.Errorf("%s%s: %s = %d, want %d", h.label(), where, name, got, want)
	}
}

func (h *harness) label() string {
	if h.prefix == "" {
		return ""
	}
	return "(wrapped " + h.prefix + ") "
}

func newHarness(t *testing.T, script, d *doc.Document, prefix string) *harness {
	t.Helper()
	nl := netlist.CompileDoc(d, netlist.Options{})
	if nl.HasErrors() {
		t.Fatalf("compile errors: %v", nl.Diagnostics)
	}
	s, err := New(nl, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, d: script, sim: s, prefix: prefix}
}

// wrap moves the fixture's root into a definition placed with orientation o,
// so every name gains the prefix "x.".
func wrap(d *doc.Document, o doc.Orient) *doc.Document {
	flat := d.Flatten()
	r := flat.Pixels.Bounds()
	for _, inst := range flat.Instances {
		r = r.Union(inst.Rect)
	}
	for _, l := range flat.Labels {
		r = r.Union(image.Rectangle{Min: l.Pos, Max: l.Pos.Add(image.Pt(1, 1))})
	}
	n := d.Clone()
	root := n.RootDef()
	def := doc.NewDefinition("wrapped", r.Dx(), r.Dy())
	root.Pixels.ForEachCell(func(x, y int, k bitmap.Kind) { def.Pixels.Put(x-r.Min.X, y-r.Min.Y, k) })
	for _, l := range root.Labels {
		def.Labels = append(def.Labels, doc.Label{X: l.X - r.Min.X, Y: l.Y - r.Min.Y, Name: l.Name})
	}
	for _, inst := range root.Instances {
		inst.X -= r.Min.X
		inst.Y -= r.Min.Y
		def.Instances = append(def.Instances, inst)
	}
	n.Defs["wrapped"] = def
	n.Defs[n.Root] = doc.NewDefinition(n.Root, 0, 0)
	n.RootDef().Instances = []doc.Instance{{ID: "w", Def: "wrapped", Orient: o, Name: "x"}}
	return n
}

func TestBehaviour(t *testing.T) {
	var files []string
	for _, dir := range []string{"sim", "runner"} {
		fs, err := filepath.Glob(filepath.Join("..", "testdata", dir, "*.mip"))
		if err != nil || len(fs) == 0 {
			t.Fatalf("no fixtures in %s: %v", dir, err)
		}
		files = append(files, fs...)
	}
	for _, path := range files {
		name := filepath.Base(filepath.Dir(path)) + "/" + strings.TrimSuffix(filepath.Base(path), ".mip")
		d, err := mipfile.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Tests) == 0 {
			if filepath.Base(filepath.Dir(path)) == "sim" {
				t.Errorf("%s has no tests", path)
			}
			continue
		}
		t.Run(name, func(t *testing.T) {
			// Determinism: two runs give identical traces.
			a := newHarness(t, d, d, "").run()
			b := newHarness(t, d, d, "").run()
			if !slices.Equal(a, b) {
				t.Errorf("two runs gave different traces (%d and %d changes)", len(a), len(b))
			}

			// The same behaviour in every orientation.
			for _, o := range doc.AllOrients {
				newHarness(t, d, wrap(d, o), "x.").run()
			}
		})
	}
}
