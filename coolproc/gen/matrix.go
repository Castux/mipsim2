package main

// The layout engine: a circuit of NOR gates becomes a gate matrix (a
// Weinberger array), the regular style of NMOS control logic.
//
// Every signal is a vertical line. Every gate is a horizontal output row,
// pulled up by a power cell at its left end, with one transistor per input
// hanging below (or above) it to a ground row. Crossings are bridges. The
// geometry is fixed, so the layout is correct by construction:
//
//	columns: the line of the signal on track t is at x = 3t; a transistor
//	gated by it sits at x = 3t+1, and x = 3t+2 stays empty in transistor
//	rows.
//
//	rows, per slot of two gate rows (six rows):
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
// bridge needs. Lines in different blocks share a track when their block
// ranges do not overlap; one empty row between blocks keeps their ends
// apart.
//
// Lines are grouped by unit (a register, the adder...), so the bit slices
// look alike. A gate reading a signal of another unit reads it from a tap:
// a line in its own unit, joined to the driver's output row, which is
// extended to reach it. A folded block (the control) is placed differently:
// see Fold.

import (
	"fmt"
	"image"
	"sort"

	"github.com/Castux/mipsim2/bitmap"
)

// Signal is a gate's output, or an input driven from outside (a pin, a
// memory).
type Signal struct {
	Name       string
	Unit       string // lines of one unit sit together ("": any free track)
	driver     *Gate
	readers    []*Gate
	External   bool // driven from outside
	track      int
	firstBlock int // the blocks the line runs through
	lastBlock  int
	order      int // creation order, for a stable layout
	// tapOf is the signal a tap carries; taps are the signal's taps.
	tapOf *Signal
	taps  []*Signal
	// In a folded block, a local line runs only from its first gate's slot
	// to its last's.
	folded      bool
	firstSlot   int
	lastSlot    int
	gateCount   int // gates it touches
	gatesPlaced int // of those, already placed
}

// Gate is a NOR gate. With no inputs it is a constant high.
type Gate struct {
	Output     *Signal
	Inputs     []*Signal
	Block      int
	inputLines []*Signal // the inputs as read: the signals or their taps
	order      int       // creation order
	row        int       // gate row within the block (two per slot)
	left       int       // footprint in cells, inclusive
	right      int
}

// signals returns the gate's output and the lines it reads.
func (g *Gate) signals() []*Signal { return append([]*Signal{g.Output}, g.inputLines...) }

// Design is a NOR-gate circuit, with a block and a unit for every gate.
type Design struct {
	signals      []*Signal
	byName       map[string]*Signal
	gates        []*Gate
	units        []string // left to right
	unitIndex    map[string]int
	blocks       []string // top to bottom
	foldedBlocks map[int]bool
	withoutTaps  map[string]bool
}

func NewDesign() *Design {
	return &Design{byName: map[string]*Signal{}, unitIndex: map[string]int{}, foldedBlocks: map[int]bool{}, withoutTaps: map[string]bool{}}
}

// Unit declares a group of lines kept together; units are laid out left to
// right in declaration order.
func (d *Design) Unit(name string) {
	if _, duplicate := d.unitIndex[name]; duplicate {
		panic("duplicate unit " + name)
	}
	d.unitIndex[name] = len(d.units)
	d.units = append(d.units, name)
}

// Block declares a band of rows, laid out top to bottom in declaration
// order, and returns its index.
func (d *Design) Block(name string) int {
	d.blocks = append(d.blocks, name)
	return len(d.blocks) - 1
}

// Fold lays a block out as irregular logic: its gates are placed by a sweep
// from the top, and a line used only in the block runs just from its first
// gate to its last, so lines share tracks. Its slots have two spacer rows,
// one where lines end and one where they start, so line ends never touch.
func (d *Design) Fold(block int) { d.foldedBlocks[block] = true }

