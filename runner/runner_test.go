package runner

import (
	"math/rand/v2"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/internal/mipfile"
	"github.com/Castux/mipsim2/netlist"
	"github.com/Castux/mipsim2/sim"
)

func load(t *testing.T, path string, opts Options) *Runner {
	t.Helper()
	d, err := mipfile.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return fromDoc(t, d, opts)
}

func fromDoc(t *testing.T, d *doc.Document, opts Options) *Runner {
	t.Helper()
	nl := netlist.CompileDoc(d, netlist.Options{})
	if nl.HasErrors() {
		t.Fatalf("compile errors: %v", nl.Diagnostics)
	}
	r, err := New(nl, opts)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAdderAllInputs(t *testing.T) {
	r := load(t, filepath.Join("..", "testdata", "runner", "adder4.mip"), Options{})
	for _, name := range []string{"a", "b", "sum"} {
		if bits, ok := r.Bus(name); !ok || len(bits) != 4 {
			t.Fatalf("bus %s = %v", name, bits)
		}
	}
	for a := range 16 {
		for b := range 16 {
			for c := range 2 {
				for _, kv := range [][2]string{{"a", strconv.Itoa(a)}, {"b", strconv.Itoa(b)}, {"cin", strconv.Itoa(c)}} {
					if err := r.Set(kv[0], kv[1]); err != nil {
						t.Fatal(err)
					}
				}
				r.Settle()
				sum, err := r.Number("sum")
				if err != nil {
					t.Fatalf("%d+%d+%d: %v", a, b, c, err)
				}
				cout, _ := r.Value("cout")
				got := int(sum)
				if cout == sim.High {
					got += 16
				} else if cout != sim.Low {
					t.Fatalf("%d+%d+%d: cout is %v", a, b, c, cout)
				}
				if got != a+b+c {
					t.Fatalf("%d+%d+%d = %d", a, b, c, got)
				}
			}
		}
	}
}

func TestNamesAndFormat(t *testing.T) {
	r := load(t, filepath.Join("..", "testdata", "runner", "adder4.mip"), Options{})
	names := r.Names()
	for _, want := range []string{"a", "a_0", "b_3", "cin", "cout", "sum", "sum_2"} {
		if !slices.Contains(names, want) {
			t.Errorf("Names() lacks %s: %v", want, names)
		}
	}
	if !slices.IsSorted(names) {
		t.Error("Names() not sorted")
	}
	if got := r.Format("cin"); got != "z" {
		t.Errorf("unpinned cin formats as %q", got)
	}
	r.Set("a", "0x3")
	r.Set("b", "0b101")
	r.Set("cin", "low")
	r.Settle()
	if got := r.Format("sum"); got != "8" {
		t.Errorf("sum = %q, want 8", got)
	}
	if got := r.Format("cout"); got != "0" {
		t.Errorf("cout = %q", got)
	}
	r.Set("a", "float")
	r.Settle()
	if got := r.Format("a"); got != "?" {
		t.Errorf("released bus a formats as %q", got)
	}
}

func TestSetErrors(t *testing.T) {
	r := load(t, filepath.Join("..", "testdata", "runner", "adder4.mip"), Options{})
	cases := [][2]string{{"nope", "1"}, {"a", "16"}, {"a", "x"}, {"cin", "5"}}
	for _, c := range cases {
		if err := r.Set(c[0], c[1]); err == nil {
			t.Errorf("Set(%q, %q) accepted", c[0], c[1])
		}
	}
	if _, err := r.Number("cin"); err == nil {
		t.Error("Number on a plain net accepted")
	}
	if r.HasClock() {
		t.Error("adder has no clock")
	}
	if err := r.HalfTick(); err == nil {
		t.Error("HalfTick without a clock accepted")
	}
}

func TestClockToggles(t *testing.T) {
	d, err := mipfile.Load(filepath.Join("..", "testdata", "sim", "inverter.mip"))
	if err != nil {
		t.Fatal(err)
	}
	r := fromDoc(t, d, Options{Clock: "in"})
	if !r.HasClock() || r.Format("out") != "1" {
		t.Fatalf("clock low at start should give out=1, got %s", r.Format("out"))
	}
	r.HalfTick()
	if r.Format("out") != "0" || r.Ticks() != 0 {
		t.Errorf("after rising edge: out=%s ticks=%d", r.Format("out"), r.Ticks())
	}
	r.HalfTick()
	if r.Format("out") != "1" || r.Ticks() != 1 {
		t.Errorf("after falling edge: out=%s ticks=%d", r.Format("out"), r.Ticks())
	}
	if err := r.Tick(3); err != nil || r.Ticks() != 4 || r.Format("out") != "1" {
		t.Errorf("after 3 more ticks: out=%s ticks=%d err=%v", r.Format("out"), r.Ticks(), err)
	}
}

func TestAdder8Sampled(t *testing.T) {
	r := load(t, filepath.Join("..", "testdata", "runner", "adder8.mip"), Options{})
	rng := rand.New(rand.NewPCG(9, 9))
	cases := [][3]int{{0, 0, 0}, {255, 255, 1}, {255, 1, 0}, {128, 128, 0}, {85, 170, 1}}
	for range 1500 {
		cases = append(cases, [3]int{rng.IntN(256), rng.IntN(256), rng.IntN(2)})
	}
	for _, c := range cases {
		r.Set("a", strconv.Itoa(c[0]))
		r.Set("b", strconv.Itoa(c[1]))
		r.Set("cin", strconv.Itoa(c[2]))
		r.Settle()
		sum, err := r.Number("sum")
		if err != nil {
			t.Fatalf("%v: %v", c, err)
		}
		got := int(sum)
		if r.Format("cout") == "1" {
			got += 256
		}
		if want := c[0] + c[1] + c[2]; got != want {
			t.Fatalf("%d+%d+%d = %d, want %d", c[0], c[1], c[2], got, want)
		}
	}
}
