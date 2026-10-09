package sim

import (
	"path/filepath"
	"testing"

	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/internal/mipfile"
	"github.com/Castux/mipsim2/netlist"
)

func load(t *testing.T, name string) (*Sim, func(string) netlist.NetID) {
	t.Helper()
	d, err := mipfile.Load(filepath.Join("..", "testdata", "sim", name))
	if err != nil {
		t.Fatal(err)
	}
	nl := netlist.CompileDoc(d, netlist.Options{})
	s, err := New(nl, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return s, func(n string) netlist.NetID {
		id, ok := nl.Lookup(n)
		if !ok {
			t.Fatalf("no net %s", n)
		}
		return id
	}
}

func countUnstable(s *Sim) int {
	n := 0
	for id := range s.Netlist().Nets {
		if s.Value(netlist.NetID(id)) == Unstable {
			n++
		}
	}
	return n
}

func TestUnstableClearsAtNextSettle(t *testing.T) {
	s, net := load(t, "ring_osc.mip")
	if countUnstable(s) == 0 {
		t.Fatal("oscillator not unstable after reset")
	}

	// Breaking the loop with a pin starts a new settle: the unstable nets are
	// released, recomputed, and the circuit is stable.
	s.Pin(net("n1"), Low)
	s.Settle()
	if n := countUnstable(s); n != 0 {
		t.Fatalf("%d nets still unstable after breaking the loop", n)
	}
	if s.Value(net("n2")) != High || s.Value(net("n3")) != Low {
		t.Errorf("n2 = %v, n3 = %v; want high, low", s.Value(net("n2")), s.Value(net("n3")))
	}

	// Releasing it oscillates again.
	s.Unpin(net("n1"))
	s.Settle()
	if countUnstable(s) == 0 {
		t.Error("oscillator not unstable after releasing the pin")
	}

	// Settling with nothing queued changes nothing.
	before := countUnstable(s)
	if steps := s.Settle(); steps != 0 || countUnstable(s) != before {
		t.Errorf("idle settle took %d steps and changed unstable nets", steps)
	}
}

func TestLowWinsOverHighPin(t *testing.T) {
	s, net := load(t, "inverter.mip")
	s.Pin(net("in"), High)
	s.Pin(net("out"), High)
	s.Settle()
	if v := s.Value(net("out")); v != Low {
		t.Errorf("out = %v; a high pin must not beat a conducting path to ground", v)
	}
	s.Pin(net("in"), Low)
	s.Unpin(net("out"))
	s.Settle()
	if v := s.Value(net("out")); v != High {
		t.Errorf("out = %v, want high", v)
	}
	if s.Pinned(net("in")) != Low || s.Pinned(net("out")) != Floating {
		t.Error("Pinned reports wrong pins")
	}
}

func TestStepMatchesSettle(t *testing.T) {
	a, netA := load(t, "xor.mip")
	b, netB := load(t, "xor.mip")
	a.Pin(netA("a"), High)
	b.Pin(netB("a"), High)

	steps := a.Settle()
	n := 0
	for b.Pending() > 0 {
		if !b.Step() {
			t.Fatal("Step returned false with work pending")
		}
		n++
	}
	if b.Step() {
		t.Error("Step returned true with an empty queue")
	}
	if n != steps {
		t.Errorf("stepping took %d steps, Settle %d", n, steps)
	}
	for id := range a.Netlist().Nets {
		if a.Value(netlist.NetID(id)) != b.Value(netlist.NetID(id)) {
			t.Fatalf("net %d differs between Settle and Step", id)
		}
	}
	if a.Value(netA("out")) != High {
		t.Errorf("xor(1,0) = %v", a.Value(netA("out")))
	}
}

func TestResetClearsPins(t *testing.T) {
	s, net := load(t, "inverter.mip")
	s.Pin(net("in"), High)
	s.Settle()
	s.Reset()
	if s.Pinned(net("in")) != Floating || s.Value(net("in")) != Floating || s.Value(net("out")) != High {
		t.Errorf("after reset: in pinned %v value %v, out %v", s.Pinned(net("in")), s.Value(net("in")), s.Value(net("out")))
	}
}

func TestNewRejectsErrors(t *testing.T) {
	d := doc.New() // a 3x4 block: a thick-region error
	for y := range 4 {
		for x := range 3 {
			d.RootDef().Pixels.Set(x, y, true)
		}
	}
	nl := netlist.CompileDoc(d, netlist.Options{})
	if _, err := New(nl, Options{}); err != ErrNetlistHasErrors {
		t.Errorf("New on a netlist with errors: %v", err)
	}
}

// TestChangeSteps checks that every change reports the step it belongs to,
// including the release of unstable nets at the start of a settle.
func TestChangeSteps(t *testing.T) {
	s, net := load(t, "ring_osc.mip")
	var changes []Change
	s.OnChange = func(c Change) { changes = append(changes, c) }
	s.Pin(net("n1"), Low)
	if !s.Step() {
		t.Fatal("nothing to step")
	}
	if len(changes) == 0 {
		t.Fatal("no changes")
	}
	for _, c := range changes {
		if c.Step != s.Steps() {
			t.Errorf("change %+v reported in step %d", c, s.Steps())
		}
	}
}

// TestFlipThreshold checks the boundary: a transistor may switch threshold
// times in one settle, and the next switch marks its nets unstable. A
// negative threshold means the default, so pins stay clean.
func TestFlipThreshold(t *testing.T) {
	d, err := mipfile.Load(filepath.Join("..", "testdata", "sim", "inverter.mip"))
	if err != nil {
		t.Fatal(err)
	}
	nl := netlist.CompileDoc(d, netlist.Options{})
	s, err := New(nl, Options{FlipThreshold: -1})
	if err != nil {
		t.Fatal(err)
	}
	in, _ := nl.Lookup("in")
	s.Pin(in, High)
	s.Settle()
	if s.Value(in) != High {
		t.Errorf("negative threshold: pinned input is %v", s.Value(in))
	}

	for _, th := range []int{1, 5} {
		ring, _ := load(t, "ring_osc.mip")
		s, _ := New(ring.Netlist(), Options{FlipThreshold: th})
		most := int32(0)
		s.OnChange = func(Change) {
			for _, f := range s.flips {
				most = max(most, f)
			}
		}
		s.Reset()
		if countUnstable(s) == 0 || most != int32(th)+1 {
			t.Errorf("threshold %d: unstable %d, most flips seen %d (want %d)", th, countUnstable(s), most, th+1)
		}
	}
}
