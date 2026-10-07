package netlist

import (
	"cmp"
	"fmt"
	"image"
	"slices"

	"github.com/Castux/mipsim2/bitmap"
	"github.com/Castux/mipsim2/doc"
)

// compiler holds per-pixel working data, indexed by the grid's raster index.
type compiler struct {
	opts  Options
	g     *bitmap.Dense
	n     int
	xs    []int32
	ys    []int32
	diags []Diagnostic

	northOf, southOf []int32 // vertical neighbour indices, -1 if off

	region  []int32 // thick region of each pixel, or -1
	regions []thickRegion
	source  []Role // RoleHigh, RoleLow or RoleOff for each pixel
	ringOf  []int32
	rings   []image.Point // top-left of each ring
	centre  []bool        // transistor centre
	thick   []bool        // pixel of an E_THICK region (thin-wires variant)
}

type thickRegion struct {
	rect   image.Rectangle // covered pixels
	blocks int
	source bool // a valid 3×3 high source
}

type transistorPx struct {
	centre, gate, a, b int // grid indices
}

type bridgePx struct {
	pos            image.Point
	n, e, s, w     int // grid indices of the arms
	armIsCentre    bool
	nsJoin, weJoin bool
}

// CompileDoc flattens and compiles a document.
func CompileDoc(d *doc.Document, opts Options) *Netlist {
	return Compile(d.Flatten(), opts)
}

// MaxArea bounds the bounding box of a compiled circuit, in pixels (about
// 50 MB of compiler grids). 16384 x 16384 is far beyond a processor.
const MaxArea = 1 << 28

// Compile classifies the flat document's pixels and builds the netlist.
func Compile(flat *doc.Flat, opts Options) *Netlist {
	// A margin of 2 keeps every neighbour lookup inside the grid.
	r := flat.Pixels.Bounds().Inset(-2)
	if area := int64(r.Dx()) * int64(r.Dy()); area > MaxArea {
		// The compiler works on a dense grid over the bounding box; two
		// pixels far apart would need gigabytes. Refuse instead.
		nl := Compile(&doc.Flat{Pixels: bitmap.New()}, opts)
		nl.Diagnostics = append(nl.Diagnostics, Diagnostic{Code: "E_TOO_LARGE", Level: Error, Pos: r.Min,
			Msg: fmt.Sprintf("the circuit spans %dx%d pixels, above the limit of %d pixels in area; move far-off pixels closer", r.Dx(), r.Dy(), MaxArea)})
		return nl
	}
	c := &compiler{opts: opts, g: flat.Pixels.ToDense(r)}
	c.n = c.g.Count()
	c.xs = make([]int32, c.n)
	c.ys = make([]int32, c.n)
	c.g.ForEach(func(i, x, y int) { c.xs[i], c.ys[i] = int32(x), int32(y) })
	// Vertical neighbours, computed once: rank lookups dominate otherwise.
	c.southOf = make([]int32, c.n)
	c.northOf = make([]int32, c.n)
	for i := range c.northOf {
		c.northOf[i] = -1
	}
	for i := range c.n {
		s := c.g.Index(int(c.xs[i]), int(c.ys[i])+1)
		c.southOf[i] = int32(s)
		if s >= 0 {
			c.northOf[s] = int32(i)
		}
	}

	c.findThickRegions()
	c.findRings()
	if opts.Variant == IsolatedSources {
		c.checkSourceBorders()
	}
	ts := c.findTransistors()
	bs := c.findBridges()
	return c.build(flat, ts, bs)
}

func (c *compiler) on(x, y int) bool     { return c.g.Get(x, y) }
func (c *compiler) idx(x, y int) int     { return c.g.Index(x, y) }
func (c *compiler) pos(i int) (int, int) { return int(c.xs[i]), int(c.ys[i]) }

// Neighbour indices of pixel i, or -1 if that neighbour is off. They accept
// -1 and return -1, so they chain: c.south(c.east(i)) is the pixel at (+1,+1)
// if (+1,0) is on. East and west are adjacent in raster order.
func (c *compiler) east(i int) int {
	if i >= 0 && i+1 < c.n && c.ys[i+1] == c.ys[i] && c.xs[i+1] == c.xs[i]+1 {
		return i + 1
	}
	return -1
}

func (c *compiler) west(i int) int {
	if i > 0 && c.ys[i-1] == c.ys[i] && c.xs[i-1] == c.xs[i]-1 {
		return i - 1
	}
	return -1
}

