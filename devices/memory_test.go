package devices

import (
	"encoding/json"
	"strings"
	"testing"
)

// fakeBus is a circuit of named nets whose value combines the device's pin
// and what the "circuit" drives, low winning, as in the simulator.
type fakeBus struct {
	drive map[string]Level // what the circuit drives
	pins  map[string]Level // the device's pins
}

func newFakeBus(names ...string) *fakeBus {
	b := &fakeBus{drive: map[string]Level{}, pins: map[string]Level{}}
	for _, n := range names {
		b.drive[n] = Floating
	}
	return b
}

func (b *fakeBus) Has(name string) bool { _, ok := b.drive[name]; return ok }
func (b *fakeBus) Value(name string) Level {
	p, d := b.pins[name], b.drive[name]
	switch {
	case p == Low || d == Low:
		return Low
	case p == High || d == High:
		return High
	}
	return d
}
func (b *fakeBus) Pin(name string, l Level) { b.pins[name] = l }
func (b *fakeBus) Unpin(name string)        { delete(b.pins, name) }
func (b *fakeBus) Pinned(name string) Level { return b.pins[name] }

func (b *fakeBus) set(base string, width int, v uint64) {
	for i := range width {
		l := Low
		if v>>i&1 == 1 {
			l = High
		}
		b.drive[bit(base, i)] = l
	}
}

func (b *fakeBus) read(base string, width int) uint64 {
	var v uint64
	for i := range width {
		if b.Value(bit(base, i)) == High {
			v |= 1 << i
		}
	}
	return v
}

func bit(base string, i int) string { return base + "_" + string(rune('0'+i)) }

func memBus() *fakeBus {
	var names []string
	for i := range 4 {
		names = append(names, bit("a", i))
	}
	for i := range 8 {
		names = append(names, bit("d", i))
	}
	return newFakeBus(append(names, "re", "we")...)
}

func memConfig() MemoryConfig {
	return MemoryConfig{Kind: "memory", Name: "m", Addr: "a", Data: "d", Read: "re", Write: "we", Words: 16, Width: 8}
}

// settle services the device until it stops changing, like the runner.
func settle(t *testing.T, m *Memory, b *fakeBus) error {
	t.Helper()
	for range 16 {
		changed, err := m.Service(b)
		if err != nil || !changed {
			return err
		}
	}
	t.Fatal("did not settle")
	return nil
}

// floatData stops the circuit driving the data bus.
func floatData(b *fakeBus) {
	for i := range 8 {
		b.drive[bit("d", i)] = Floating
	}
}

