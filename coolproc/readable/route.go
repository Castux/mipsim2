package main

import (
	"fmt"
	"image"
	"os"
	"sort"
	"time"

	"github.com/Castux/mipsim2/bitmap"
)

// Builder makes a component from instances of others, wired by a router.
//
// Wires obey the cell rules: a wire never touches a cell of another net
// (orthogonally), so nets only meet where intended. A wire may cross
// another one where that one runs straight through a cell: the cell becomes
// a bridge, and the crossing wire goes straight through it.
type Builder struct {
	root  bool // the document's root: no edge to keep clear
	lib   *Library
	def   *Def
	nets  []*net
	named map[string]*net
}

type net struct {
	name   string
	terms  []image.Point // parent cells that must be wired
	links  [][2]int      // terminals already joined inside a child
	ports  []string      // port names of exports on this net
	cells  map[image.Point]bool
	source bitmap.Kind // a constant: Power or Ground on its first terminal
	label  bool
}

func (l *Library) build(id string, w, h int) *Builder {
	return &Builder{lib: l, def: newDef(id, w, h), named: map[string]*net{}}
}

func (b *Builder) net(name string) *net {
	if n, ok := b.named[name]; ok {
		return n
	}
	n := &net{name: name, cells: map[image.Point]bool{}, label: true}
	b.nets = append(b.nets, n)
	b.named[name] = n
	return n
}

// Place puts an instance of d at (x, y).
func (b *Builder) Place(d *Def, name string, x, y int) *Inst {
	in := &Inst{Name: name, Def: d, X: x, Y: y}
	r := in.Rect()
	// A margin of 2, so that wires reaching its ports stay off our edge.
	if r.Min.X < 2 || r.Min.Y < 2 || r.Max.X > b.def.W-2 || r.Max.Y > b.def.H-2 {
		panic(fmt.Sprintf("%s: %s at %v does not fit inside %dx%d with a margin", b.def.ID, name, r, b.def.W, b.def.H))
	}
	for _, o := range b.def.insts {
		if o.Rect().Inset(-1).Overlaps(r) {
			panic(fmt.Sprintf("%s: %s overlaps or touches %s", b.def.ID, name, o.Name))
		}
	}
	b.def.insts = append(b.def.insts, in)
	return in
}

// Connect wires an instance's port to the net called name.
func (b *Builder) Connect(name string, in *Inst, port string) {
	b.net(name).terms = append(b.net(name).terms, in.outside(port))
}

// Through wires a child's two ports to the net when the child joins them
// inside (a lane passing through it): the router need not join them.
func (b *Builder) Through(name string, in *Inst, a, c string) {
	n := b.net(name)
	n.terms = append(n.terms, in.outside(a), in.outside(c))
	n.links = append(n.links, [2]int{len(n.terms) - 2, len(n.terms) - 1})
}

// Export makes a port of the component at p, on its edge, wired to the net.
//
// The first port of a net is labelled with the net's name; other ports of
// the same net (a wire passing through, say) are not, as one net carries
// one name.
func (b *Builder) Export(name, port string, p image.Point) {
	n := b.net(name)
	b.def.port(port, p, false)
	if len(n.ports) == 0 {
		b.def.labels[p] = name
	}
	n.terms = append(n.terms, p)
	n.ports = append(n.ports, port)
}

// Tie makes a net a constant: high (Power) or low (Ground).
func (b *Builder) Tie(name string, k bitmap.Kind) { b.net(name).source = k }

// Constant puts a named constant (Power or Ground) at p, for devices that
// need a net always high or low.
func (b *Builder) Constant(name string, k bitmap.Kind, p image.Point) {
	n := b.net(name)
	n.terms = append(n.terms, p)
	n.source = k
}

// Quiet stops a net from being labelled (for nets whose name is not worth
// showing).
func (b *Builder) Quiet(name string) { b.net(name).label = false }

// router state