// NoTap makes the gates of a unit read other units' signals directly,
// without taps (for a unit of irregular logic in a folded block).
func (d *Design) NoTap(unit string) { d.withoutTaps[unit] = true }

// Signal returns the signal called name, creating it.
func (d *Design) Signal(name string) *Signal {
	if s, ok := d.byName[name]; ok {
		return s
	}
	s := &Signal{Name: name, order: len(d.signals)}
	d.signals = append(d.signals, s)
	d.byName[name] = s
	return s
}

// External declares an input driven from outside, with its line in unit.
func (d *Design) External(name, unit string) *Signal {
	s := d.Signal(name)
	s.External, s.Unit = true, unit
	return s
}

// NOR adds a gate output = NOR(inputs) in block, with its output line in
// unit.
func (d *Design) NOR(block int, unit, output string, inputs ...string) *Signal {
	out := d.Signal(output)
	if out.driver != nil || out.External {
		panic("signal driven twice: " + output)
	}
	if _, ok := d.unitIndex[unit]; !ok && unit != "" {
		panic("unknown unit " + unit)
	}
	g := &Gate{Output: out, Block: block, order: len(d.gates)}
	seen := map[string]bool{}
	for _, name := range inputs {
		if name == output {
			panic("gate reads its own output: " + output)
		}
		if seen[name] {
			continue // a repeated input is one transistor
		}
		seen[name] = true
		in := d.Signal(name)
		g.Inputs = append(g.Inputs, in)
		in.readers = append(in.readers, g)
	}
	out.driver, out.Unit = g, unit
	d.gates = append(d.gates, g)
	return out
}

// Not is an inverter: a NOR of one input.
func (d *Design) Not(block int, unit, output, input string) *Signal {
	return d.NOR(block, unit, output, input)
}

// Check reports signals that are used but never driven.
func (d *Design) Check() error {
	for _, s := range d.signals {
		if s.driver == nil && !s.External {
			return fmt.Errorf("signal %s is never driven", s.Name)
		}
	}
	return nil
}

// Layout is the drawn matrix.
type Layout struct {
	Cells      map[image.Point]bitmap.Kind
	Labels     map[image.Point]string
	Bounds     image.Rectangle
	Tracks     int
	gateRowY   [][]int  // per block, the y of each gate row
	blockLineY [][2]int // per block, the first and last row of its lines
	slotTopY   [][]int  // per block, the y of each slot's first row
}

const (
	cellsPerTrack = 3
	slotRows      = 6
)

// trackColumn is the x of a track's line.
func trackColumn(track int) int { return cellsPerTrack * track }

// interval is a range of blocks, or of cells, inclusive.
type interval struct{ first, last int }

func (a interval) overlaps(b interval) bool { return a.first <= b.last && b.first <= a.last }

// LayOut places every signal on a track and every gate in a row, then
// draws.
func (d *Design) LayOut() (*Layout, error) {
	if err := d.Check(); err != nil {
		return nil, err
	}
	all := d.addTaps()
	if err := d.findBlockRanges(all); err != nil {
		return nil, err
	}
	tracks := d.assignUnitTracks(all)
	tracks = d.assignFreeTracks(all, tracks)

	gatesOf := make([][]*Gate, len(d.blocks))
	for _, g := range d.gates {
		gatesOf[g.Block] = append(gatesOf[g.Block], g)
	}
	gateRows := make([]int, len(d.blocks))
	for block := range d.blocks {
		if d.foldedBlocks[block] {
			gateRows[block] = d.placeFolded(block, gatesOf[block], all)
		}
	}
	for _, s := range all {
		tracks = max(tracks, s.track+1)
	}
	for _, g := range d.gates {
		g.left, g.right = g.footprint(func(s *Signal) int { return s.track })
	}
	for block, gates := range gatesOf {
		if !d.foldedBlocks[block] {
			gateRows[block] = placeInRows(gates)
		}
	}
	layout := &Layout{Cells: map[image.Point]bitmap.Kind{}, Labels: map[image.Point]string{}, Tracks: tracks}
	d.measureBlocks(layout, gateRows)
	d.draw(layout, all, gatesOf)
	return layout, nil
}