func TestMemoryReadWrite(t *testing.T) {
	m, err := NewMemory(memConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	b := memBus()
	if err := m.Attach(b); err != nil {
		t.Fatal(err)
	}
	// Idle: floating read and write count as low.
	if err := settle(t, m, b); err != nil {
		t.Fatalf("idle: %v", err)
	}

	// Read address 3 (0): the memory drives data at once.
	b.drive["re"], b.drive["we"] = High, Low
	b.set("a", 4, 3)
	if err := settle(t, m, b); err != nil {
		t.Fatal(err)
	}
	if got := b.read("d", 8); got != 0 || b.Pinned("d_0") != Low {
		t.Fatalf("read 0: data %d, pinned %v", got, b.Pinned("d_0"))
	}

	// Write 0xA5: lower read, raise write. The memory releases the bus
	// first, then stores what the circuit drives.
	b.drive["re"], b.drive["we"] = Low, High
	b.set("d", 8, 0xA5)
	changed, err := m.Service(b)
	if err != nil || !changed || b.Pinned("d_0") != Floating {
		t.Fatalf("release: changed %v err %v", changed, err)
	}
	if changed, err := m.Service(b); err != nil || changed || m.Words()[3] != 0xA5 {
		t.Fatalf("store: changed %v err %v word %#x", changed, err, m.Words()[3])
	}
	// Level-sensitive: while write is high the word follows the data bus.
	b.set("d", 8, 0x3C)
	if err := settle(t, m, b); err != nil || m.Words()[3] != 0x3C {
		t.Fatalf("follow: err %v word %#x", err, m.Words()[3])
	}
	// Lowering write keeps the last value, whatever the bus does next.
	b.drive["we"] = Low
	floatData(b)
	if err := settle(t, m, b); err != nil || m.Words()[3] != 0x3C {
		t.Fatalf("hold: err %v word %#x", err, m.Words()[3])
	}

	// Read it back.
	b.drive["re"] = High
	if err := settle(t, m, b); err != nil || b.read("d", 8) != 0x3C {
		t.Errorf("read back: err %v data %#x", err, b.read("d", 8))
	}
	// Lowering read releases the bus.
	b.drive["re"] = Low
	if err := settle(t, m, b); err != nil || b.Pinned("d_0") != Floating {
		t.Errorf("release: err %v, still pinned %v", err, b.Pinned("d_0"))
	}
}

// TestMemoryShort: read and write both high is allowed silently. The memory
// drives the word and stores what the bus settles to: low wins, so bits the
// circuit pulls low are cleared.
func TestMemoryShort(t *testing.T) {
	m, err := NewMemory(memConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	b := memBus()
	m.Words()[5] = 0xF0
	b.set("a", 4, 5)
	b.drive["re"], b.drive["we"] = High, High
	floatData(b)
	b.drive["d_4"], b.drive["d_7"] = Low, High // the circuit pulls bit 4 low
	if err := settle(t, m, b); err != nil {
		t.Fatal(err)
	}
	if m.Words()[5] != 0xE0 || b.read("d", 8) != 0xE0 {
		t.Errorf("word %#x, bus %#x, want 0xe0", m.Words()[5], b.read("d", 8))
	}
}

func TestMemoryErrors(t *testing.T) {
	cases := []struct {
		name  string
		setup func(b *fakeBus)
		cfg   func(c *MemoryConfig)
		want  string
	}{
		{"unstable read", func(b *fakeBus) { b.drive["re"] = Unstable }, nil, "read re is unstable"},
		{"unstable write", func(b *fakeBus) { b.drive["we"] = Unstable }, nil, "write we is unstable"},
		{"floating address bit", func(b *fakeBus) {
			b.drive["re"] = High
			b.set("a", 4, 1)
			b.drive["a_2"] = Floating
		}, nil, "address a_2 is floating"},
		{"floating data on write", func(b *fakeBus) {
			b.drive["we"] = High
			b.set("a", 4, 1)
		}, nil, "data d_0 is floating"},
		{"address out of range", func(b *fakeBus) {
			b.drive["re"] = High
			b.set("a", 4, 12)
		}, func(c *MemoryConfig) { c.Words = 10 }, "out of range"},
		{"write to ROM", func(b *fakeBus) {
			b.drive["we"] = High
			b.set("a", 4, 1)
			b.set("d", 8, 1)
		}, func(c *MemoryConfig) { c.ReadOnly = true }, "read-only"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := memConfig()
			if c.cfg != nil {
				c.cfg(&cfg)
			}
			m, err := NewMemory(cfg, nil)
			if err != nil {
				t.Fatal(err)
			}
			b := memBus()
			c.setup(b)
			_, err = m.Service(b)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestMemoryConfigAndInit(t *testing.T) {
	raw := json.RawMessage(`{"kind": "memory", "name": "rom", "addr": "a", "data": "d", "read": "re", "write": "we", "words": 4, "width": 12, "init": "rom.bin", "readonly": true}`)
	read := func(name string) ([]byte, error) {
		if name != "rom.bin" {
			t.Errorf("init file %q", name)
		}
		return []byte{0x34, 0x12, 0xFF, 0xFF, 0x01}, nil // 2 bytes per 12-bit word
	}
	d, err := Parse("memory", raw, read)
	if err != nil {
		t.Fatal(err)
	}
	m := d.(*Memory)
	if got := m.Words(); got[0] != 0x234 || got[1] != 0xFFF || got[2] != 0x001 || got[3] != 0 {
		t.Errorf("words %#x", got)
	}
	if err := m.Attach(newFakeBus("a_0", "re", "we")); err == nil || !strings.Contains(err.Error(), "a_1") {
		t.Errorf("missing nets not reported: %v", err)
	}
	for _, bad := range []string{
		`{"kind": "memory", "name": "x"}`,
		`{"kind": "memory", "name": "x", "addr": "a", "data": "d", "read": "s", "write": "w", "words": 0, "width": 8}`,
		`{"kind": "memory", "name": "x", "addr": "a", "data": "d", "read": "s", "write": "w", "words": 4, "width": 8, "colour": 1}`,
		`{"kind": "memory", "name": "x", "addr": "a", "data": "d", "select": "s", "write": "w", "words": 4, "width": 8}`,
	} {
		if _, err := Parse("memory", json.RawMessage(bad), nil); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	old := `{"kind": "memory", "name": "x", "addr": "a", "data": "d", "select": "s", "write": "w", "words": 4, "width": 8}`
	if _, err := Parse("memory", json.RawMessage(old), nil); err == nil || !strings.Contains(err.Error(), "replaced by") {
		t.Errorf("old select configuration: %v", err)
	}
	if _, err := Parse("console", nil, nil); err == nil {
		t.Error("unknown kind accepted")
	}
}

func TestOneWordMemoryHasNoAddress(t *testing.T) {
	c := memConfig()
	c.Words = 1
	m, err := NewMemory(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	b := newFakeBus("re", "we", "d_0", "d_1", "d_2", "d_3", "d_4", "d_5", "d_6", "d_7")
	if err := m.Attach(b); err != nil {
		t.Fatalf("attach without address nets: %v", err)
	}
	b.drive["re"], b.drive["we"] = High, Low
	m.Words()[0] = 0x5a
	if err := settle(t, m, b); err != nil || b.read("d", 8) != 0x5a {
		t.Errorf("read: %v %#x", err, b.read("d", 8))
	}
	if _, err := Parse("memory", json.RawMessage(`{"kind": "rom"}`), nil); err == nil {
		t.Error("a configuration of another kind parsed as a memory")
	}
}
