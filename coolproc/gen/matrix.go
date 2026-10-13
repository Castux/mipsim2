package main

// The layout engine: a circuit of NOR gates becomes a gate matrix (a
// Weinberger array), the regular style of NMOS control logic.
//
// Every signal is a vertical line. Every gate is a horizontal output row,
// pulled up by a power cell at its left end, with one transistor per input
// hanging below (or above) it to a ground row. Crossings are bridges. The
// geometry is fixed, so the layout is correct by construction:
//
//	columns: signal track t is the line at x = 3t; a transistor gated by it
//	sits at x = 3t+1, and x = 3t+2 stays empty in transistor rows.
//
//	rows, per slot of two gate rows (period 6):
//	  y0    output rows of the upper gates   H B##B##+##  (+ junction)
//	  y0+1  their transistors                 #T #  #T
//	  y0+2  ground row, shared               LB##B##B#
//	  y0+3  transistors of the lower gates    #T #  #
//	  y0+4  output rows of the lower gates
//	  y0+5  spacer: lines only
//
// Signals live in blocks (horizontal bands, such as one bit of the
// datapath). A line spans every row of the blocks from its first to its
// last use, so it always passes straight through the rows it crosses, as a
// bridge needs. Lines in different blocks share a column when their block
// ranges do not overlap; one empty row between blocks keeps their ends
// apart.

import (
	"fmt"
	"image"
	"sort"

	"github.com/Castux/mipsim2/bitmap"
)

// Sig is a signal: a gate's output, or an input driven from outside (a pin,
// a device).
type Sig struct {
	Name  string
	Unit  string // column group: lines of one unit sit together ("": any free track)
	drv   *Gate
	users []*Gate
	Ext   bool  // driven from outside
	Block int   // for an external signal, a block it must reach (-1: none)
	col   int   // track
	lo    int   // first block of the line
	hi    int   // last block of the line
	order int   // creation order, for stable layouts
	extra []int // more blocks the line must reach
}

// Gate is a NOR gate. With no inputs it is a constant high.
type Gate struct {
	Out   *Sig
	In    []*Sig
	Block int
	row   int // gate row within the block (2 per slot)
	xl    int // footprint in cells, inclusive
	xr    int
}

// Design is a NOR-gate circuit with a block and unit for every gate.
type Design struct {
	sigs   []*Sig
	byName map[string]*Sig
	gates  []*Gate
	units  []string // left to right
	unitOf map[string]int
	blocks []string // top to bottom
}

func NewDesign() *Design {
	return &Design{byName: map[string]*Sig{}, unitOf: map[string]int{}}
}

// Unit declares a column group; units are laid out left to right in
// declaration order.
func (d *Design) Unit(name string) {
	if _, ok := d.unitOf[name]; ok {
		panic("duplicate unit " + name)
	}
	d.unitOf[name] = len(d.units)
	d.units = append(d.units, name)
}

// Block declares a band of rows, laid out top to bottom in declaration
// order, and returns its index.
func (d *Design) Block(name string) int {
	d.blocks = append(d.blocks, name)
	return len(d.blocks) - 1
}

// S returns the signal called name, creating it.
func (d *Design) S(name string) *Sig {
	if s, ok := d.byName[name]; ok {
		return s
	}
	s := &Sig{Name: name, Block: -1, order: len(d.sigs)}
	d.sigs = append(d.sigs, s)
	d.byName[name] = s
	return s
}

// Has reports whether a signal exists.
func (d *Design) Has(name string) bool {
	_, ok := d.byName[name]
	return ok
}

// Ext declares an input driven from outside, with its line in unit and
// reaching block.
func (d *Design) Ext(name, unit string, block int) *Sig {
	s := d.S(name)
	s.Ext, s.Unit, s.Block = true, unit, block
	return s
}

// Reach makes a signal's line also reach block, for example so that a
// label or a probe sits where it is easy to find.
func (d *Design) Reach(name string, block int) {
	s := d.S(name)
	s.extra = append(s.extra, block)
}

// NOR adds a gate out = NOR(ins) in block, with its output line in unit.
func (d *Design) NOR(block int, unit, out string, ins ...string) *Sig {
	o := d.S(out)
	if o.drv != nil || o.Ext {
		panic("signal driven twice: " + out)
	}
	if _, ok := d.unitOf[unit]; !ok && unit != "" {
		panic("unknown unit " + unit)
	}
	g := &Gate{Out: o, Block: block}
	seen := map[string]bool{}
	for _, in := range ins {
		if in == out {
			panic("gate reads its own output: " + out)
		}
		if seen[in] {
			continue // a repeated input is one transistor
		}
		seen[in] = true
		s := d.S(in)
		g.In = append(g.In, s)
		s.users = append(s.users, g)
	}
	o.drv, o.Unit = g, unit
	d.gates = append(d.gates, g)
	return o
}