// addTaps gives every gate the lines it reads: a signal of another unit is
// read from a tap in the gate's own unit. It returns every line: the
// signals, then the taps.
func (d *Design) addTaps() []*Signal {
	all := append([]*Signal(nil), d.signals...)
	for _, s := range d.signals {
		s.taps = nil
	}
	for _, g := range d.gates {
		g.inputLines = make([]*Signal, len(g.Inputs))
		unit := g.Output.Unit
		for k, in := range g.Inputs {
			g.inputLines[k] = in
			if unit == "" || d.withoutTaps[unit] || in.Unit == "" || in.Unit == unit || in.driver == nil {
				continue
			}
			var tap *Signal
			for _, t := range in.taps {
				if t.Unit == unit {
					tap = t
				}
			}
			if tap == nil {
				tap = &Signal{Name: in.Name + "@" + unit, Unit: unit, tapOf: in, order: in.order}
				in.taps = append(in.taps, tap)
				all = append(all, tap)
			}
			g.inputLines[k] = tap
		}
	}
	return all
}

// findBlockRanges finds the blocks each line runs through: from its first
// gate's to its last's (for a tap, its signal's driver counts too).
func (d *Design) findBlockRanges(all []*Signal) error {
	readers := map[*Signal][]*Gate{}
	for _, g := range d.gates {
		for _, line := range g.inputLines {
			readers[line] = append(readers[line], g)
		}
	}
	for _, s := range all {
		s.firstBlock, s.lastBlock = 1<<30, -1
		use := func(block int) {
			s.firstBlock, s.lastBlock = min(s.firstBlock, block), max(s.lastBlock, block)
		}
		if s.driver != nil {
			use(s.driver.Block)
		}
		if s.tapOf != nil {
			use(s.tapOf.driver.Block)
		}
		for _, g := range readers[s] {
			use(g.Block)
		}
		if s.lastBlock < 0 {
			return fmt.Errorf("signal %s is not used anywhere", s.Name)
		}
		s.folded = s.tapOf == nil && s.firstBlock == s.lastBlock && d.foldedBlocks[s.firstBlock]
	}
	return nil
}

// assignUnitTracks gives tracks to the lines of each unit, unit by unit,
// packing lines whose blocks do not overlap onto the same track (left
// edge). It returns the number of tracks used.
func (d *Design) assignUnitTracks(all []*Signal) int {
	tracks := 0
	for _, unit := range d.units {
		var lines []*Signal
		for _, s := range all {
			if s.Unit == unit && !s.folded {
				lines = append(lines, s)
			}
		}
		sort.SliceStable(lines, func(i, j int) bool {
			if lines[i].firstBlock != lines[j].firstBlock {
				return lines[i].firstBlock < lines[j].firstBlock
			}
			return lines[i].order < lines[j].order
		})
		var lastBlockOn []int // per track of this unit, the last block in use
		for _, s := range lines {
			k := -1
			for i, last := range lastBlockOn {
				if last < s.firstBlock {
					k = i
					break
				}
			}
			if k < 0 {
				k = len(lastBlockOn)
				lastBlockOn = append(lastBlockOn, -1)
			}
			lastBlockOn[k] = s.lastBlock
			s.track = tracks + k
		}
		tracks += len(lastBlockOn)
	}
	return tracks
}

