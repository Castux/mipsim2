package bitmap

import (
	"image"
	"math/bits"
)

// Dense is a fixed rectangle of pixels stored as packed rows, for code that
// scans every pixel and its neighbours (the pattern compiler). Pixels outside
// Rect read as off.
type Dense struct {
	Rect   image.Rectangle
	stride int // words per row
	words  []uint64
}

// ToDense copies the pixels inside r into a Dense.
func (b *Bitmap) ToDense(r image.Rectangle) *Dense {
	r = r.Canon()
	d := &Dense{Rect: r, stride: (r.Dx() + 63) / 64}
	d.words = make([]uint64, d.stride*r.Dy())
	if r.Empty() {
		return d
	}
	for _, k := range b.keys {
		c := b.chunks[k]
		cx, cy := k.x<<chunkShift, k.y<<chunkShift
		if cx+chunkSize <= r.Min.X || cx >= r.Max.X || cy+chunkSize <= r.Min.Y || cy >= r.Max.Y {
			continue
		}
		for ly, row := range c {
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

// Get reports whether pixel (x, y), in world coordinates, is on.
func (d *Dense) Get(x, y int) bool {
	lx, ly := x-d.Rect.Min.X, y-d.Rect.Min.Y
	if lx < 0 || ly < 0 || lx >= d.Rect.Dx() || ly >= d.Rect.Dy() {
		return false
	}
	return d.words[ly*d.stride+lx>>6]>>(lx&63)&1 != 0
}