// Not is an inverter: a NOR of one input.
func (d *Design) Not(block int, unit, out, in string) *Sig {
	return d.NOR(block, unit, out, in)
}

// Check reports signals that are used but never driven.
func (d *Design) Check() error {
	for _, s := range d.sigs {
		if s.drv == nil && !s.Ext {
			return fmt.Errorf("signal %s is never driven", s.Name)
		}
	}
	return nil
}

// Layout is the drawn matrix.
type Layout struct {
	Cells  map[image.Point]bitmap.Kind
	Labels map[image.Point]string
	Bounds image.Rectangle
	Tracks int
	rowY   [][]int // per block, the y of each gate row
	lineY  [][2]int
}

const (
	pitch    = 3 // cells per track
	slotRows = 6
)

func trackX(t int) int { return pitch * t }

// Lay places every signal on a track and every gate in a row, then draws.
func (d *Design) Lay() (*Layout, error) {
	if err := d.Check(); err != nil {
		return nil, err
	}
	// Block ranges of lines.
	for _, s := range d.sigs {
		s.lo, s.hi = 1<<30, -1
		use := func(b int) {
			s.lo, s.hi = min(s.lo, b), max(s.hi, b)
		}
		if s.drv != nil {
			use(s.drv.Block)
		}
		if s.Block >= 0 {
			use(s.Block)
		}
		for _, g := range s.users {
			use(g.Block)
		}
		for _, b := range s.extra {
			use(b)
		}
		if s.hi < 0 {
			return nil, fmt.Errorf("signal %s is not used anywhere", s.Name)
		}
	}

	// Tracks: unit by unit, left-edge packing on block ranges.
	tracks := 0
	for _, u := range d.units {
		var ss []*Sig
		for _, s := range d.sigs {
			if s.Unit == u {
				ss = append(ss, s)
			}
		}
		sort.SliceStable(ss, func(i, j int) bool {
			if ss[i].lo != ss[j].lo {
				return ss[i].lo < ss[j].lo
			}
			return ss[i].order < ss[j].order
		})
		var last []int // per track of this unit, the last block used
		for _, s := range ss {
			k := -1
			for i, h := range last {
				if h < s.lo {
					k = i
					break
				}
			}
			if k < 0 {
				k = len(last)
				last = append(last, -1)
			}
			last[k] = s.hi
			s.col = tracks + k
		}
		tracks += len(last)
	}
	// Signals of no unit take the free track nearest the signals they
	// connect to (those already placed), so their gates stay short.
	used := make([][][2]int, tracks)
	placed := map[*Sig]bool{}
	for _, s := range d.sigs {
		if s.Unit != "" {
			used[s.col] = append(used[s.col], [2]int{s.lo, s.hi})
			placed[s] = true
		}
	}
	free := func(t int, s *Sig) bool {
		for _, iv := range used[t] {
			if s.lo <= iv[1] && iv[0] <= s.hi {
				return false
			}
		}
		return true
	}
	for _, s := range d.sigs {
		if s.Unit != "" {
			continue
		}
		sum, n := 0, 0
		near := func(o *Sig) {
			if placed[o] {
				sum += o.col
				n++
			}
		}
		if s.drv != nil {
			for _, in := range s.drv.In {
				near(in)
			}
		}
		for _, g := range s.users {
			near(g.Out)
			for _, in := range g.In {
				near(in)
			}
		}
		pref := 0
		if n > 0 {
			pref = sum / n
		}
		s.col = -1
		for dist := 0; dist <= len(used) && s.col < 0; dist++ {
			for _, t := range []int{pref - dist, pref + dist} {
				if t >= 0 && t < len(used) && free(t, s) {
					s.col = t
					break
				}
			}
		}
		if s.col < 0 {
			s.col = len(used)
			used = append(used, nil)
		}
		used[s.col] = append(used[s.col], [2]int{s.lo, s.hi})
		placed[s] = true
	}
	tracks = len(used)

	// Gate footprints in cells: the power cell at x = 3*tmin-1, to the
	// rightmost transistor (3t+1) or the output junction (3t).
	for _, g := range d.gates {
		tmin, xr := g.Out.col, trackX(g.Out.col)
		for _, s := range g.In {
			tmin = min(tmin, s.col)
			xr = max(xr, trackX(s.col)+1)
		}
		g.xl, g.xr = trackX(tmin)-1, xr
	}

	// Rows: first fit, per block, gates sorted by their left end.
	lay := &Layout{Cells: map[image.Point]bitmap.Kind{}, Labels: map[image.Point]string{}, Tracks: tracks}
	nrows := make([]int, len(d.blocks))
	byBlock := make([][]*Gate, len(d.blocks))
	for _, g := range d.gates {
		byBlock[g.Block] = append(byBlock[g.Block], g)
	}
	for b, gs := range byBlock {
		sort.SliceStable(gs, func(i, j int) bool { return gs[i].xl < gs[j].xl })
		var rows [][][2]int
		for _, g := range gs {
			placed := false
			for r := range rows {
				ok := true
				for _, iv := range rows[r] {
					// One empty cell between footprints.
					if g.xl <= iv[1]+1 && iv[0] <= g.xr+1 {
						ok = false
						break
					}
				}
				if ok {
					rows[r] = append(rows[r], [2]int{g.xl, g.xr})
					g.row = r
					placed = true
					break
				}
			}
			if !placed {
				rows = append(rows, [][2]int{{g.xl, g.xr}})
				g.row = len(rows) - 1
			}
		}
		nrows[b] = len(rows)
	}

	// Block geometry: line top row, slots, then one empty row.
	lay.rowY = make([][]int, len(d.blocks))
	lay.lineY = make([][2]int, len(d.blocks))
	y := 0
	for b := range d.blocks {
		top := y
		slots := (nrows[b] + 1) / 2
		y0 := top + 1
		for r := range nrows[b] {
			ry := y0 + slotRows*(r/2)
			if r%2 == 1 {
				ry += 4
			}
			lay.rowY[b] = append(lay.rowY[b], ry)
		}
		bot := top + slotRows*slots // the last spacer row (or top, if empty)
		lay.lineY[b] = [2]int{top, bot}
		y = bot + 2
	}

	put := func(x, y int, k bitmap.Kind) { lay.Cells[image.Pt(x, y)] = k }
	isLine := map[image.Point]bool{}
	// Lines.
	for _, s := range d.sigs {
		x := trackX(s.col)
		for yy := lay.lineY[s.lo][0]; yy <= lay.lineY[s.hi][1]; yy++ {
			put(x, yy, bitmap.Wire)
			isLine[image.Pt(x, yy)] = true
		}
	}
	// hrow draws a horizontal run, bridging the lines it crosses except
	// at the junction column (-1 for none).
	hrow := func(x0, x1, y, junction int) {
		for x := x0; x <= x1; x++ {
			p := image.Pt(x, y)
			if isLine[p] && x != junction {
				if x == x0 || x == x1 {
					panic(fmt.Sprintf("row ends on a line at %v", p))
				}
				put(x, y, bitmap.Bridge)
				continue
			}
			put(x, y, bitmap.Wire)
		}
	}
	for b := range d.blocks {
		// Ground rows: one per slot, spanning its transistors.
		type span struct{ lo, hi int }
		grounds := map[int]*span{}
		for _, g := range byBlock[b] {
			ry := lay.rowY[b][g.row]
			hrow(g.xl, g.xr, ry, trackX(g.Out.col))
			put(g.xl, ry, bitmap.Power)
			ty, gy := ry+1, ry+2
			if g.row%2 == 1 {
				ty, gy = ry-1, ry-2
			}
			for _, s := range g.In {
				x := trackX(s.col) + 1
				put(x, ty, bitmap.Transistor)
				sp := grounds[gy]
				if sp == nil {
					sp = &span{x, x}
					grounds[gy] = sp
				}
				sp.lo, sp.hi = min(sp.lo, x), max(sp.hi, x)
			}
			lay.Labels[image.Pt(trackX(g.Out.col), ry)] = g.Out.Name
		}
		for gy, sp := range grounds {
			x0 := sp.lo - 2 // one cell left of the leftmost transistor's line
			hrow(x0, sp.hi, gy, -1)
			put(x0, gy, bitmap.Ground)
		}
	}
	// Labels of external signals: at the top of their line.
	for _, s := range d.sigs {
		if s.Ext {
			lay.Labels[image.Pt(trackX(s.col), lay.lineY[s.lo][0])] = s.Name
		}
	}
	for p := range lay.Cells {
		lay.Bounds = lay.Bounds.Union(image.Rectangle{Min: p, Max: p.Add(image.Pt(1, 1))})
	}
	return lay, nil
}
