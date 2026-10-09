package bitmap

import (
	"image"
	"math/bits"
)

// Dense is a fixed rectangle of pixels stored as packed rows, for code that
// scans every pixel and its neighbours (the pattern compiler). Pixels outside
// Rect read as off.
//
// Every on pixel also has an index: its position in raster order among the on
// pixels, from 0 to Count()-1. Index is O(1), so per-pixel data can live in
// slices sized by the number of on pixels rather than the rectangle's area.
type Dense struct {
	Rect   image.Rectangle
	stride int // words per row
	words  []uint64
	rank   []int32 // rank[i] = number of on pixels in words[:i]
	count  int
	kinds  []Kind // by index
}

// ToDense copies the pixels inside r into a Dense.
func (b *Bitmap) ToDense(r image.Rectangle) *Dense {
	r = r.Canon()
	d := &Dense{Rect: r, stride: (r.Dx() + 63) / 64}
	d.words = make([]uint64, d.stride*r.Dy())
	defer func() {
		d.buildRank()
		d.kinds = make([]Kind, d.count)
		d.ForEach(func(i, x, y int) { d.kinds[i] = b.At(x, y) })
	}()
	if r.Empty() {
		return d
	}
	for _, k := range b.keys {
		c := b.chunks[k]
		cx, cy := k.x<<chunkShift, k.y<<chunkShift
		if cx+chunkSize <= r.Min.X || cx >= r.Max.X || cy+chunkSize <= r.Min.Y || cy >= r.Max.Y {
			continue
		}
		for ly, row := range c.on {
			y := cy + ly
			if row == 0 || y < r.Min.Y || y >= r.Max.Y {
				continue
			}
			for row != 0 {
				lx := bits.TrailingZeros64(row)
				row &= row - 1
				if x := cx + lx; x >= r.Min.X && x < r.Max.X {
					d.set(x-r.Min.X, y-r.Min.Y)
				}
			}
		}
	}
	return d
}

func (d *Dense) set(lx, ly int) {
	d.words[ly*d.stride+lx>>6] |= 1 << (lx & 63)
}

func (d *Dense) buildRank() {
	d.rank = make([]int32, len(d.words)+1)
	n := 0
	for i, w := range d.words {
		d.rank[i] = int32(n)
		n += bits.OnesCount64(w)
	}
	d.rank[len(d.words)] = int32(n)
	d.count = n
}

// Get reports whether pixel (x, y), in world coordinates, is on.
func (d *Dense) Get(x, y int) bool {
	lx, ly := x-d.Rect.Min.X, y-d.Rect.Min.Y
	if lx < 0 || ly < 0 || lx >= d.Rect.Dx() || ly >= d.Rect.Dy() {
		return false
	}
	return d.words[ly*d.stride+lx>>6]>>(lx&63)&1 != 0
}

// Count returns the number of on pixels.
func (d *Dense) Count() int { return d.count }

// Kind returns the kind of the on pixel with index i.
func (d *Dense) Kind(i int) Kind { return d.kinds[i] }

// At returns the kind of cell (x, y), in world coordinates.
func (d *Dense) At(x, y int) Kind {
	if i := d.Index(x, y); i >= 0 {
		return d.kinds[i]
	}
	return Empty
}

// Index returns the raster-order index of on pixel (x, y), or -1 if it is off.
func (d *Dense) Index(x, y int) int {
	lx, ly := x-d.Rect.Min.X, y-d.Rect.Min.Y
	if lx < 0 || ly < 0 || lx >= d.Rect.Dx() || ly >= d.Rect.Dy() {
		return -1
	}
	wi := ly*d.stride + lx>>6
	w := d.words[wi]
	bit := uint(lx & 63)
	if w>>bit&1 == 0 {
		return -1
	}
	return int(d.rank[wi]) + bits.OnesCount64(w&(1<<bit-1))
}

// ForEach calls fn for every on pixel in raster order with its index, which
// therefore counts up from 0.
func (d *Dense) ForEach(fn func(i, x, y int)) {
	i := 0
	for ly := range d.Rect.Dy() {
		for k := range d.stride {
			w := d.words[ly*d.stride+k]
			for w != 0 {
				lx := k<<6 + bits.TrailingZeros64(w)
				w &= w - 1
				fn(i, d.Rect.Min.X+lx, d.Rect.Min.Y+ly)
				i++
			}
		}
	}
}
