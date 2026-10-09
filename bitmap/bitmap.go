// Package bitmap stores unbounded grids of typed cells in 64x64 chunks, with
// rectangle operations and the row-string codec used by files and the
// clipboard. A cell is empty or one of the Kinds; "on" means not empty.
//
// Iteration is always deterministic: ForEach visits pixels in raster order
// (y, then x), whatever order they were set in.
package bitmap

import (
	"image"
	"math/bits"
	"slices"
)

const (
	chunkShift = 6
	chunkSize  = 1 << chunkShift // 64
	chunkMask  = chunkSize - 1
)

// Kind is what a cell is.
type Kind uint8

const (
	Empty Kind = iota
	Wire
	Power  // high source
	Ground // low source
	Transistor
	Bridge
	numKinds
)

// Kinds lists the non-empty kinds, in order.
var Kinds = []Kind{Wire, Power, Ground, Transistor, Bridge}

func (k Kind) String() string {
	return [...]string{"empty", "wire", "power", "ground", "transistor", "bridge"}[k]
}

// kindBits is the number of planes coding a cell's kind minus one (Wire = 0).
const kindBits = 3

// chunk holds 64 rows of 64 cells. Bit x of on[y] is set when cell (x, y) is
// not empty; its kind minus one is spread over the planes in k, which stay
// nil while every cell of the chunk is a wire.
type chunk struct {
	on [chunkSize]uint64
	k  *[kindBits][chunkSize]uint64
}

func (c *chunk) empty() bool {
	for _, r := range c.on {
		if r != 0 {
			return false
		}
	}
	return true
}

func (c *chunk) kind(lx, ly int) Kind {
	if c.on[ly]>>lx&1 == 0 {
		return Empty
	}
	if c.k == nil {
		return Wire
	}
	v := Kind(0)
	for p := range kindBits {
		v |= Kind(c.k[p][ly]>>lx&1) << p
	}
	return v + 1
}

func (c *chunk) clone() *chunk {
	n := &chunk{on: c.on}
	if c.k != nil {
		k := *c.k
		n.k = &k
	}
	return n
}

// equal compares cells, ignoring stale kind bits under empty cells.
func (c *chunk) equal(o *chunk) bool {
	if c.on != o.on {
		return false
	}
	for ly, row := range c.on {
		for p := range kindBits {
			var a, b uint64
			if c.k != nil {
				a = c.k[p][ly]
			}
			if o.k != nil {
				b = o.k[p][ly]
			}
			if (a^b)&row != 0 {
				return false
			}
		}
	}
	return true
}

type key struct{ x, y int }

func (a key) cmp(b key) int {
	if a.y != b.y {
		return a.y - b.y
	}
	return a.x - b.x
}

// Bitmap is an unbounded grid of cells. The zero value is not usable; call New.
type Bitmap struct {
	chunks map[key]*chunk
	keys   []key // every key in chunks; sorted when sorted is true
	sorted bool

	// One-entry cache: raster access stays in one chunk for 64 pixels.
	lastKey   key
	lastChunk *chunk
}

// New returns an empty bitmap.
func New() *Bitmap {
	return &Bitmap{chunks: map[key]*chunk{}, sorted: true}
}

func split(x, y int) (key, int, int) {
	return key{x >> chunkShift, y >> chunkShift}, x & chunkMask, y & chunkMask
}

func (b *Bitmap) lookup(k key) *chunk {
	if b.lastChunk != nil && b.lastKey == k {
		return b.lastChunk
	}
	c := b.chunks[k]
	if c != nil {
		b.lastKey, b.lastChunk = k, c
	}
	return c
}

// Get reports whether cell (x, y) is not empty.
func (b *Bitmap) Get(x, y int) bool {
	k, lx, ly := split(x, y)
	c := b.lookup(k)
	return c != nil && c.on[ly]>>lx&1 != 0
}

// At returns the kind of cell (x, y).
func (b *Bitmap) At(x, y int) Kind {
	k, lx, ly := split(x, y)
	c := b.lookup(k)
	if c == nil {
		return Empty
	}
	return c.kind(lx, ly)
}

