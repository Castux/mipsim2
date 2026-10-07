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
	FlipThreshold int // 0 or less means DefaultFlipThreshold
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

	values []Value
	pins   []Value // Floating means not pinned
	// Every transistor gated by a net flips together, so flip counts are
	// kept per gate net (only for nets that gate something).
	flips   []int32
	flipped []netlist.NetID

	// Flat (CSR) copies of the netlist adjacency, for cache locality.
	chStart []int32 // net -> range in ch
	ch      []edge
	gStart  []int32 // net -> range in gated
	gated   []pair
	src     []uint8 // srcLow|srcHigh from drive, pinLow|pinHigh from pins

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

type edge struct{ gate, other netlist.NetID }
type pair struct{ a, b netlist.NetID }

const (
	srcLow uint8 = 1 << iota
	srcHigh
	pinLow
	pinHigh
)

// ErrNetlistHasErrors is returned by New when the netlist has compile errors.
var ErrNetlistHasErrors = errors.New("sim: netlist has compile errors")

// New returns a simulator in its initial state (see Reset).
func New(nl *netlist.Netlist, opts Options) (*Sim, error) {
	if nl.HasErrors() {
		return nil, ErrNetlistHasErrors
	}
	th := opts.FlipThreshold
	if th <= 0 {
		th = DefaultFlipThreshold
	}
	n := len(nl.Nets)
	s := &Sim{
		nl:        nl,
		threshold: int32(th),
		values:    make([]Value, n),
		pins:      make([]Value, n),
		flips:     make([]int32, n),
		mark:      make([]uint32, n),
		chStart:   make([]int32, n+1),
		gStart:    make([]int32, n+1),
		src:       make([]uint8, n),
	}
	for id := range n {
		for _, t := range nl.ChannelOf[id] {
			tr := &nl.Transistors[t]
			other := tr.A
			if other == netlist.NetID(id) {
				other = tr.B
			}
			s.ch = append(s.ch, edge{tr.Gate, other})
		}
		s.chStart[id+1] = int32(len(s.ch))
		for _, t := range nl.GatedBy[id] {
			tr := &nl.Transistors[t]
			s.gated = append(s.gated, pair{tr.A, tr.B})
		}
		s.gStart[id+1] = int32(len(s.gated))
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
	for id, net := range s.nl.Nets {
		switch net.Drive {
		case netlist.DriveLow:
			s.src[id] = srcLow
		case netlist.DriveHigh:
			s.src[id] = srcHigh
		default:
			s.src[id] = 0
		}
	}
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

// Steps returns the number of Step calls that did work since Reset began,
// including those of Reset's own initial settle.
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
	s.src[net] &^= pinLow | pinHigh
	if v == Low {
		s.src[net] |= pinLow
	} else {
		s.src[net] |= pinHigh
	}
	s.Update(net)
}

// Unpin releases a net and queues it.
func (s *Sim) Unpin(net netlist.NetID) {
	s.pins[net] = Floating
	s.src[net] &^= pinLow | pinHigh
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
		gs := s.gated[s.gStart[n]:s.gStart[n+1]]
		if len(gs) > 0 {
			if s.flips[n] == 0 {
				s.flipped = append(s.flipped, n)
			}
			s.flips[n]++
		}
		for _, p := range gs {
			s.queue = append(s.queue, p.a)
			if !isOn {
				// Turned off: both sides need re-evaluating. Turned on: they
				// are one group now, so one side is enough (v1's sd1/sd2 rule).
				s.queue = append(s.queue, p.b)
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
				s.OnChange(Change{Step: s.steps + 1, Net: n, Old: Unstable, New: Floating}) // part of the step about to run
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
	for _, n := range s.flipped {
		s.flips[n] = 0
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
		for _, e := range s.ch[s.chStart[n]:s.chStart[n+1]] {
			if s.values[e.gate] != High {
				continue
			}
			other := e.other
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
	var drives uint8
	for _, n := range group {
		drives |= s.src[n]
		if s.flips[n] > s.threshold {
			return Unstable
		}
	}
	switch {
	case drives&(srcLow|pinLow) != 0:
		return Low
	case drives&(srcHigh|pinHigh) != 0:
		return High
	}
	return Floating
}
