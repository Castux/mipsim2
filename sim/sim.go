// Package sim is the switch-level simulator: breadth-first update over a
// compiled netlist, pins, Step and Settle, and unstable detection. It knows
// nothing about pixels or graphics. See docs/SPEC.md, "Simulation model".
package sim

import (
	"errors"
	"slices"

	"github.com/Castux/mipsim2/netlist"
)

// Value is the state of a net.
type Value uint8

const (
	Floating Value = iota
	High
	Low
	Unstable
)

func (v Value) String() string {
	return [...]string{"floating", "high", "low", "unstable"}[v]
}

// DefaultFlipThreshold is how many times a transistor may switch within one
// settle before its gate's group is declared Unstable ("above" the threshold,
// as in v1, so the 21st flip trips it).
const DefaultFlipThreshold = 20

// Options configures a simulator.
type Options struct {
	FlipThreshold int // 0 means DefaultFlipThreshold
}

// Change is one net value change, reported to the OnChange hook.
type Change struct {
	Step     int // number of Step calls so far, including the current one
	Net      netlist.NetID
	Old, New Value
}

// Sim simulates one netlist.
type Sim struct {
	nl        *netlist.Netlist
	threshold int32

	values  []Value
	pins    []Value // Floating means not pinned
	flips   []int32
	flipped []int32 // transistors with a non-zero flip count

	queue []netlist.NetID
	head  int
	idle  bool // the queue was empty after the last step: the next step starts a settle

	unstable []netlist.NetID // nets that became Unstable during this settle

	// Flood-fill scratch.
	mark  []uint32
	gen   uint32
	group []netlist.NetID

	steps int

	// OnChange, if set, is called for every net value change, in order.
	OnChange func(Change)
}

// ErrNetlistHasErrors is returned by New when the netlist has compile errors.
var ErrNetlistHasErrors = errors.New("sim: netlist has compile errors")

// New returns a simulator in its initial state (see Reset).
func New(nl *netlist.Netlist, opts Options) (*Sim, error) {
	if nl.HasErrors() {
		return nil, ErrNetlistHasErrors
	}
	th := opts.FlipThreshold
	if th == 0 {
		th = DefaultFlipThreshold
	}
	n := len(nl.Nets)
	s := &Sim{
		nl:        nl,
		threshold: int32(th),
		values:    make([]Value, n),
		pins:      make([]Value, n),
		flips:     make([]int32, len(nl.Transistors)),
		mark:      make([]uint32, n),
	}
	s.Reset()
	return s, nil
}

// Netlist returns the simulated netlist.
func (s *Sim) Netlist() *netlist.Netlist { return s.nl }

// Reset clears all pins and state, sets every net Floating, pushes every
// source net in ascending ID order and settles, as v1's setup does.
func (s *Sim) Reset() {
	clear(s.values)
	clear(s.pins)
	clear(s.flips)
	s.flipped = s.flipped[:0]
	s.queue, s.head = s.queue[:0], 0
	s.unstable = s.unstable[:0]
	s.idle = true
	s.steps = 0
	for id, net := range s.nl.Nets {
		if net.Drive != netlist.DriveNone {
			s.Update(netlist.NetID(id))
		}
	}
	s.Settle()
}

// Value returns the current value of a net.
func (s *Sim) Value(net netlist.NetID) Value { return s.values[net] }

// Pinned returns High or Low if the net is pinned, else Floating.
func (s *Sim) Pinned(net netlist.NetID) Value { return s.pins[net] }

// Conducting reports whether transistor t currently conducts.
func (s *Sim) Conducting(t int) bool { return s.values[s.nl.Transistors[t].Gate] == High }

// Steps returns the number of Step calls that did work since Reset.
func (s *Sim) Steps() int { return s.steps }

// Pending returns the number of queued nets.
func (s *Sim) Pending() int { return len(s.queue) - s.head }

// Update queues a net for re-evaluation. Nothing is evaluated until Step or
// Settle.
func (s *Sim) Update(net netlist.NetID) {
	s.queue = append(s.queue, net)
}

// Pin forces a net High or Low and queues it. Pinned nets act as sources;
// low still wins within a group.
func (s *Sim) Pin(net netlist.NetID, v Value) {
	if v != High && v != Low {
		panic("sim: Pin value must be High or Low")
	}
	s.pins[net] = v
	s.Update(net)
}