// Set makes cell (x, y) a wire, or empty.
func (b *Bitmap) Set(x, y int, on bool) {
	if on {
		b.Put(x, y, Wire)
	} else {
		b.Put(x, y, Empty)
	}
}

// Put sets the kind of cell (x, y).
func (b *Bitmap) Put(x, y int, kind Kind) {
	k, lx, ly := split(x, y)
	c := b.lookup(k)
	if c == nil {
		if kind == Empty {
			return
		}
		c = new(chunk)
		b.chunks[k] = c
		if n := len(b.keys); n > 0 && b.sorted && b.keys[n-1].cmp(k) > 0 {
			b.sorted = false
		}
		b.keys = append(b.keys, k)
		b.lastKey, b.lastChunk = k, c
	}
	if kind == Empty {
		c.on[ly] &^= 1 << lx
		return
	}
	c.on[ly] |= 1 << lx
	v := kind - 1
	if v != 0 && c.k == nil {
		c.k = new([kindBits][chunkSize]uint64)
	}
	if c.k != nil {
		for p := range kindBits {
			if v>>p&1 != 0 {
				c.k[p][ly] |= 1 << lx
			} else {
				c.k[p][ly] &^= 1 << lx
			}
		}
	}
}

func (b *Bitmap) sortedKeys() []key {
	if !b.sorted {
		slices.SortFunc(b.keys, key.cmp)
		b.sorted = true
	}
	return b.keys
}

// Compact drops chunks that have become empty. It never changes the pixels.
func (b *Bitmap) Compact() {
	keys := b.keys[:0]
	for _, k := range b.keys {
		if b.chunks[k].empty() {
			delete(b.chunks, k)
			continue
		}
		keys = append(keys, k)
	}
	b.keys = keys
	b.lastChunk = nil
}

// Count returns the number of on pixels.
func (b *Bitmap) Count() int {
	n := 0
	for _, k := range b.keys {
		for _, r := range b.chunks[k].on {
			n += bits.OnesCount64(r)
		}
	}
	return n
}

// IsEmpty reports whether no pixel is on.
func (b *Bitmap) IsEmpty() bool {
	for _, k := range b.keys {
		if !b.chunks[k].empty() {
			return false
		}
	}
	return true
}

// Bounds returns the smallest rectangle containing every on pixel, or the
// zero rectangle if the bitmap is empty.
func (b *Bitmap) Bounds() image.Rectangle {
	var r image.Rectangle
	first := true
	for _, k := range b.keys {
		c := b.chunks[k]
		var or uint64
		minY, maxY := -1, -1
		for y, row := range c.on {
			if row != 0 {
				if minY < 0 {
					minY = y
				}
				maxY = y
				or |= row
			}
		}
		if or == 0 {
			continue
		}
		cr := image.Rect(
			k.x<<chunkShift+bits.TrailingZeros64(or),
			k.y<<chunkShift+minY,
			k.x<<chunkShift+64-bits.LeadingZeros64(or),
			k.y<<chunkShift+maxY+1,
		)
		if first {
			r, first = cr, false
		} else {
			r = r.Union(cr)
		}
	}
	return r
}

// ForEach calls fn for every non-empty cell in raster order: increasing y,
// then increasing x. fn must not modify the bitmap.
func (b *Bitmap) ForEach(fn func(x, y int)) {
	b.ForEachCell(func(x, y int, _ Kind) { fn(x, y) })
}

// ForEachCell is ForEach with each cell's kind.
func (b *Bitmap) ForEachCell(fn func(x, y int, k Kind)) {
	keys := b.sortedKeys()
	for i := 0; i < len(keys); {
		// keys[i:j] is one row of chunks, sorted by x.
		j := i
		for j < len(keys) && keys[j].y == keys[i].y {
			j++
		}
		row := keys[i:j]
		// Resolve the row's chunks once, not once per pixel row.
		type ent struct {
			c  *chunk
			x0 int
		}
		var buf [32]ent
		cs := buf[:0]
		for _, k := range row {
			cs = append(cs, ent{b.chunks[k], k.x << chunkShift})
		}
		y0 := keys[i].y << chunkShift
		for ly := range chunkSize {
			for _, e := range cs {
				w := e.c.on[ly]
				for w != 0 {
					lx := bits.TrailingZeros64(w)
					w &= w - 1
					fn(e.x0+lx, y0+ly, e.c.kind(lx, ly))
				}
			}
		}
		i = j
	}
}