// assignFreeTracks gives the lines of no unit the free track nearest the
// lines they connect to (those already placed), so their gates stay short.
// It returns the number of tracks used.
func (d *Design) assignFreeTracks(all []*Signal, tracks int) int {
	used := make([][]interval, tracks) // per track, the blocks in use
	placed := map[*Signal]bool{}
	for _, s := range all {
		if s.Unit != "" && !s.folded {
			used[s.track] = append(used[s.track], interval{s.firstBlock, s.lastBlock})
			placed[s] = true
		}
	}
	free := func(track int, s *Signal) bool {
		for _, in := range used[track] {
			if in.overlaps(interval{s.firstBlock, s.lastBlock}) {
				return false
			}
		}
		return true
	}
	for _, s := range d.signals {
		if s.Unit != "" || s.folded {
			continue
		}
		sum, count := 0, 0
		near := func(o *Signal) {
			if placed[o] {
				sum += o.track
				count++
			}
		}
		if s.driver != nil {
			for _, in := range s.driver.Inputs {
				near(in)
			}
		}
		for _, g := range s.readers {
			near(g.Output)
			for _, in := range g.Inputs {
				near(in)
			}
		}
		preferred := 0
		if count > 0 {
			preferred = sum / count
		}
		s.track = -1
		for distance := 0; distance <= len(used) && s.track < 0; distance++ {
			for _, track := range []int{preferred - distance, preferred + distance} {
				if track >= 0 && track < len(used) && free(track, s) {
					s.track = track
					break
				}
			}
		}
		if s.track < 0 {
			s.track = len(used)
			used = append(used, nil)
		}
		used[s.track] = append(used[s.track], interval{s.firstBlock, s.lastBlock})
		placed[s] = true
	}
	return len(used)
}

// fits reports whether a footprint fits in a row, with one empty cell
// between footprints.
func fits(row []interval, footprint interval) bool {
	for _, other := range row {
		if footprint.overlaps(interval{other.first - 1, other.last + 1}) {
			return false
		}
	}
	return true
}

// placeInRows puts a block's gates in gate rows, first fit, by their left
// end. It returns the number of rows.
func placeInRows(gates []*Gate) int {
	sort.SliceStable(gates, func(i, j int) bool { return gates[i].left < gates[j].left })
	var rows [][]interval
	for _, g := range gates {
		footprint := interval{g.left, g.right}
		g.row = -1
		for r := range rows {
			if fits(rows[r], footprint) {
				rows[r] = append(rows[r], footprint)
				g.row = r
				break
			}
		}
		if g.row < 0 {
			rows = append(rows, []interval{footprint})
			g.row = len(rows) - 1
		}
	}
	return len(rows)
}

// measureBlocks finds the rows of each block: its lines' top row, its
// slots, then one empty row.
func (d *Design) measureBlocks(layout *Layout, gateRows []int) {
	layout.gateRowY = make([][]int, len(d.blocks))
	layout.blockLineY = make([][2]int, len(d.blocks))
	layout.slotTopY = make([][]int, len(d.blocks))
	y := 0
	for block := range d.blocks {
		period := slotRows
		if d.foldedBlocks[block] {
			period++ // a second spacer row
		}
		top := y
		slots := (gateRows[block] + 1) / 2
		firstSlot := top + 1
		for slot := range slots {
			layout.slotTopY[block] = append(layout.slotTopY[block], firstSlot+period*slot)
		}
		for row := range gateRows[block] {
			rowY := firstSlot + period*(row/2)
			if row%2 == 1 {
				rowY += 4 // the lower gate row of the slot
			}
			layout.gateRowY[block] = append(layout.gateRowY[block], rowY)
		}
		bottom := top + period*slots // the last spacer row (or top, if empty)
		layout.blockLineY[block] = [2]int{top, bottom}
		y = bottom + 2
	}
}

