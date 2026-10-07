package runner

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/internal/fixture"
	"github.com/Castux/mipsim2/netlist"
	"github.com/Castux/mipsim2/sim"
)

func ramRunner(t *testing.T, edit func(d *doc.Document)) (*Runner, error) {
	t.Helper()
	fx, err := fixture.ParseFile(filepath.Join("..", "testdata", "runner", "ram.fix"))
	if err != nil {
		t.Fatal(err)
	}
	if edit != nil {
		edit(fx.Doc)
	}
	devs, err := DevicesFromDoc(fx.Doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	nl := netlist.CompileDoc(fx.Doc, netlist.Options{})
	if nl.HasErrors() {
		t.Fatal(nl.Diagnostics)
	}
	return New(nl, Options{Devices: devs})
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// TestRAMReadWrite is the M8 completion check from Go: a circuit reads and
// writes a 256-byte RAM through its buses, and its own logic (inverters on
// the data bus) sees what the memory drives.
func TestRAMReadWrite(t *testing.T) {
	r, err := ramRunner(t, nil)
	must(t, err)
	value := func(i int) uint64 { return uint64(i*37+11) & 0xFF }

	must(t, r.Set("sel", "high"))
	must(t, r.Set("we", "high"))
	for i := range 256 {
		must(t, r.Set("addr", strconv.Itoa(i)))
		must(t, r.Set("data", strconv.FormatUint(value(i), 10)))
		must(t, r.Settle())
	}
	must(t, r.Set("we", "low"))
	must(t, r.Set("data", "float"))
	for i := range 256 {
		must(t, r.Set("addr", strconv.Itoa(i)))
		must(t, r.Settle())
		got, err := r.Number("data")
		must(t, err)
		if got != value(i) {
			t.Fatalf("address %d: read %d, want %d", i, got, value(i))
		}
		q, err := r.Number("q")
		must(t, err)
		if q != ^value(i)&0xFF {
			t.Fatalf("address %d: inverters show %d, want %d", i, q, ^value(i)&0xFF)
		}
	}
	if mem := r.Devices()[0]; mem.Name() != "ram" {
		t.Errorf("device %q", mem.Name())
	}
}

func TestPinLayers(t *testing.T) {
	r, err := ramRunner(t, nil)
	must(t, err)
	d0, _ := r.Netlist().Lookup("data_0")
	// The user pins data_0 high; the memory reads 0 at address 0 and drives
	// it low: low wins while both pin.
	must(t, r.Set("data_0", "high"))
	must(t, r.Set("sel", "high"))
	must(t, r.Set("we", "low"))
	must(t, r.Set("addr", "0"))
	must(t, r.Settle())
	if r.Sim().Value(d0) != sim.Low {
		t.Fatalf("data_0 = %v, want low (memory drives 0, low wins)", r.Sim().Value(d0))
	}
	// Deselecting releases the memory's pin; the user's pin is still there.
	must(t, r.Set("sel", "low"))
	must(t, r.Settle())
	if r.Sim().Value(d0) != sim.High || r.UserPin(d0) != sim.High {
		t.Errorf("after release: data_0 = %v, user pin %v", r.Sim().Value(d0), r.UserPin(d0))
	}
}

func TestDeviceErrors(t *testing.T) {
	// Two memories on the same buses, both reading: they drive data together.
	r, err := ramRunner(t, func(d *doc.Document) {
		c := d.Devices[0]
		c.Name = "ram2"
		c.Raw = json.RawMessage(strings.Replace(string(c.Raw), `"ram"`, `"ram2"`, 1))
		d.Devices = append(d.Devices, c)
	})
	must(t, err)
	must(t, r.Set("sel", "high"))
	must(t, r.Set("we", "low"))
	must(t, r.Set("addr", "5"))
	if err := r.Settle(); err == nil || !strings.Contains(err.Error(), "both drive") {
		t.Errorf("two drivers: %v", err)
	}

	// A floating address bit while selected.
	r, err = ramRunner(t, nil)
	must(t, err)
	must(t, r.Set("sel", "high"))
	must(t, r.Set("we", "low"))
	if err := r.Settle(); err == nil || !strings.Contains(err.Error(), "address addr_0 is floating") {
		t.Errorf("floating address: %v", err)
	}

	// A memory whose nets are missing is refused when attached.
	_, err = ramRunner(t, func(d *doc.Document) {
		d.Devices[0], _ = doc.NewDeviceConfig(json.RawMessage(strings.Replace(string(d.Devices[0].Raw), `"addr":"addr"`, `"addr":"nope"`, 1)))
	})
	if err == nil || !strings.Contains(err.Error(), "nope_0") {
		t.Errorf("missing nets: %v", err)
	}
}

// TestBusBitNames checks that only canonical bit numbers make bus bits:
// a_01 is a plain name, not a second bit 1.
func TestBusBitNames(t *testing.T) {
	if busBit.MatchString("a_01") || !busBit.MatchString("a_0") || !busBit.MatchString("a_10") {
		t.Error("bus bit pattern")
	}
}