func (c *compiler) north(i int) int {
	if i < 0 {
		return -1
	}
	return int(c.northOf[i])
}

func (c *compiler) south(i int) int {
	if i < 0 {
		return -1
	}
	return int(c.southOf[i])
}

// neighbours returns the orthogonal neighbours in the order N, E, S, W.
func (c *compiler) neighbours(i int) [4]int {
	return [4]int{c.north(i), c.east(i), c.south(i), c.west(i)}
}

func (c *compiler) diag(code string, level Level, p image.Point, format string, args ...any) {
	c.diags = append(c.diags, Diagnostic{Code: code, Level: level, Pos: p, Msg: fmt.Sprintf(format, args...)})
}

// Step 1: union overlapping 2×2 blocks into thick regions.
func (c *compiler) findThickRegions() {
	parent := make(unionFind, c.n) // over block top-left pixels; -1 if not a block
	for i := range parent {
		parent[i] = -1
	}
	find, union := parent.find, parent.union

	var blocks []int32
	for i := range c.n {
		if c.east(i) < 0 || c.south(c.east(i)) < 0 || c.south(i) < 0 {
			continue
		}
		parent[i] = int32(i)
		blocks = append(blocks, int32(i))
		// Earlier blocks (in raster order) that overlap this one, by their
		// top-left pixel: (-1,-1), (0,-1), (+1,-1), (-1,0). Each such block
		// contains a pixel adjacent to this one, so the chains find it.
		n := c.north(i)
		for _, j := range [4]int{c.west(n), n, c.north(c.east(i)), c.west(i)} {
			if j >= 0 && parent[j] >= 0 {
				union(int32(i), int32(j))
			}
		}
	}

	c.region = make([]int32, c.n)
	for i := range c.region {
		c.region[i] = -1
	}
	regionOfRoot := make([]int32, c.n) // region ID + 1, by union-find root; 0 = none yet
	for _, b := range blocks {
		root := find(b)
		id := regionOfRoot[root] - 1
		if id < 0 {
			id = int32(len(c.regions))
			regionOfRoot[root] = id + 1
			x, y := c.pos(int(b))
			c.regions = append(c.regions, thickRegion{rect: image.Rect(x, y, x+2, y+2)})
		}
		reg := &c.regions[id]
		x, y := c.pos(int(b))
		reg.rect = reg.rect.Union(image.Rect(x, y, x+2, y+2))
		reg.blocks++
		s := c.south(int(b))
		for _, p := range [4]int{int(b), c.east(int(b)), s, c.east(s)} {
			c.region[p] = id
		}
	}

	c.source = make([]Role, c.n)
	c.thick = make([]bool, c.n)
	for id := range c.regions {
		reg := &c.regions[id]
		reg.source = reg.rect.Dx() == 3 && reg.rect.Dy() == 3 && reg.blocks == 4
		if !reg.source && c.opts.Variant == ThinWires {
			c.diag("E_THICK", Error, reg.rect.Min,
				"thick region %dx%d is not a 3×3 high source; wires must be one pixel wide", reg.rect.Dx(), reg.rect.Dy())
		}
	}
	for i := range c.n {
		id := c.region[i]
		if id < 0 {
			continue
		}
		if c.regions[id].source {
			c.source[i] = RoleHigh
		} else if c.opts.Variant == ThinWires {
			c.thick[i] = true
		}
	}
}

// Step 2: 3×3 rings with an off centre are low sources.
func (c *compiler) findRings() {
	c.ringOf = make([]int32, c.n)
	for i := range c.ringOf {
		c.ringOf[i] = -1
	}
	ring := int32(0)
	for i := range c.n {
		// i is the candidate top-left; walk the ring through neighbours.
		top1 := c.east(i)
		top2 := c.east(top1)
		if top2 < 0 || c.south(top1) >= 0 { // centre must be off
			continue
		}
		mid0, mid2 := c.south(i), c.south(top2)
		bot0 := c.south(mid0)
		bot1 := c.east(bot0)
		bot2 := c.east(bot1)
		if mid2 < 0 || bot2 < 0 {
			continue
		}
		px := [8]int{i, top1, top2, mid0, mid2, bot0, bot1, bot2}
		x, y := c.pos(i)
		if c.opts.Variant == IsolatedSources {
			// Inside thick wire, a 3×3 hole is just a hole.
			inThick := false
			for _, p := range px {
				if c.region[p] >= 0 {
					inThick = true
				}
			}
			if inThick {
				continue
			}
		}
		centre := image.Pt(x+1, y+1)
		overlap := false
		for _, p := range px {
			if c.ringOf[p] >= 0 || c.source[p] == RoleHigh {
				overlap = true
			}
		}
		if overlap {
			c.diag("E_RING_OVERLAP", Error, centre, "low source ring overlaps another ring or a high source")
		}
		for _, p := range px {
			c.ringOf[p] = ring
			if c.source[p] != RoleHigh {
				c.source[p] = RoleLow
			}
		}
		c.rings = append(c.rings, image.Pt(x, y))
		ring++
	}
}