type grid struct {
	w, h    int
	blocked []bool // inside an instance
	owner   []*net // wire cells and instance port cells
	bridge  []bool
	edge    bool // cells on the component's edge are reserved for its ports
	exports map[image.Point]*net
	// search buffers, reused: a state's best cost and previous state are
	// valid when its stamp is the current generation.
	best   []int32
	prev   []int32
	stamp  []uint32
	closed []uint32
	gen    uint32
}

func (g *grid) in(p image.Point) bool { return p.X >= 0 && p.Y >= 0 && p.X < g.w && p.Y < g.h }
func (g *grid) i(p image.Point) int   { return p.Y*g.w + p.X }

var dirs = [4]image.Point{{1, 0}, {0, 1}, {-1, 0}, {0, -1}}

// Route wires every net and finishes the component. When a net cannot be
// routed, it starts over with that net first, a few times.
func (b *Builder) Route() *Def {
	d := b.def
	order := append([]*net(nil), b.nets...)
	// Nets with more terminals first, then in order.
	sort.SliceStable(order, func(i, j int) bool { return len(order[i].terms) > len(order[j].terms) })
	var g *grid
	start := time.Now()
	for attempt := 0; ; attempt++ {
		var failed *net
		var targets []image.Point
		g, failed, targets = b.routeAll(order)
		if os.Getenv("ROUTE_TIMES") != "" {
			fmt.Fprintf(os.Stderr, "%s attempt %d: %v failed %v\n", d.ID, attempt, time.Since(start), failed != nil)
		}
		if failed == nil {
			break
		}
		if attempt == 10 {
			b.debugDump(g, failed, targets)
			panic(fmt.Sprintf("%s: cannot route %s to %v", d.ID, failed.name, targets))
		}
		for k, n := range order {
			if n == failed {
				order = append(append([]*net{failed}, order[:k]...), order[k+1:]...)
				break
			}
		}
	}
	// Draw.
	for _, n := range b.nets {
		for p := range n.cells {
			d.cells[p] = bitmap.Wire
		}
	}
	for i, br := range g.bridge {
		if br {
			d.cells[image.Pt(i%g.w, i/g.w)] = bitmap.Bridge
		}
	}
	for _, n := range b.nets {
		if n.source != bitmap.Empty {
			d.cells[n.terms[0]] = n.source
		}
		if !n.label || len(n.ports) > 0 {
			continue // an exported net is named by its port
		}
		// Label the net on one of its wire cells, top-left first.
		var cs []image.Point
		for p := range n.cells {
			if k := d.cells[p]; k == bitmap.Wire || k == bitmap.Power || k == bitmap.Ground {
				cs = append(cs, p)
			}
		}
		sort.Slice(cs, func(i, j int) bool {
			if cs[i].Y != cs[j].Y {
				return cs[i].Y < cs[j].Y
			}
			return cs[i].X < cs[j].X
		})
		if len(cs) > 0 {
			d.labels[cs[0]] = n.name
		}
	}
	return b.lib.add(d)
}

