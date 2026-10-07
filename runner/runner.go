// Package runner drives a simulation: clock, settle loop, device servicing,
// named nets and numbers, and traces. The CLI, editor and tests all use it.
package runner

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Castux/mipsim2/netlist"
	"github.com/Castux/mipsim2/sim"
)

// DefaultClock is the name of the clock net when Options.Clock is empty.
const DefaultClock = "clock"

// Options configures a runner.
type Options struct {
	Clock         string // name of the clock net; DefaultClock if empty
	FlipThreshold int    // passed to the simulator; 0 for its default
}

// Runner owns a simulator and drives it by name.
type Runner struct {
	sim *sim.Sim
	nl  *netlist.Netlist

	numbers map[string][]netlist.NetID // bus base name -> bits, LSB first (lookups only)
	names   []string                   // every net name and bus name, sorted

	clock     netlist.NetID
	clockHigh bool
	halfTicks int
}

var busBit = regexp.MustCompile(`^(.+)_(\d+)$`)

// New compiles nothing: it takes a netlist without errors, starts the
// simulator, and pins the clock low if the circuit has one.
func New(nl *netlist.Netlist, opts Options) (*Runner, error) {
	s, err := sim.New(nl, sim.Options{FlipThreshold: opts.FlipThreshold})
	if err != nil {
		return nil, err
	}
	r := &Runner{sim: s, nl: nl, numbers: map[string][]netlist.NetID{}, clock: netlist.NoNet}

	// Collect names in net order (deterministic), then buses from name_N.
	type bit struct {
		i  int
		id netlist.NetID
	}
	bits := map[string][]bit{}
	var bases []string
	for id, n := range nl.Nets {
		for _, name := range n.Names {
			r.names = append(r.names, name)
			m := busBit.FindStringSubmatch(name)
			if m == nil {
				continue
			}
			i, err := strconv.Atoi(m[2])
			if err != nil || i > 63 {
				continue
			}
			if _, seen := bits[m[1]]; !seen {
				bases = append(bases, m[1])
			}
			bits[m[1]] = append(bits[m[1]], bit{i, netlist.NetID(id)})
		}
	}
	for _, base := range bases {
		bs := bits[base]
		width := 0
		for _, b := range bs {
			width = max(width, b.i+1)
		}
		ids := make([]netlist.NetID, width)
		for i := range ids {
			ids[i] = netlist.NoNet
		}
		for _, b := range bs {
			ids[b.i] = b.id
		}
		r.numbers[base] = ids
		if _, isNet := nl.Lookup(base); !isNet {
			r.names = append(r.names, base)
		}
	}
	slices.Sort(r.names)

	name := opts.Clock
	if name == "" {
		name = DefaultClock
	}
	if id, ok := nl.Lookup(name); ok {
		r.clock = id
		s.Pin(id, sim.Low)
		s.Settle()
	}
	return r, nil
}

// Sim returns the underlying simulator.
func (r *Runner) Sim() *sim.Sim { return r.sim }

// Netlist returns the simulated netlist.
func (r *Runner) Netlist() *netlist.Netlist { return r.nl }

// Names returns every net name and bus name, sorted.
func (r *Runner) Names() []string { return r.names }

// HasClock reports whether the circuit has a clock net.
func (r *Runner) HasClock() bool { return r.clock != netlist.NoNet }

// Ticks returns the number of full clock periods run so far.
func (r *Runner) Ticks() int { return r.halfTicks / 2 }

// Bus returns the bits of bus name (name_0 first). Missing bits are NoNet.
func (r *Runner) Bus(name string) ([]netlist.NetID, bool) {
	ids, ok := r.numbers[name]
	return ids, ok
}

// Settle runs the simulator until stable. Devices are serviced here from M8.
func (r *Runner) Settle() {
	r.sim.Settle()
}