// borderCells lists the 16 cells around a 3×3 square at offset (0,0), in
// cyclic order starting at the top-left corner (-1,-1).
var borderCells = func() []image.Point {
	var cells []image.Point
	for x := -1; x <= 3; x++ {
		cells = append(cells, image.Pt(x, -1))
	}
	for y := 0; y <= 3; y++ {
		cells = append(cells, image.Pt(3, y))
	}
	for x := 2; x >= -1; x-- {
		cells = append(cells, image.Pt(x, 3))
	}
	for y := 2; y >= 0; y-- {
		cells = append(cells, image.Pt(-1, y))
	}
	return cells
}()

// checkSourceBorders implements the isolated-sources variant: the cells
// around a source are off except single-pixel attachments on its sides, and
// no two attachments touch, even diagonally.
func (c *compiler) checkSourceBorders() {
	check := func(tl image.Point, kind string) {
		var on []image.Point
		bad := false
		for _, d := range borderCells {
			p := tl.Add(d)
			if !c.on(p.X, p.Y) {
				continue
			}
			corner := (d.X == -1 || d.X == 3) && (d.Y == -1 || d.Y == 3)
			if corner {
				bad = true
			}
			for _, q := range on {
				if abs(q.X-p.X) <= 1 && abs(q.Y-p.Y) <= 1 {
					bad = true
				}
			}
			on = append(on, p)
		}
		if bad {
			c.diag("E_SOURCE_BORDER", Error, tl,
				"%s source needs an empty border except single, separate wire attachments", kind)
		}
	}
	for _, reg := range c.regions {
		if reg.source {
			check(reg.rect.Min, "high")
		}
	}
	for _, tl := range c.rings {
		check(tl, "low")
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Step 3: on pixels with exactly three on orthogonal neighbours.
func (c *compiler) findTransistors() []transistorPx {
	c.centre = make([]bool, c.n)
	var ts []transistorPx
	for i := range c.n {
		if c.source[i] != RoleOff || c.region[i] >= 0 {
			continue
		}
		arm := c.neighbours(i)
		count, missing := 0, -1
		for k := range arm {
			if arm[k] >= 0 {
				count++
			} else {
				missing = k
			}
		}
		if count != 3 {
			continue
		}
		if c.opts.Variant == IsolatedSources {
			thickArm := false
			for _, a := range arm {
				if a >= 0 && c.region[a] >= 0 {
					thickArm = true
				}
			}
			if thickArm {
				continue
			}
		}
		gate := (missing + 2) % 4
		// Channel ends perpendicular to the gate axis, in raster order:
		// west then east, or north then south.
		var a, b int
		if gate%2 == 0 { // gate north or south: channel west–east
			a, b = arm[3], arm[1]
		} else {
			a, b = arm[0], arm[2]
		}
		c.centre[i] = true
		ts = append(ts, transistorPx{centre: i, gate: arm[gate], a: a, b: b})
	}
	for _, t := range ts {
		for _, a := range [3]int{t.gate, t.a, t.b} {
			if c.centre[a] && a > t.centre {
				x, y := c.pos(a)
				c.diag("E_ADJ_TRANSISTOR", Error, image.Pt(x, y), "transistor centre touches another transistor centre")
			}
		}
	}
	return ts
}

// Step 4: off pixels with four on orthogonal and no on diagonal neighbours.
func (c *compiler) findBridges() []bridgePx {
	var bs []bridgePx
	for i := range c.n {
		if c.south(i) >= 0 { // i is a candidate north arm; the gap below must be off
			continue
		}
		x, y := c.pos(i)
		gx, gy := x, y+1
		s, w, e := c.idx(gx, gy+1), c.idx(gx-1, gy), c.idx(gx+1, gy)
		if s < 0 || w < 0 || e < 0 {
			continue
		}
		diagonals := 0
		for _, d := range [4]image.Point{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
			if c.on(gx+d.X, gy+d.Y) {
				diagonals++
			}
		}
		gap := image.Pt(gx, gy)
		switch diagonals {
		case 0:
			b := bridgePx{pos: gap, n: i, e: e, s: s, w: w, nsJoin: true, weJoin: true}
			for _, a := range [4]int{i, e, s, w} {
				if c.centre[a] {
					b.armIsCentre = true
				}
			}
			if b.armIsCentre {
				c.diag("E_BRIDGE_ARM", Error, gap, "a bridge arm is a transistor centre")
				b.nsJoin = !c.centre[i] && !c.centre[s]
				b.weJoin = !c.centre[w] && !c.centre[e]
			}
			bs = append(bs, b)
		case 4:
			// The centre of a low source ring.
		default:
			c.diag("E_GAP_AMBIGUOUS", Error, gap,
				"bridge-like gap with %d on diagonal pixel(s), which would short its arms", diagonals)
		}
	}
	return bs
}

// build unions pixels into nets, names them and runs the remaining lint rules.
func (c *compiler) build(flat *doc.Flat, ts []transistorPx, bs []bridgePx) *Netlist {
	parent := newUnionFind(c.n)
	find := parent.find
	union := func(a, b int) { parent.union(int32(a), int32(b)) }
	for i := range c.n {
		if c.centre[i] {
			continue
		}
		if j := c.east(i); j >= 0 && !c.centre[j] {
			union(i, j)
		}
		if j := c.south(i); j >= 0 && !c.centre[j] {
			union(i, j)
		}
	}
	for _, b := range bs {
		if b.nsJoin {
			union(b.n, b.s)
		}
		if b.weJoin {
			union(b.w, b.e)
		}
	}

	nl := &Netlist{names: map[string]NetID{}, grid: c.g, bridges: bitmap.New()}
	nl.roles = make([]Role, c.n)
	nl.pixNet = make([]NetID, c.n)
	netOfRoot := make([]NetID, c.n)
	for i := range netOfRoot {
		netOfRoot[i] = NoNet
	}
	hasHigh, hasLow := []bool{}, []bool{}
	for i := range c.n {
		switch {
		case c.centre[i]:
			nl.roles[i] = RoleTransistor
		case c.source[i] != RoleOff:
			nl.roles[i] = c.source[i]
		case c.thick[i]:
			nl.roles[i] = RoleThick
		default:
			nl.roles[i] = RoleWire
		}
		if c.centre[i] {
			nl.pixNet[i] = NoNet
			continue
		}
		root := find(int32(i))
		id := netOfRoot[root]
		if id == NoNet {
			id = NetID(len(nl.Nets))
			netOfRoot[root] = id
			x, y := c.pos(i)
			nl.Nets = append(nl.Nets, Net{First: image.Pt(x, y)})
			hasHigh, hasLow = append(hasHigh, false), append(hasLow, false)
		}
		nl.pixNet[i] = id
		switch c.source[i] {
		case RoleHigh:
			hasHigh[id] = true
		case RoleLow:
			hasLow[id] = true
		}
	}
	for id := range nl.Nets {
		switch {
		case hasLow[id]:
			nl.Nets[id].Drive = DriveLow
			if hasHigh[id] {
				c.diag("W_SOURCE_SHORT", Warning, nl.Nets[id].First, "net contains both a high and a low source; it is always low")
			}
		case hasHigh[id]:
			nl.Nets[id].Drive = DriveHigh
		}
	}
	for _, b := range bs {
		nl.bridges.Set(b.pos.X, b.pos.Y, true)
	}

	nl.GatedBy = make([][]int32, len(nl.Nets))
	nl.ChannelOf = make([][]int32, len(nl.Nets))
	for k, t := range ts {
		x, y := c.pos(t.centre)
		tr := Transistor{Gate: nl.pixNet[t.gate], A: nl.pixNet[t.a], B: nl.pixNet[t.b], Pos: image.Pt(x, y)}
		nl.Transistors = append(nl.Transistors, tr)
		if tr.Gate != NoNet {
			nl.GatedBy[tr.Gate] = append(nl.GatedBy[tr.Gate], int32(k))
		}
		for _, ch := range [2]NetID{tr.A, tr.B} {
			if ch != NoNet {
				nl.ChannelOf[ch] = append(nl.ChannelOf[ch], int32(k))
			}
		}
	}

	c.applyLabels(nl, flat)
	c.transistorLints(nl)
	c.diagonalLint(nl)

	slices.SortStableFunc(c.diags, func(a, b Diagnostic) int {
		return cmp.Or(cmp.Compare(a.Pos.Y, b.Pos.Y), cmp.Compare(a.Pos.X, b.Pos.X), cmp.Compare(a.Code, b.Code))
	})
	nl.Diagnostics = c.diags
	return nl
}

func (c *compiler) applyLabels(nl *Netlist, flat *doc.Flat) {
	type scoped struct {
		net   NetID
		scope string
	}
	firstInScope := map[scoped]string{} // looked up only
	warned := map[scoped]bool{}
	for _, l := range flat.Labels {
		i := c.g.Index(l.Pos.X, l.Pos.Y)
		if i < 0 || c.centre[i] {
			c.diag("E_LABEL_OFF_NET", Error, l.Pos, "label %q is not on a wire or source pixel", l.Name)
			continue
		}
		net := nl.pixNet[i]
		if other, dup := nl.names[l.Name]; dup && other != net {
			c.diag("E_DUPLICATE_NAME", Error, l.Pos, "name %q is already used by another net", l.Name)
			continue
		}
		nl.names[l.Name] = net
		if !slices.Contains(nl.Nets[net].Names, l.Name) {
			nl.Nets[net].Names = append(nl.Nets[net].Names, l.Name)
		}
		key := scoped{net, l.Scope}
		if first, ok := firstInScope[key]; !ok {
			firstInScope[key] = l.Name
		} else if first != l.Name && !warned[key] {
			warned[key] = true
			c.diag("W_LABEL_CONFLICT", Warning, l.Pos, "net is labelled both %q and %q", first, l.Name)
		}
	}
}

func (c *compiler) transistorLints(nl *Netlist) {
	for _, t := range nl.Transistors {
		if t.Gate == NoNet || t.A == NoNet || t.B == NoNet {
			continue // already an error
		}
		if t.A == t.B {
			c.diag("W_CHANNEL_SHORT", Warning, t.Pos, "both channel ends are the same net")
		}
		if t.Gate == t.A || t.Gate == t.B {
			c.diag("W_GATE_ON_CHANNEL", Warning, t.Pos, "gate is the same net as a channel end")
		}
		g := nl.Nets[t.Gate]
		if g.Drive == DriveNone && len(g.Names) == 0 && len(nl.ChannelOf[t.Gate]) == 0 {
			c.diag("W_FLOATING_GATE", Warning, t.Pos, "gate net has no source, no label and no transistor driving it")
		}
	}
}

// diagonalLint warns about on pixels of different nets that touch only at a
// corner, except arms of one transistor or one bridge, which always do.
func (c *compiler) diagonalLint(nl *Netlist) {
	for i := range c.n {
		if c.centre[i] {
			continue
		}
		x, y := c.pos(i)
		for _, dx := range [2]int{-1, 1} {
			// The diagonal pixel, reached through an on neighbour when there
			// is one; otherwise both shared cells are off and a lookup is needed.
			side, j := c.east(i), -1
			if dx < 0 {
				side = c.west(i)
			}
			if side >= 0 {
				j = c.south(side)
			} else if s := c.south(i); s >= 0 {
				if dx < 0 {
					j = c.west(s)
				} else {
					j = c.east(s)
				}
			} else {
				j = c.idx(x+dx, y+1)
			}
			if j < 0 || c.centre[j] || nl.pixNet[i] == nl.pixNet[j] {
				continue
			}
			// The two cells orthogonally adjacent to both pixels.
			c1, c2 := image.Pt(x+dx, y), image.Pt(x, y+1)
			exempt := false
			for _, p := range [2]image.Point{c1, c2} {
				if k := c.idx(p.X, p.Y); k >= 0 && c.centre[k] {
					exempt = true
				}
				if nl.bridges.Get(p.X, p.Y) {
					exempt = true
				}
			}
			if !exempt {
				c.diag("W_DIAGONAL_TOUCH", Warning, image.Pt(x, y),
					"pixel touches pixel %d,%d of another net only diagonally; diagonals never connect", x+dx, y+1)
			}
		}
	}
}

// unionFind is a disjoint-set forest over pixel indices. The root of a set
// is its smallest member, so roots come out in raster order.
type unionFind []int32

func newUnionFind(n int) unionFind {
	u := make(unionFind, n)
	for i := range u {
		u[i] = int32(i)
	}
	return u
}

func (u unionFind) find(i int32) int32 {
	for u[i] != i {
		u[i] = u[u[i]] // path halving
		i = u[i]
	}
	return i
}

func (u unionFind) union(a, b int32) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u[max(ra, rb)] = min(ra, rb)
	}
}