// routeAll routes the nets in order on a fresh grid. It returns the net
// that failed, if any, and where it was going.
func (b *Builder) routeAll(order []*net) (*grid, *net, []image.Point) {
	d := b.def
	for _, n := range b.nets {
		n.cells = map[image.Point]bool{}
	}
	g := &grid{w: d.W, h: d.H, edge: !b.root, exports: map[image.Point]*net{}}
	g.blocked = make([]bool, d.W*d.H)
	g.owner = make([]*net, d.W*d.H)
	g.bridge = make([]bool, d.W*d.H)
	// Instance cells are blocked; their ports belong to their nets (or to
	// a net of their own when unconnected).
	stubOf := map[image.Point]*net{}
	for _, n := range b.nets {
		for _, t := range n.terms {
			if o := stubOf[t]; o != nil && o != n {
				panic(fmt.Sprintf("%s: nets %s and %s share the terminal %v", d.ID, o.name, n.name, t))
			}
			stubOf[t] = n
		}
		for _, p := range n.ports {
			g.exports[d.Port(p)] = n
		}
	}
	for _, in := range d.insts {
		r := in.Rect()
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				g.blocked[g.i(image.Pt(x, y))] = true
			}
		}
		for _, name := range in.Def.order {
			p := in.Port(name)
			o := stubOf[in.outside(name)]
			if o == nil {
				o = &net{name: in.Name + "." + name + " (unconnected)", cells: map[image.Point]bool{}}
			}
			g.owner[g.i(p)] = o
		}
	}
	// Terminals are reserved for their nets from the start.
	for _, n := range b.nets {
		for _, t := range n.terms {
			if !g.in(t) || g.blocked[g.i(t)] {
				panic(fmt.Sprintf("%s: terminal of %s at %v is outside or inside an instance", d.ID, n.name, t))
			}
			g.owner[g.i(t)] = n
			n.cells[t] = true
		}
	}
	for _, n := range order {
		groups := n.groups()
		if len(groups) < 2 {
			continue
		}
		tree := append([]image.Point(nil), groups[0]...)
		rest := groups[1:]
		for len(rest) > 0 {
			// The group nearest the tree next.
			best, bd := 0, 1<<30
			for k, grp := range rest {
				for _, t := range grp {
					for _, c := range tree {
						if dd := abs(t.X-c.X) + abs(t.Y-c.Y); dd < bd {
							best, bd = k, dd
						}
					}
				}
			}
			grp := rest[best]
			rest = append(rest[:best], rest[best+1:]...)
			path, bridges, ok := g.search(n, tree, grp)
			if !ok {
				return g, n, grp
			}
			tree = append(tree, grp...)
			for _, p := range path {
				g.owner[g.i(p)] = n
				n.cells[p] = true
				tree = append(tree, p)
			}
			for _, p := range bridges {
				g.bridge[g.i(p)] = true
			}
		}
	}
	return g, nil, nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// otherAt returns the net owning p if it is not n (wire, bridge or port).
func (g *grid) other(p image.Point, n *net) bool {
	if !g.in(p) {
		return false
	}
	o := g.owner[g.i(p)]
	return o != nil && o != n
}

// crossable reports whether a wire going in direction d can cross the cell
// q of another net: q is a plain wire running straight across d, with
// exactly its two neighbours across, neither a bridge, and the cell beyond
// q is free.
func (g *grid) crossable(q image.Point, d int, n *net) bool {
	if !g.in(q) || g.blocked[g.i(q)] || g.bridge[g.i(q)] {
		return false
	}
	m := g.owner[g.i(q)]
	if m == nil || m == n {
		return false
	}
	if _, isExport := g.exports[q]; isExport {
		return false
	}
	across := dirs[(d+1)%4]
	for _, s := range []image.Point{q.Add(across), q.Sub(across)} {
		if !g.in(s) || g.owner[g.i(s)] != m || g.bridge[g.i(s)] {
			return false
		}
	}
	beyond := q.Add(dirs[d])
	return g.in(beyond) && g.owner[g.i(beyond)] == nil && !g.blocked[g.i(beyond)]
}

// enter reports whether the net can put a wire on c, arriving in direction
// d, and whether it must then cross the cell ahead.
func (g *grid) enter(c image.Point, d int, n *net, from image.Point) (ok, must bool) {
	if !g.in(c) || g.blocked[g.i(c)] || g.bridge[g.i(c)] {
		return false, false
	}
	if o := g.owner[g.i(c)]; o != nil && o != n {
		return false, false
	}
	if g.edge && (c.X == 0 || c.Y == 0 || c.X == g.w-1 || c.Y == g.h-1) && g.exports[c] != n {
		return false, false
	}
	for k, dv := range dirs {
		q := c.Add(dv)
		if q == from || !g.in(q) {
			continue
		}
		if g.bridge[g.i(q)] {
			return false, false
		}
		if g.other(q, n) {
			if k == d && g.crossable(q, d, n) {
				must = true
				continue
			}
			return false, false
		}
	}
	return true, must
}

