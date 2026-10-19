package main

import (
	"fmt"
	"image"

	"github.com/Castux/mipsim2/bitmap"
)

// Leaf gates, drawn in cells. Inputs come in from the left edge, the output
// leaves from the right edge, and nothing else touches the edges, so the
// router can bring wires right up to a gate without touching it.

var kinds = map[byte]bitmap.Kind{'#': bitmap.Wire, 'H': bitmap.Power, 'L': bitmap.Ground, 'T': bitmap.Transistor, 'B': bitmap.Bridge}

// fromRows makes a component from rows of cells.
func fromRows(id string, rows []string) *Def {
	w := 0
	for _, r := range rows {
		w = max(w, len(r))
	}
	d := newDef(id, w, len(rows))
	for y, r := range rows {
		for x := range len(r) {
			if k, ok := kinds[r[x]]; ok {
				d.cells[image.Pt(x, y)] = k
			}
		}
	}
	return d
}

func inputName(k, n int) string {
	if n == 1 {
		return "input"
	}
	return fmt.Sprintf("input_%d", k+1)
}

// nand returns an n-input NAND gate: a power cell, then n transistors in
// series to ground. With one input it is an inverter.
func (l *Library) nand(n int) *Def {
	id := fmt.Sprintf("nand_%d", n)
	if n == 1 {
		id = "inverter"
	}
	if d, ok := l.get(id); ok {
		return d
	}
	// Inputs four rows apart, so wires reaching them have room to cross
	// one another.
	rows := []string{".....", "..H##"}
	for k := range n {
		rows = append(rows, "##T..")
		if k < n-1 {
			rows = append(rows, "..#..", "..#..", "..#..")
		}
	}
	rows = append(rows, "..L..", ".....")
	d := fromRows(id, rows)
	for k := range n {
		d.port(inputName(k, n), image.Pt(0, 2+4*k), false)
	}
	d.port("output", image.Pt(4, 1), false)
	return l.add(d)
}

// nor returns an n-input NOR gate: a power cell on an output spine, with
// one transistor per input from the spine to ground.
func (l *Library) nor(n int) *Def {
	id := fmt.Sprintf("nor_%d", n)
	if d, ok := l.get(id); ok {
		return d
	}
	rows := []string{".......", "....H##"}
	for k := range n {
		rows = append(rows, "..###..", "##T.#..", "..L.#..", "....#..")
		if k == n-1 {
			rows[len(rows)-3] = "##T...."
			rows[len(rows)-2] = "..L...."
			rows[len(rows)-1] = "......."
		}
	}
	d := fromRows(id, rows)
	for k := range n {
		d.port(inputName(k, n), image.Pt(0, 3+4*k), false)
	}
	d.port("output", image.Pt(6, 1), false)
	return l.add(d)
}

// busDriver is an open drain: it pulls the bus low when enable is high and
// data_inverted is high (the data is 0). The bus has one pull-up elsewhere.
func (l *Library) busDriver() *Def {
	if d, ok := l.get("bus_driver"); ok {
		return d
	}
	d := fromRows("bus_driver", []string{
		".....",
		"..###",
		"##T..",
		"..#..",
		"##T..",
		"..L..",
		".....",
	})
	d.port("enable", image.Pt(0, 2), false)
	d.port("data_inverted", image.Pt(0, 4), false)
	d.port("bus", image.Pt(4, 1), false)
	return l.add(d)
}