// HalfTick toggles the clock and settles.
func (r *Runner) HalfTick() error {
	if r.clock == netlist.NoNet {
		return fmt.Errorf("the circuit has no clock net")
	}
	r.clockHigh = !r.clockHigh
	v := sim.Low
	if r.clockHigh {
		v = sim.High
	}
	r.sim.Pin(r.clock, v)
	r.Settle()
	r.halfTicks++
	return nil
}

// Tick runs n full clock periods (rising then falling edge).
func (r *Runner) Tick(n int) error {
	for range 2 * n {
		if err := r.HalfTick(); err != nil {
			return err
		}
	}
	return nil
}

func parseLevel(s string) (sim.Value, bool) {
	switch strings.ToLower(s) {
	case "high", "1", "h":
		return sim.High, true
	case "low", "0", "l":
		return sim.Low, true
	case "float", "floating", "z":
		return sim.Floating, true
	}
	return 0, false
}

// Set pins a net or a bus by name. A net takes high, low or float (also 1, 0,
// z); a bus takes a number (decimal, or 0x/0b prefixed) or float to release
// every bit. Set does not settle.
func (r *Runner) Set(name, value string) error {
	if id, ok := r.nl.Lookup(name); ok {
		v, ok := parseLevel(value)
		if !ok {
			return fmt.Errorf("%s: want high, low or float, got %q", name, value)
		}
		r.pin(id, v)
		return nil
	}
	ids, ok := r.numbers[name]
	if !ok {
		return fmt.Errorf("no net or bus named %q", name)
	}
	if v, ok := parseLevel(value); ok && v == sim.Floating {
		for _, id := range ids {
			if id != netlist.NoNet {
				r.sim.Unpin(id)
			}
		}
		return nil
	}
	n, err := strconv.ParseUint(value, 0, 64)
	if err != nil {
		return fmt.Errorf("%s: want a number, got %q", name, value)
	}
	if len(ids) < 64 && n>>len(ids) != 0 {
		return fmt.Errorf("%s: %d does not fit in %d bits", name, n, len(ids))
	}
	for i, id := range ids {
		if id == netlist.NoNet {
			return fmt.Errorf("%s: bit %d has no net", name, i)
		}
		if n>>i&1 == 1 {
			r.pin(id, sim.High)
		} else {
			r.pin(id, sim.Low)
		}
	}
	return nil
}

func (r *Runner) pin(id netlist.NetID, v sim.Value) {
	if v == sim.Floating {
		r.sim.Unpin(id)
	} else {
		r.sim.Pin(id, v)
	}
}

// Value returns the value of a single net by name.
func (r *Runner) Value(name string) (sim.Value, error) {
	id, ok := r.nl.Lookup(name)
	if !ok {
		return 0, fmt.Errorf("no net named %q", name)
	}
	return r.sim.Value(id), nil
}

// Number reads a bus as an unsigned number. It fails if a bit is not High
// or Low.
func (r *Runner) Number(name string) (uint64, error) {
	ids, ok := r.numbers[name]
	if !ok {
		return 0, fmt.Errorf("no bus named %q", name)
	}
	var n uint64
	for i, id := range ids {
		if id == netlist.NoNet {
			return 0, fmt.Errorf("%s: bit %d has no net", name, i)
		}
		switch v := r.sim.Value(id); v {
		case sim.High:
			n |= 1 << i
		case sim.Low:
		default:
			return 0, fmt.Errorf("%s: bit %d is %v", name, i, v)
		}
	}
	return n, nil
}

// Format renders a net (0, 1, z for floating, x for unstable) or a bus (a
// decimal number, or ? if any bit is not a clean level) for display.
func (r *Runner) Format(name string) string {
	if id, ok := r.nl.Lookup(name); ok {
		return [...]string{"z", "1", "0", "x"}[r.sim.Value(id)]
	}
	if _, ok := r.numbers[name]; ok {
		n, err := r.Number(name)
		if err != nil {
			return "?"
		}
		return strconv.FormatUint(n, 10)
	}
	return "-"
}
