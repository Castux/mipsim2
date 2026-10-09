package netlist

import (
	"cmp"
	"fmt"
	"image"
	"slices"

	"github.com/Castux/mipsim2/bitmap"
	"github.com/Castux/mipsim2/doc"
)

// compiler holds per-cell working data, indexed by the grid's raster index.
type compiler struct {
	opts  Options
	g     *bitmap.Dense
	n     int
	xs    []int32
	ys    []int32
	kind  []bitmap.Kind
	diags []Diagnostic

	northOf, southOf []int32 // vertical neighbour indices, -1 if empty

	conducts []bool // wire, power or ground: part of a net
	invalid  []bool // a transistor or bridge breaking its neighbour rule
}

type transistorPx struct {
	centre, gate, a, b int // grid indices
}

type bridgePx struct {
	n, e, s, w int // grid indices of the arms
}

// CompileDoc flattens and compiles a document.
func CompileDoc(d *doc.Document, opts Options) *Netlist {
	return Compile(d.Flatten(), opts)
}

// MaxArea bounds the bounding box of a compiled circuit, in cells (about
// 50 MB of compiler grids). 16384 x 16384 is far beyond a processor.
const MaxArea = 1 << 28

// Compile checks the flat document's cells and builds the netlist.
func Compile(flat *doc.Flat, opts Options) *Netlist {
	// A margin of 2 keeps every neighbour lookup inside the grid.
	r := flat.Pixels.Bounds().Inset(-2)
	if area := int64(r.Dx()) * int64(r.Dy()); area > MaxArea {
		// The compiler works on a dense grid over the bounding box; two
		// cells far apart would need gigabytes. Refuse instead.
		nl := Compile(&doc.Flat{Pixels: bitmap.New()}, opts)
		nl.Diagnostics = append(nl.Diagnostics, Diagnostic{Code: "E_TOO_LARGE", Level: Error, Pos: r.Min,
			Msg: fmt.Sprintf("the circuit spans %dx%d cells, above the limit of %d cells in area; move far-off cells closer", r.Dx(), r.Dy(), MaxArea)})
		return nl
	}
	c := &compiler{opts: opts, g: flat.Pixels.ToDense(r)}
	c.n = c.g.Count()
	c.xs = make([]int32, c.n)
	c.ys = make([]int32, c.n)
	c.kind = make([]bitmap.Kind, c.n)
	c.conducts = make([]bool, c.n)
	c.invalid = make([]bool, c.n)
	c.g.ForEach(func(i, x, y int) {
		c.xs[i], c.ys[i] = int32(x), int32(y)
		c.kind[i] = c.g.Kind(i)
		k := c.kind[i]
		c.conducts[i] = k == bitmap.Wire || k == bitmap.Power || k == bitmap.Ground
	})
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

	ts := c.findTransistors()
	bs := c.findBridges()
	return c.build(flat, ts, bs)
}

func (c *compiler) idx(x, y int) int     { return c.g.Index(x, y) }
func (c *compiler) pos(i int) (int, int) { return int(c.xs[i]), int(c.ys[i]) }
func (c *compiler) at(i int) image.Point { return image.Pt(c.pos(i)) }

// Neighbour indices of cell i, or -1 if that neighbour is empty. They accept
// -1 and return -1, so they chain: c.south(c.east(i)) is the cell at (+1,+1)
// if (+1,0) is not empty. East and west are adjacent in raster order.
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

// armsOK reports whether every arm conducts, reporting the ones that do not:
// a transistor or bridge cannot touch another transistor or bridge.
func (c *compiler) armsOK(i int, arms []int) bool {
	ok := true
	for _, a := range arms {
		switch c.kind[a] {
		case bitmap.Transistor:
			if c.kind[i] == bitmap.Transistor {
				if a > i { // report each pair once
					c.diag("E_ADJ_TRANSISTOR", Error, c.at(a), "two transistors touch; put a wire between them")
				}
			} else {
				c.diag("E_BRIDGE_ARM", Error, c.at(i), "a bridge touches a transistor; put a wire between them")
			}
			ok = false
		case bitmap.Bridge:
			if c.kind[i] == bitmap.Bridge {
				if a > i {
					c.diag("E_BRIDGE_ARM", Error, c.at(a), "two bridges touch; put a wire between them")
				}
			}
			ok = false
		}
	}
	return ok
}