// draw puts the cells: lines, gate rows with their power cells,
// transistors, ground rows, and labels.
func (d *Design) draw(layout *Layout, all []*Signal, gatesOf [][]*Gate) {
	put := func(x, y int, k bitmap.Kind) { layout.Cells[image.Pt(x, y)] = k }
	isLine := map[image.Point]bool{}
	for _, s := range all {
		x := trackColumn(s.track)
		top, bottom := layout.blockLineY[s.firstBlock][0], layout.blockLineY[s.lastBlock][1]
		if s.folded {
			top, bottom = layout.slotTopY[s.firstBlock][s.firstSlot]-1, layout.slotTopY[s.firstBlock][s.lastSlot]+5
		}
		for y := top; y <= bottom; y++ {
			put(x, y, bitmap.Wire)
			isLine[image.Pt(x, y)] = true
		}
	}
	// horizontalRun draws a row from x0 to x1, bridging the lines it
	// crosses, except at its junctions.
	horizontalRun := func(x0, x1, y int, junctions map[int]bool) {
		for x := x0; x <= x1; x++ {
			p := image.Pt(x, y)
			if isLine[p] && !junctions[x] {
				if x == x0 || x == x1 {
					panic(fmt.Sprintf("row ends on a line at %v", p))
				}
				put(x, y, bitmap.Bridge)
				continue
			}
			put(x, y, bitmap.Wire)
		}
	}
	for block := range d.blocks {
		groundRows := map[int]*interval{} // per ground row's y, the transistors' x
		for _, g := range gatesOf[block] {
			rowY := layout.gateRowY[block][g.row]
			junctions := map[int]bool{trackColumn(g.Output.track): true}
			for _, tap := range g.Output.taps {
				junctions[trackColumn(tap.track)] = true
			}
			horizontalRun(g.left, g.right, rowY, junctions)
			put(g.left, rowY, bitmap.Power)
			transistorY, groundY := rowY+1, rowY+2
			if g.row%2 == 1 {
				transistorY, groundY = rowY-1, rowY-2
			}
			for _, line := range g.inputLines {
				x := trackColumn(line.track) + 1
				put(x, transistorY, bitmap.Transistor)
				span := groundRows[groundY]
				if span == nil {
					span = &interval{x, x}
					groundRows[groundY] = span
				}
				span.first, span.last = min(span.first, x), max(span.last, x)
			}
			layout.Labels[image.Pt(trackColumn(g.Output.track), rowY)] = g.Output.Name
		}
		for groundY, span := range groundRows {
			left := span.first - 2 // one cell left of the leftmost transistor's line
			horizontalRun(left, span.last, groundY, nil)
			put(left, groundY, bitmap.Ground)
		}
	}
	// External signals are labelled at the top of their line.
	for _, s := range d.signals {
		if s.External {
			y := layout.blockLineY[s.firstBlock][0]
			if s.folded {
				y = layout.slotTopY[s.firstBlock][s.firstSlot] - 1
			}
			layout.Labels[image.Pt(trackColumn(s.track), y)] = s.Name
		}
	}
	for p := range layout.Cells {
		layout.Bounds = layout.Bounds.Union(image.Rectangle{Min: p, Max: p.Add(image.Pt(1, 1))})
	}
}

// footprint is the gate's extent in cells, given the tracks of its lines:
// from its power cell, left of the leftmost line, to its rightmost
// transistor (3t+1) or output junction (3t).
func (g *Gate) footprint(trackOf func(*Signal) int) (int, int) {
	leftmost, right := trackOf(g.Output), trackColumn(trackOf(g.Output))
	for _, tap := range g.Output.taps {
		leftmost, right = min(leftmost, trackOf(tap)), max(right, trackColumn(trackOf(tap)))
	}
	for _, line := range g.inputLines {
		leftmost = min(leftmost, trackOf(line))
		right = max(right, trackColumn(trackOf(line))+1)
	}
	return trackColumn(leftmost) - 1, right
}

