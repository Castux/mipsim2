package devices

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/bits"
	"strings"
)

// MemoryConfig is the JSON configuration of a memory device.
type MemoryConfig struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Addr     string `json:"addr"`  // bus base name: addr_0 ... addr_{k-1}
	Data     string `json:"data"`  // bus base name: data_0 ... data_{width-1}
	Read     string `json:"read"`  // net name
	Write    string `json:"write"` // net name
	Words    int    `json:"words"`
	Width    int    `json:"width"`
	Init     string `json:"init,omitempty"` // raw binary file, little-endian words of ceil(width/8) bytes
	ReadOnly bool   `json:"readonly,omitempty"`
}

// Memory is a RAM or ROM attached to address, data, read and write nets,
// modelled as gated D latches:
//
//   - read high, write low: it pins data to the word at addr;
//   - write high, read low: it stores data at addr, at each settle point
//     while write stays high (level-sensitive), and keeps the value when
//     write goes low. If it was still pinning data (read just went low), it
//     releases the bus first and stores on the next service, so the circuit
//     alone drives the value;
//   - neither: it releases the data bus;
//   - both: a short, silently allowed. It pins data to the word and stores
//     what the bus settles to; since low wins, that is the word ANDed with
//     whatever the circuit drives.
//
// Floating read and write count as low, so an undriven memory is idle. An
// unstable read or write is an error, and so is any address or data bit it
// needs that is floating or unstable, an address out of range, or a write to
// a read-only memory.
type Memory struct {
	cfg   MemoryConfig
	addr  []string // bit names, LSB first
	data  []string
	words []uint64

	// The last access, for display: the address (-1 before any) and whether
	// it was a write.
	Last      int
	LastWrite bool
}

func parseMemory(raw json.RawMessage, read ReadFile) (*Memory, error) {
	var old struct {
		Select *string `json:"select"`
	}
	if json.Unmarshal(raw, &old) == nil && old.Select != nil {
		return nil, fmt.Errorf("memory: \"select\" was replaced by \"read\" (read high drives the data bus, write high stores it)")
	}
	var c MemoryConfig
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("memory: %w", err)
	}
	return NewMemory(c, read)
}

// NewMemory builds a memory from its configuration, loading its init file.
func NewMemory(c MemoryConfig, read ReadFile) (*Memory, error) {
	where := fmt.Sprintf("memory %q", c.Name)
	switch {
	case c.Name == "":
		return nil, fmt.Errorf("memory: needs a name")
	case c.Words < 1 || c.Words > 1<<24:
		return nil, fmt.Errorf("%s: words %d out of range 1..%d", where, c.Words, 1<<24)
	case c.Width < 1 || c.Width > 64:
		return nil, fmt.Errorf("%s: width %d out of range 1..64", where, c.Width)
	case c.Addr == "" || c.Data == "" || c.Read == "" || c.Write == "":
		return nil, fmt.Errorf("%s: needs addr, data, read and write", where)
	}
	m := &Memory{cfg: c, words: make([]uint64, c.Words), Last: -1}
	k := bits.Len(uint(c.Words - 1)) // a 1-word memory has no address bits
	for i := range k {
		m.addr = append(m.addr, fmt.Sprintf("%s_%d", c.Addr, i))
	}
	for i := range c.Width {
		m.data = append(m.data, fmt.Sprintf("%s_%d", c.Data, i))
	}
	if c.Init != "" {
		if read == nil {
			return nil, fmt.Errorf("%s: cannot read init file %q here", where, c.Init)
		}
		b, err := read(c.Init)
		if err != nil {
			return nil, fmt.Errorf("%s: init file: %w", where, err)
		}
		m.Load(b)
	}
	return m, nil
}

// Name returns the device's name.
func (m *Memory) Name() string { return m.cfg.Name }

// Kind returns "memory".
func (m *Memory) Kind() string { return "memory" }

// Config returns the memory's configuration.
func (m *Memory) Config() MemoryConfig { return m.cfg }

// Words returns the memory's contents (shared, not a copy).
func (m *Memory) Words() []uint64 { return m.words }

// AddrBits is the width of the address bus.
func (m *Memory) AddrBits() int { return len(m.addr) }

