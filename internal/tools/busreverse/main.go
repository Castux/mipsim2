// Command busreverse writes a component that reverses the bit order of a
// bus without leaving its horizontal band.
//
//	go run ./internal/tools/busreverse [-bits 16] [-o file.mip]
//
// The bus has one wire every other row (rows 0, 2, ..., 2n-2), entering on
// the left and leaving on the right reversed. The band needs one spare track
// below the bus (rows 2n-1 and 2n): a wire can only change track by landing
// in a free slot, so each column is a jump of one wire into the free slot,
// crossing the wires in between on bridges. Each pair (k, n-1-k) swaps
// through the free slot in three jumps, so the component is (3n+1) wide and
// 2n+1 tall. Inputs are labelled in_0 ... in_{n-1} (top to bottom).
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Castux/mipsim2/bitmap"
	"github.com/Castux/mipsim2/doc"
)

func main() {
	bits := flag.Int("bits", 16, "bus width")
	out := flag.String("o", "", "output file (default: standard output)")
	flag.Parse()
	if *bits < 2 || *bits > 1024 {
		fmt.Fprintln(os.Stderr, "busreverse: -bits must be between 2 and 1024")
		os.Exit(2)
	}
	data, err := reverser(*bits).Save()
	if err != nil {
		fmt.Fprintln(os.Stderr, "busreverse:", err)
		os.Exit(1)
	}
	if *out == "" {
		os.Stdout.Write(data)
		return
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "busreverse:", err)
		os.Exit(1)
	}
}

type jump struct{ wire, from, to int } // slots; slot s is row 2s

// jumps returns the moves that reverse n wires with one free slot at n.
func jumps(n int) []jump {
	slot := make([]int, n)
	for w := range slot {
		slot[w] = w
	}
	free := n
	var js []jump
	move := func(w int) {
		js = append(js, jump{w, slot[w], free})
		slot[w], free = free, slot[w]
	}
	for k := range n / 2 {
		move(k)
		move(n - 1 - k)
		move(k)
	}
	return js
}

// reverser returns a document whose root holds one instance of the
// reversing component.
func reverser(n int) *doc.Document {
	js := jumps(n)
	w, h := 2*len(js)+1, 2*n+1
	def := doc.NewDefinition("rev", w, h)
	def.Name = fmt.Sprintf("%dbusReverse", n)
	px := def.Pixels
	slot := make([]int, n)
	for i := range slot {
		slot[i] = i
	}
	for m, j := range js {
		x := 2*m + 1 // the jumping wire's column
		for wire, s := range slot {
			px.Set(x-1, 2*s, true)
			if wire != j.wire {
				px.Set(x, 2*s, true)
			}
		}
		lo, hi := min(2*j.from, 2*j.to), max(2*j.from, 2*j.to)
		for y := lo; y <= hi; y++ {
			px.Set(x, y, true)
		}
		for wire, s := range slot { // wires in between cross on bridges
			if wire != j.wire && lo < 2*s && 2*s < hi {
				px.Put(x, 2*s, bitmap.Bridge)
			}
		}
		slot[j.wire] = j.to
	}
	for _, s := range slot {
		px.Set(w-1, 2*s, true)
	}
	for k := range n {
		def.Labels = append(def.Labels, doc.Label{X: 0, Y: 2 * k, Name: fmt.Sprintf("in_%d", k)})
	}
	d := doc.New()
	d.Note = fmt.Sprintf("reverses a %d-bit bus within its band, using one spare track below it (go run ./internal/tools/busreverse)", n)
	d.Defs[def.ID] = def
	d.RootDef().Instances = []doc.Instance{{ID: "i1", Def: def.ID, Name: "rev"}}
	return d
}
