// Package bitmap stores unbounded one-bit images in 64x64 chunks, with
// rectangle operations and the row-string codec used by files, fixtures and
// the clipboard.
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

// chunk holds 64 rows of 64 bits; bit x of rows[y] is pixel (x, y).
type chunk [chunkSize]uint64

func (c *chunk) empty() bool {
	for _, r := range c {
		if r != 0 {
			return false
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

// Bitmap is an unbounded set of on pixels. The zero value is not usable; call New.
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

// Get reports whether pixel (x, y) is on.
func (b *Bitmap) Get(x, y int) bool {
	k, lx, ly := split(x, y)
	c := b.lookup(k)
	return c != nil && c[ly]>>lx&1 != 0
}

// Set turns pixel (x, y) on or off.
func (b *Bitmap) Set(x, y int, on bool) {
	k, lx, ly := split(x, y)
	c := b.lookup(k)
	if c == nil {
		if !on {
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
	if on {
		c[ly] |= 1 << lx
	} else {
		c[ly] &^= 1 << lx
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
		for _, r := range b.chunks[k] {
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
		for y, row := range c {
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

// ForEach calls fn for every on pixel in raster order: increasing y, then
// increasing x. fn must not modify the bitmap.
func (b *Bitmap) ForEach(fn func(x, y int)) {
	keys := b.sortedKeys()
	for i := 0; i < len(keys); {
		// keys[i:j] is one row of chunks, sorted by x.
		j := i
		for j < len(keys) && keys[j].y == keys[i].y {
			j++
		}
		row := keys[i:j]
		for ly := range chunkSize {
			y := keys[i].y<<chunkShift + ly
			for _, k := range row {
				w := b.chunks[k][ly]
				for w != 0 {
					lx := bits.TrailingZeros64(w)
					w &= w - 1
					fn(k.x<<chunkShift+lx, y)
				}
			}
		}
		i = j
	}
}

// ForEachIn is ForEach restricted to pixels inside r.
func (b *Bitmap) ForEachIn(r image.Rectangle, fn func(x, y int)) {
	// Simple and correct; callers that need speed on huge bitmaps use Dense.
	b.ForEach(func(x, y int) {
		if image.Pt(x, y).In(r) {
			fn(x, y)
		}
	})
}

// Clone returns an independent copy.
func (b *Bitmap) Clone() *Bitmap {
	n := &Bitmap{chunks: make(map[key]*chunk, len(b.chunks)), sorted: b.sorted}
	n.keys = slices.Clone(b.keys)
	for _, k := range b.keys {
		c := *b.chunks[k]
		n.chunks[k] = &c
	}
	return n
}

// Equal reports whether both bitmaps have exactly the same on pixels.
func (b *Bitmap) Equal(o *Bitmap) bool {
	for _, k := range b.keys {
		c, oc := b.chunks[k], o.chunks[k]
		if oc == nil {
			if !c.empty() {
				return false
			}
		} else if *c != *oc {
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

// Or turns on every pixel of src, translated by (dx, dy).
func (b *Bitmap) Or(src *Bitmap, dx, dy int) {
	src.ForEach(func(x, y int) { b.Set(x+dx, y+dy, true) })
}

// Crop returns the pixels inside r, at their original coordinates.
func (b *Bitmap) Crop(r image.Rectangle) *Bitmap {
	n := New()
	b.ForEachIn(r, func(x, y int) { n.Set(x, y, true) })
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
		for ly, row := range c {
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