// Transistor cells need exactly three non-empty orthogonal neighbours: the
// two collinear ones are the channel, the odd one is the gate.
func (c *compiler) findTransistors() []transistorPx {
	var ts []transistorPx
	for i := range c.n {
		if c.kind[i] != bitmap.Transistor {
			continue
		}
		arm := c.neighbours(i)
		count, missing := 0, -1
		var arms []int
		for k := range arm {
			if arm[k] >= 0 {
				count++
				arms = append(arms, arm[k])
			} else {
				missing = k
			}
		}
		if count != 3 {
			c.invalid[i] = true
			c.diag("E_TRANSISTOR_ARMS", Error, c.at(i), "a transistor needs exactly 3 neighbours (gate and channel), it has %d", count)
			continue
		}
		if !c.armsOK(i, arms) {
			c.invalid[i] = true
			continue
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
		ts = append(ts, transistorPx{centre: i, gate: arm[gate], a: a, b: b})
	}
	return ts
}

// Bridge cells need exactly four non-empty orthogonal neighbours; they join
// north to south and west to east.
func (c *compiler) findBridges() []bridgePx {
	var bs []bridgePx
	for i := range c.n {
		if c.kind[i] != bitmap.Bridge {
			continue
		}
		arm := c.neighbours(i)
		count := 0
		var arms []int
		for _, a := range arm {
			if a >= 0 {
				count++
				arms = append(arms, a)
			}
		}
		if count != 4 {
			c.invalid[i] = true
			c.diag("E_BRIDGE_ARMS", Error, c.at(i), "a bridge needs exactly 4 neighbours, it has %d", count)
			continue
		}
		if !c.armsOK(i, arms) {
			c.invalid[i] = true
			continue
		}
		bs = append(bs, bridgePx{n: arm[0], e: arm[1], s: arm[2], w: arm[3]})
	}
	return bs
}

// build unions cells into nets, names them and runs the remaining lint rules.
// Wire, power and ground cells conduct to each other; transistor and bridge
// cells belong to no net.
func (c *compiler) build(flat *doc.Flat, ts []transistorPx, bs []bridgePx) *Netlist {
	parent := newUnionFind(c.n)
	find := parent.find
	union := func(a, b int) { parent.union(int32(a), int32(b)) }
	for i := range c.n {
		if !c.conducts[i] {
			continue
		}
		if j := c.east(i); j >= 0 && c.conducts[j] {
			union(i, j)
		}
		if j := c.south(i); j >= 0 && c.conducts[j] {
			union(i, j)
		}
	}
	for _, b := range bs {
		union(b.n, b.s)
		union(b.w, b.e)
	}

	nl := &Netlist{names: map[string]NetID{}, grid: c.g}
	nl.roles = make([]Role, c.n)
	nl.pixNet = make([]NetID, c.n)
	netOfRoot := make([]NetID, c.n)
	for i := range netOfRoot {
		netOfRoot[i] = NoNet
	}
	hasHigh, hasLow := []bool{}, []bool{}
	for i := range c.n {
		nl.roles[i] = [...]Role{bitmap.Wire: RoleWire, bitmap.Power: RoleHigh, bitmap.Ground: RoleLow,
			bitmap.Transistor: RoleTransistor, bitmap.Bridge: RoleBridge}[c.kind[i]]
		if c.invalid[i] {
			nl.roles[i] = RoleInvalid
		}
		if !c.conducts[i] {
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
		switch c.kind[i] {
		case bitmap.Power:
			hasHigh[id] = true
		case bitmap.Ground:
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
	nl.GatedBy = make([][]int32, len(nl.Nets))
	nl.ChannelOf = make([][]int32, len(nl.Nets))
	for k, t := range ts {
		tr := Transistor{Gate: nl.pixNet[t.gate], A: nl.pixNet[t.a], B: nl.pixNet[t.b], Pos: c.at(t.centre)}
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
		if i < 0 || !c.conducts[i] {
			c.diag("E_LABEL_OFF_NET", Error, l.Pos, "label %q is not on a wire, power or ground cell", l.Name)
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

// diagonalLint warns about cells of different nets that touch only at a
// corner, except arms of one transistor or one bridge, which always do.
func (c *compiler) diagonalLint(nl *Netlist) {
	for i := range c.n {
		if !c.conducts[i] {
			continue
		}
		x, y := c.pos(i)
		for _, dx := range [2]int{-1, 1} {
			j := c.idx(x+dx, y+1)
			if j < 0 || !c.conducts[j] || nl.pixNet[i] == nl.pixNet[j] {
				continue
			}
			// The two cells orthogonally adjacent to both.
			exempt := false
			for _, p := range [2]image.Point{{x + dx, y}, {x, y + 1}} {
				if k := c.idx(p.X, p.Y); k >= 0 && (c.kind[k] == bitmap.Transistor || c.kind[k] == bitmap.Bridge) {
					exempt = true
				}
			}
			if !exempt {
				c.diag("W_DIAGONAL_TOUCH", Warning, image.Pt(x, y),
					"cell touches cell %d,%d of another net only diagonally; diagonals never connect", x+dx, y+1)
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
