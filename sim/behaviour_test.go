package sim

import (
	"fmt"
	"image"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/internal/fixture"
	"github.com/Castux/mipsim2/netlist"
)

// Behaviour fixtures in testdata/sim declare inputs and outputs and list
// "# expect" lines. Each line pins the inputs it names (high, low, float, or
// a number for a name_N bus), settles, then checks the outputs it names.
// Pins persist from line to line, so sequential circuits can be tested.

type harness struct {
	t      *testing.T
	fx     *fixture.Fixture
	sim    *Sim
	prefix string // added to names when the circuit is wrapped in an instance
}

func (h *harness) net(name string) (netlist.NetID, bool) {
	return h.sim.Netlist().Lookup(h.prefix + name)
}

// bits returns the nets of bus name_0, name_1, ... declared in names.
func (h *harness) bits(name string, names []string) []netlist.NetID {
	var ids []netlist.NetID
	for i := 0; ; i++ {
		bit := fmt.Sprintf("%s_%d", name, i)
		if !slices.Contains(names, bit) {
			return ids
		}
		id, ok := h.net(bit)
		if !ok {
			h.t.Fatalf("no net %s", bit)
		}
		ids = append(ids, id)
	}
}

func parseValue(s string) (Value, bool) {
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

// run executes every expect line and returns a trace of all value changes.
func (h *harness) run() []Change {
	var trace []Change
	h.sim.OnChange = func(c Change) { trace = append(trace, c) }
	for _, dir := range h.fx.Find("expect") {
		where := fmt.Sprintf("line %d", dir.Line)
		var checks [][2]string
		for _, arg := range dir.Args {
			name, val, ok := strings.Cut(arg, "=")
			if !ok {
				h.t.Fatalf("%s: bad assignment %q", where, arg)
			}
			switch {
			case slices.Contains(h.fx.Inputs, name):
				h.pin(where, name, val)
			case slices.Contains(h.fx.Inputs, name+"_0"):
				n, err := strconv.ParseUint(val, 0, 64)
				if err != nil {
					h.t.Fatalf("%s: bus %s needs a number, got %q", where, name, val)
				}
				for i, id := range h.bits(name, h.fx.Inputs) {
					h.sim.Pin(id, map[bool]Value{true: High, false: Low}[n>>i&1 == 1])
				}
			case slices.Contains(h.fx.Outputs, name), slices.Contains(h.fx.Outputs, name+"_0"):
				checks = append(checks, [2]string{name, val})
			default:
				h.t.Fatalf("%s: %q is not a declared input or output", where, name)
			}
		}
		h.sim.Settle()
		for _, c := range checks {
			h.check(where, c[0], c[1])
		}
	}
	// "# any-unstable NAME...": after all expect lines, at least one of the
	// nets is Unstable. Which one trips first depends on evaluation order.
	for _, dir := range h.fx.Find("any-unstable") {
		found := false
		for _, name := range dir.Args {
			id, ok := h.net(name)
			if !ok {
				h.t.Fatalf("line %d: no net %s", dir.Line, name)
			}
			found = found || h.sim.Value(id) == Unstable
		}
		if !found {
			h.t.Errorf("%sline %d: none of %v is unstable", h.label(), dir.Line, dir.Args)
		}
	}
	return trace
}

func (h *harness) pin(where, name, val string) {
	id, ok := h.net(name)
	if !ok {
		h.t.Fatalf("%s: no net %s", where, name)
	}
	v, ok := parseValue(val)
	switch {
	case !ok || v == Unstable:
		h.t.Fatalf("%s: cannot pin %s to %q", where, name, val)
	case v == Floating:
		h.sim.Unpin(id)
	default:
		h.sim.Pin(id, v)
	}
}

func (h *harness) check(where, name, val string) {
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
	want, err := strconv.ParseUint(val, 0, 64)
	if err != nil {
		h.t.Fatalf("%s: bus %s needs a number, got %q", where, name, val)
	}
	var got uint64
	for i, id := range h.bits(name, h.fx.Outputs) {
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

func newHarness(t *testing.T, fx *fixture.Fixture, d *doc.Document, prefix string) *harness {
	t.Helper()
	nl := netlist.CompileDoc(d, netlist.Options{})
	if nl.HasErrors() {
		t.Fatalf("compile errors: %v", nl.Diagnostics)
	}
	s, err := New(nl, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, fx: fx, sim: s, prefix: prefix}
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
	root.Pixels.ForEach(func(x, y int) { def.Pixels.Set(x-r.Min.X, y-r.Min.Y, true) })
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
	files, err := filepath.Glob(filepath.Join("..", "testdata", "sim", "*.fix"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, path := range files {
		name := strings.TrimSuffix(filepath.Base(path), ".fix")
		t.Run(name, func(t *testing.T) {
			fx, err := fixture.ParseFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(fx.Find("expect")) == 0 {
				t.Fatal("fixture has no expect lines")
			}

			// Determinism: two runs give identical traces.
			a := newHarness(t, fx, fx.Doc, "").run()
			b := newHarness(t, fx, fx.Doc, "").run()
			if !slices.Equal(a, b) {
				t.Errorf("two runs gave different traces (%d and %d changes)", len(a), len(b))
			}

			// The same behaviour in every orientation.
			for _, o := range doc.AllOrients {
				newHarness(t, fx, wrap(fx.Doc, o), "x.").run()
			}
		})
	}
}