// placeFolded places the gates of a folded block slot by slot from the top,
// and gives tracks to its local lines, sharing a track between lines whose
// slots are at least one slot apart. It returns the number of gate rows.
func (d *Design) placeFolded(block int, gates []*Gate, all []*Signal) int {
	passing := map[int]bool{} // tracks of lines passing through the block
	width := 0                // stay within the lines laid out already when possible
	for _, s := range all {
		if !s.folded && s.firstBlock <= block && block <= s.lastBlock {
			passing[s.track] = true
		}
		if !s.folded {
			width = max(width, s.track+1)
		}
	}
	for _, s := range all {
		if s.folded && s.firstBlock == block {
			s.track, s.firstSlot, s.lastSlot, s.gateCount, s.gatesPlaced = -1, -1, -1, 0, 0
		}
	}
	for _, g := range gates {
		for _, s := range g.signals() {
			if s.folded {
				s.gateCount++
			}
		}
	}
	holder := map[int]*Signal{} // the line running on a track
	freeFrom := map[int]int{}   // the slot from which a track is free again
	lastTrack := 0
	waiting := append([]*Gate(nil), gates...)
	slot := 0
	for len(waiting) > 0 {
		sortForFolding(waiting)
		var rows [2][]interval
		var later []*Gate
		for _, g := range waiting {
			newTracks := d.tracksForGate(g, slot, lastTrack, width, passing, holder, freeFrom)
			left, right := g.footprint(func(s *Signal) int {
				if track, ok := newTracks[s]; ok {
					return track
				}
				return s.track
			})
			footprint := interval{left, right}
			row := -1
			for r := range 2 {
				if fits(rows[r], footprint) {
					row = r
					break
				}
			}
			if row < 0 {
				later = append(later, g)
				continue
			}
			for s, track := range newTracks {
				s.track, s.firstSlot = track, slot
				holder[track] = s
				lastTrack = track
			}
			for _, s := range g.signals() {
				if s.folded {
					s.gatesPlaced++
				}
			}
			g.row = 2*slot + row
			rows[row] = append(rows[row], footprint)
		}
		// Lines whose gates are all placed end in this slot.
		for track, s := range holder {
			if s.gatesPlaced == s.gateCount {
				s.lastSlot = slot
				delete(holder, track)
				freeFrom[track] = slot + 2
			}
		}
		waiting = later
		slot++
	}
	return 2 * slot
}

// sortForFolding orders gates waiting for a slot: those that finish lines
// already running first, then the design's order.
func sortForFolding(gates []*Gate) {
	score := func(g *Gate) int {
		n := 0
		for _, s := range g.signals() {
			if s.folded && s.track >= 0 {
				n++
				if s.gatesPlaced+1 == s.gateCount {
					n += 2
				}
			}
		}
		return n
	}
	sort.SliceStable(gates, func(i, j int) bool {
		a, b := score(gates[i]), score(gates[j])
		if a != b {
			return a > b
		}
		return gates[i].order < gates[j].order
	})
}

// tracksForGate picks tracks for the gate's local lines that have none yet:
// free ones nearest the gate's other lines (or the last track given),
// within width first.
func (d *Design) tracksForGate(g *Gate, slot, lastTrack, width int, passing map[int]bool, holder map[int]*Signal, freeFrom map[int]int) map[*Signal]int {
	sum, count := 0, 0
	for _, s := range g.signals() {
		if s.track >= 0 {
			sum += s.track
			count++
		}
	}
	anchor := lastTrack
	if count > 0 {
		anchor = sum / count
	}
	anchor = min(anchor, max(width-1, 0))
	newTracks := map[*Signal]int{}
	taken := map[int]bool{}
	available := func(track int) bool {
		return !taken[track] && !passing[track] && holder[track] == nil && freeFrom[track] <= slot
	}
	for _, s := range g.signals() {
		if !s.folded || s.track >= 0 {
			continue
		}
		if _, done := newTracks[s]; done {
			continue
		}
		track := -1
		for distance := 0; distance < width && track < 0; distance++ {
			for _, candidate := range []int{anchor - distance, anchor + distance} {
				if candidate >= 0 && candidate < width && available(candidate) {
					track = candidate
					break
				}
			}
		}
		for candidate := width; track < 0; candidate++ {
			if available(candidate) {
				track = candidate
			}
		}
		newTracks[s], taken[track] = track, true
	}
	return newTracks
}