// ForEachIn is ForEach restricted to cells inside r.
func (b *Bitmap) ForEachIn(r image.Rectangle, fn func(x, y int)) {
	b.ForEachCellIn(r, func(x, y int, _ Kind) { fn(x, y) })
}

// ForEachCellIn is ForEachCell restricted to cells inside r.
func (b *Bitmap) ForEachCellIn(r image.Rectangle, fn func(x, y int, k Kind)) {
	// Simple and correct; callers that need speed on huge bitmaps use Dense.
	b.ForEachCell(func(x, y int, k Kind) {
		if image.Pt(x, y).In(r) {
			fn(x, y, k)
		}
	})
}

// Clone returns an independent copy.
func (b *Bitmap) Clone() *Bitmap {
	n := &Bitmap{chunks: make(map[key]*chunk, len(b.chunks)), sorted: b.sorted}
	n.keys = slices.Clone(b.keys)
	for _, k := range b.keys {
		n.chunks[k] = b.chunks[k].clone()
	}
	return n
}

// Equal reports whether both bitmaps have exactly the same cells.
func (b *Bitmap) Equal(o *Bitmap) bool {
	for _, k := range b.keys {
		c, oc := b.chunks[k], o.chunks[k]
		if oc == nil {
			if !c.empty() {
				return false
			}
		} else if !c.equal(oc) {
			return false
		}
	}
	for _, k := range o.keys {
		if b.chunks[k] == nil && !o.chunks[k].empty() {
			return false
		}
	}
	return true
}

// Or copies every non-empty cell of src, translated by (dx, dy), over b.
func (b *Bitmap) Or(src *Bitmap, dx, dy int) {
	if src == b {
		src = b.Clone() // ForEach forbids changing the bitmap it walks
	}
	src.ForEachCell(func(x, y int, k Kind) { b.Put(x+dx, y+dy, k) })
}

// Crop returns the cells inside r, at their original coordinates.
func (b *Bitmap) Crop(r image.Rectangle) *Bitmap {
	n := New()
	b.ForEachCellIn(r, func(x, y int, k Kind) { n.Put(x, y, k) })
	return n
}

// ClearRect turns off every pixel inside r.
func (b *Bitmap) ClearRect(r image.Rectangle) {
	var off []image.Point
	b.ForEachIn(r, func(x, y int) { off = append(off, image.Pt(x, y)) })
	for _, p := range off {
		b.Set(p.X, p.Y, false)
	}
}

// AnyIn reports whether any pixel inside r is on.
func (b *Bitmap) AnyIn(r image.Rectangle) bool {
	r = r.Canon()
	if r.Empty() {
		return false
	}
	k0, _, _ := split(r.Min.X, r.Min.Y)
	k1, _, _ := split(r.Max.X-1, r.Max.Y-1)
	for _, k := range b.keys {
		if k.x < k0.x || k.x > k1.x || k.y < k0.y || k.y > k1.y {
			continue
		}
		c := b.chunks[k]
		for ly, row := range c.on {
			if row == 0 {
				continue
			}
			y := k.y<<chunkShift + ly
			if y < r.Min.Y || y >= r.Max.Y {
				continue
			}
			x0 := max(r.Min.X-k.x<<chunkShift, 0)
			x1 := min(r.Max.X-k.x<<chunkShift, chunkSize)
			if x0 >= x1 {
				continue
			}
			if row&rangeMask(x0, x1) != 0 {
				return true
			}
		}
	}
	return false
}

// rangeMask has bits [lo, hi) set, for 0 <= lo < hi <= 64.
func rangeMask(lo, hi int) uint64 {
	var m uint64 = ^uint64(0)
	if hi < 64 {
		m = 1<<hi - 1
	}
	return m &^ (1<<lo - 1)
}