// BytesPerWord is how many bytes a word takes in init files.
func (m *Memory) BytesPerWord() int { return (m.cfg.Width + 7) / 8 }

// Mask is the bits a word can hold.
func (m *Memory) Mask() uint64 {
	if m.cfg.Width == 64 {
		return ^uint64(0)
	}
	return 1<<m.cfg.Width - 1
}

// Load fills the memory from raw little-endian words, from address 0.
// Missing bytes leave words at 0; extra bytes are ignored.
func (m *Memory) Load(b []byte) {
	n := m.BytesPerWord()
	for i := range m.words {
		var w uint64
		for j := range n {
			if p := i*n + j; p < len(b) {
				w |= uint64(b[p]) << (8 * j)
			}
		}
		m.words[i] = w & m.Mask()
	}
}

// Attach checks that the address, data, read and write nets exist.
func (m *Memory) Attach(bus Bus) error {
	var missing []string
	for _, n := range append(append(append([]string{}, m.addr...), m.data...), m.cfg.Read, m.cfg.Write) {
		if !bus.Has(n) {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("memory %q: missing nets %s", m.cfg.Name, strings.Join(missing, ", "))
	}
	return nil
}

func (m *Memory) level(bus Bus, name, role string) (Level, error) {
	l := bus.Value(name)
	if l != High && l != Low {
		return l, fmt.Errorf("memory %q: %s %s is %v", m.cfg.Name, role, name, l)
	}
	return l, nil
}

func (m *Memory) number(bus Bus, names []string, role string) (uint64, error) {
	var v uint64
	for i, n := range names {
		l, err := m.level(bus, n, role)
		if err != nil {
			return 0, err
		}
		if l == High {
			v |= 1 << i
		}
	}
	return v, nil
}

// pinning reports whether this memory currently pins any data bit.
func (m *Memory) pinning(bus Bus) bool {
	for _, n := range m.data {
		if bus.Pinned(n) != Floating {
			return true
		}
	}
	return false
}

func (m *Memory) release(bus Bus) bool {
	if !m.pinning(bus) {
		return false
	}
	for _, n := range m.data {
		bus.Unpin(n)
	}
	return true
}

// control reads a read or write line: floating counts as low.
func (m *Memory) control(bus Bus, name, role string) (bool, error) {
	switch bus.Value(name) {
	case High:
		return true, nil
	case Unstable:
		return false, fmt.Errorf("memory %q: %s %s is unstable", m.cfg.Name, role, name)
	}
	return false, nil
}

// pin drives data to v and reports whether any pin changed.
func (m *Memory) pin(bus Bus, v uint64) bool {
	changed := false
	for i, n := range m.data {
		want := Low
		if v>>i&1 == 1 {
			want = High
		}
		if bus.Pinned(n) != want {
			bus.Pin(n, want)
			changed = true
		}
	}
	return changed
}

// Service implements Device.
func (m *Memory) Service(bus Bus) (bool, error) {
	read, err := m.control(bus, m.cfg.Read, "read")
	if err != nil {
		return false, err
	}
	write, err := m.control(bus, m.cfg.Write, "write")
	if err != nil {
		return false, err
	}
	if !read && !write {
		return m.release(bus), nil
	}
	addr, err := m.number(bus, m.addr, "address")
	if err != nil {
		return false, err
	}
	if addr >= uint64(len(m.words)) {
		return false, fmt.Errorf("memory %q: address %d out of range (%d words)", m.cfg.Name, addr, len(m.words))
	}
	m.Last, m.LastWrite = int(addr), write
	if write && m.cfg.ReadOnly {
		return false, fmt.Errorf("memory %q: write to read-only memory at address %d", m.cfg.Name, addr)
	}

	switch {
	case read && write:
		// The short: store what the bus settled to, then drive it.
		if m.pinning(bus) {
			v, err := m.number(bus, m.data, "data")
			if err != nil {
				return false, err
			}
			m.words[addr] = v
		}
		return m.pin(bus, m.words[addr]), nil
	case write:
		if m.release(bus) {
			return true, nil // let the circuit alone drive the bus first
		}
		v, err := m.number(bus, m.data, "data")
		if err != nil {
			return false, err
		}
		m.words[addr] = v
		return false, nil
	}
	return m.pin(bus, m.words[addr]), nil
}