func (g *grid) diagonals(c image.Point, n *net) int {
	k := 0
	for _, dv := range []image.Point{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
		if g.other(c.Add(dv), n) {
			k++
		}
	}
	return k
}

// groups returns the net's terminals, those joined inside a child together.
func (n *net) groups() [][]image.Point {
	root := make([]int, len(n.terms))
	for i := range root {
		root[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for root[i] != i {
			i = root[i]
		}
		return i
	}
	for _, l := range n.links {
		root[find(l[0])] = find(l[1])
	}
	byRoot := map[int]int{}
	var out [][]image.Point
	for i, t := range n.terms {
		r := find(i)
		k, ok := byRoot[r]
		if !ok {
			k = len(out)
			byRoot[r] = k
			out = append(out, nil)
		}
		out[k] = append(out[k], t)
	}
	return out
}

// search finds a path from the tree to one of the targets. It returns the
// new wire cells and the cells that become bridges. It searches a window
// around the tree and targets first, widening it when that fails.
func (g *grid) search(n *net, tree []image.Point, targets []image.Point) ([]image.Point, []image.Point, bool) {
	box := image.Rectangle{Min: targets[0], Max: targets[0].Add(image.Pt(1, 1))}
	for _, p := range append(append([]image.Point(nil), tree...), targets...) {
		box = box.Union(image.Rectangle{Min: p, Max: p.Add(image.Pt(1, 1))})
	}
	full := image.Rect(0, 0, g.w, g.h)
	for margin := 24; ; margin *= 4 {
		win := box.Inset(-margin).Intersect(full)
		t0 := time.Now()
		path, bridges, ok := g.searchIn(win, n, tree, targets)
		if os.Getenv("ROUTE_TIMES") != "" && time.Since(t0) > 200*time.Millisecond {
			fmt.Fprintf(os.Stderr, "  slow search %s window %v ok %v: %v\n", n.name, win.Size(), ok, time.Since(t0))
		}
		if ok {
			return path, bridges, true
		}
		if win == full {
			return nil, nil, false
		}
	}
}

// searchIn is A* restricted to win. States are a cell and the direction of
// arrival (4 at the tree). Crossing another wire is one straight move over
// it: the crossed cell becomes a bridge.
func (g *grid) searchIn(win image.Rectangle, n *net, tree []image.Point, targets []image.Point) ([]image.Point, []image.Point, bool) {
	ww := win.Dx()
	idx := func(p image.Point, d int) int { return ((p.Y-win.Min.Y)*ww+(p.X-win.Min.X))*5 + d }
	size := ww * win.Dy() * 5
	if cap(g.best) < size {
		g.best = make([]int32, size)
		g.prev = make([]int32, size)
		g.stamp = make([]uint32, size)
		g.closed = make([]uint32, size)
	}
	g.gen++
	best, prev, stamp, closed := g.best[:size], g.prev[:size], g.stamp[:size], g.closed[:size]
	seen := func(k int) bool { return stamp[k] == g.gen }
	h := func(p image.Point) int {
		b := 1 << 30
		for _, t := range targets {
			b = min(b, abs(p.X-t.X)+abs(p.Y-t.Y))
		}
		return b
	}
	isTarget := func(p image.Point) bool {
		for _, t := range targets {
			if p == t {
				return true
			}
		}
		return false
	}
	// Weighted A*: the heuristic counts double, so the search heads for
	// the target and settles for a near-shortest path.
	const weight = 2
	open := &pq{}
	for _, p := range tree {
		if !p.In(win) {
			continue
		}
		k := idx(p, 4)
		stamp[k], best[k], prev[k] = g.gen, 0, -1
		open.push(item{k: k, p: p, d: 4, f: weight * h(p)})
	}
	const bend, cross, diag = 3, 4, 2
	for len(*open) > 0 {
		it := open.pop()
		if it.c > int(best[it.k]) || closed[it.k] == g.gen {
			continue
		}
		closed[it.k] = g.gen // expanded once: weighted A* never reopens
		if isTarget(it.p) {
			// Walk back: each move is a straight run from the previous state.
			var path, bridges []image.Point
			k, p, d := it.k, it.p, it.d
			for prev[k] >= 0 {
				pk := int(prev[k])
				pc := pk / 5
				pp := image.Pt(win.Min.X+pc%ww, win.Min.Y+pc/ww)
				for q := p; q != pp; q = q.Sub(dirs[d]) {
					if o := g.owner[g.i(q)]; o != nil && o != n {
						bridges = append(bridges, q)
					} else {
						path = append(path, q)
					}
				}
				k, p, d = pk, pp, pk%5
			}
			return path, bridges, true
		}
		for nd := range 4 {
			if it.d < 4 && nd == (it.d+2)%4 {
				continue // no turning back
			}
			c := it.p.Add(dirs[nd])
			if !c.In(win) {
				continue
			}
			ok, must := g.enter(c, nd, n, it.p)
			if !ok {
				continue
			}
			cost := it.c + 1 + diag*g.diagonals(c, n)
			if it.d < 4 && nd != it.d {
				cost += bend
			}
			cur := c
			for must && ok {
				q := cur.Add(dirs[nd])
				nx := q.Add(dirs[nd])
				if !nx.In(win) {
					ok = false
					break
				}
				ok, must = g.enter(nx, nd, n, q)
				cur = nx
				cost += 2 + cross + diag*g.diagonals(nx, n)
			}
			if !ok {
				continue
			}
			k := idx(cur, nd)
			if closed[k] == g.gen || seen(k) && int(best[k]) <= cost {
				continue
			}
			stamp[k], best[k], prev[k] = g.gen, int32(cost), int32(it.k)
			open.push(item{k: k, p: cur, d: nd, c: cost, f: cost + weight*h(cur)})
		}
	}
	return nil, nil, false
}

type item struct {
	k    int
	p    image.Point
	d    int
	f, c int
}

// pq is a binary min-heap of items by f, deeper items first on ties.
type pq []item

func (q pq) less(i, j int) bool {
	if q[i].f != q[j].f {
		return q[i].f < q[j].f
	}
	return q[i].c > q[j].c
}

func (q *pq) push(it item) {
	*q = append(*q, it)
	h := *q
	for i := len(h) - 1; i > 0; {
		p := (i - 1) / 2
		if !h.less(i, p) {
			break
		}
		h[i], h[p] = h[p], h[i]
		i = p
	}
}

func (q *pq) pop() item {
	h := *q
	top := h[0]
	n := len(h) - 1
	h[0] = h[n]
	h = h[:n]
	for i := 0; ; {
		l, r, m := 2*i+1, 2*i+2, i
		if l < n && h.less(l, m) {
			m = l
		}
		if r < n && h.less(r, m) {
			m = r
		}
		if m == i {
			break
		}
		h[i], h[m] = h[m], h[i]
		i = m
	}
	*q = h
	return top
}

// debugDump writes the component as routed so far, with the failing
// targets marked by labels, to the file named by ROUTE_DEBUG.
func (b *Builder) debugDump(g *grid, failed *net, targets []image.Point) {
	path := os.Getenv("ROUTE_DEBUG")
	if path == "" {
		return
	}
	d := b.def
	for _, n := range b.nets {
		for p := range n.cells {
			d.cells[p] = bitmap.Wire
		}
	}
	for i, br := range g.bridge {
		if br {
			d.cells[image.Pt(i%g.w, i/g.w)] = bitmap.Bridge
		}
	}
	for k, t := range targets {
		d.labels[t] = fmt.Sprintf("FAILED_%d", k)
	}
	root := newDef("top", 0, 0)
	root.insts = append(root.insts, &Inst{Name: "debug", Def: d})
	b.lib.add(d)
	data, err := b.lib.Document(root).Save()
	if err == nil {
		os.WriteFile(path, data, 0o644)
	}
}