// Unpin releases a net and queues it.
func (s *Sim) Unpin(net netlist.NetID) {
	s.pins[net] = Floating
	s.Update(net)
}

// Settle steps until the queue is empty and returns the number of steps.
func (s *Sim) Settle() int {
	n := 0
	for s.Step() {
		n++
	}
	return n
}

// Step processes one queued net and reports whether there was one.
func (s *Sim) Step() bool {
	if s.head == len(s.queue) {
		return false
	}
	if s.idle {
		s.beginSettle()
	}
	start := s.queue[s.head]
	s.head++
	s.steps++

	group := s.flood(start)
	v := s.groupValue(group)

	for _, n := range group {
		prev := s.values[n]
		if prev == Unstable || prev == v {
			continue
		}
		s.values[n] = v
		if s.OnChange != nil {
			s.OnChange(Change{Step: s.steps, Net: n, Old: prev, New: v})
		}
		if v == Unstable {
			s.unstable = append(s.unstable, n)
			continue // its transistors stop conducting; channels are not pushed (v1)
		}
		wasOn, isOn := prev == High, v == High
		if wasOn == isOn {
			continue
		}
		for _, t := range s.nl.GatedBy[n] {
			if s.flips[t] == 0 {
				s.flipped = append(s.flipped, t)
			}
			s.flips[t]++
			tr := &s.nl.Transistors[t]
			s.Update(tr.A)
			if !isOn {
				// Turned off: both sides need re-evaluating. Turned on: they
				// are one group now, so one side is enough (v1's sd1/sd2 rule).
				s.Update(tr.B)
			}
		}
	}

	if s.head == len(s.queue) {
		s.endSettle()
	}
	return true
}

// beginSettle releases nets left Unstable by the previous settle so they are
// recomputed: a real oscillator goes Unstable again, a glitch recovers.
func (s *Sim) beginSettle() {
	s.idle = false
	if len(s.unstable) == 0 {
		return
	}
	slices.Sort(s.unstable)
	for _, n := range slices.Compact(s.unstable) {
		if s.values[n] == Unstable {
			s.values[n] = Floating
			if s.OnChange != nil {
				s.OnChange(Change{Step: s.steps, Net: n, Old: Unstable, New: Floating})
			}
			s.queue = append(s.queue, n)
			// When n went Unstable its transistors stopped conducting without
			// their channels being re-evaluated (v1's rule), so those may hold
			// stale values. Queue them too.
			for _, t := range s.nl.GatedBy[n] {
				tr := &s.nl.Transistors[t]
				s.queue = append(s.queue, tr.A, tr.B)
			}
		}
	}
	s.unstable = s.unstable[:0]
}

func (s *Sim) endSettle() {
	s.queue, s.head = s.queue[:0], 0
	for _, t := range s.flipped {
		s.flips[t] = 0
	}
	s.flipped = s.flipped[:0]
	s.idle = true
}

// flood returns the group of start: the nets reachable through conducting
// transistors, in breadth-first order (deterministic: adjacency lists are in
// ascending transistor order).
func (s *Sim) flood(start netlist.NetID) []netlist.NetID {
	s.gen++
	if s.gen == 0 { // wrapped: clear stale marks
		clear(s.mark)
		s.gen = 1
	}
	g := s.group[:0]
	g = append(g, start)
	s.mark[start] = s.gen
	for i := 0; i < len(g); i++ {
		n := g[i]
		for _, t := range s.nl.ChannelOf[n] {
			tr := &s.nl.Transistors[t]
			if s.values[tr.Gate] != High {
				continue
			}
			other := tr.A
			if other == n {
				other = tr.B
			}
			if s.mark[other] != s.gen {
				s.mark[other] = s.gen
				g = append(g, other)
			}
		}
	}
	s.group = g
	return g
}

// groupValue is Low if the group has a low source or a net pinned low, else
// High if it has a high source or a net pinned high, else Floating; and
// Unstable if any transistor gated by the group has flipped too often.
func (s *Sim) groupValue(group []netlist.NetID) Value {
	v := Floating
	for _, n := range group {
		if s.nl.Nets[n].Drive == netlist.DriveLow || s.pins[n] == Low {
			v = Low
			break
		}
		if s.nl.Nets[n].Drive == netlist.DriveHigh || s.pins[n] == High {
			v = High
		}
	}
	for _, n := range group {
		for _, t := range s.nl.GatedBy[n] {
			if s.flips[t] > s.threshold {
				return Unstable
			}
		}
	}
	return v
}
