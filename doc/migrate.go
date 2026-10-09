package doc

import (
	"image"

	"github.com/Castux/mipsim2/bitmap"
)

// MigratePatterns turns a one-bit drawing in the pattern language of format
// versions 1 and 2 (thin-wires rules) into typed cells:
//
//   - a thick region that is exactly a 3×3 square becomes power cells;
//   - a 3×3 ring of on pixels around an off centre becomes ground cells;
//   - an on pixel outside sources and 2×2 blocks with exactly three on
//     orthogonal neighbours becomes a transistor;
//   - an off pixel with four on orthogonal neighbours and no on diagonal one
//     becomes a bridge;
//   - every other on pixel is a wire, including malformed thick regions,
//     which were errors.
//
// Classification is local to the bitmap, so patterns split across an
// instance edge are not recognised; they were rare.
func MigratePatterns(px *bitmap.Bitmap) *bitmap.Bitmap {
	on := px.Get
	out := bitmap.New()
	source := map[image.Point]bitmap.Kind{}

	// 3×3 squares whose thick region is exactly that square.
	block := func(x, y int) bool { return on(x, y) && on(x+1, y) && on(x, y+1) && on(x+1, y+1) }
	px.ForEach(func(x, y int) {
		for dy := 0; dy < 3; dy++ {
			for dx := 0; dx < 3; dx++ {
				if !on(x+dx, y+dy) {
					return
				}
			}
		}
		// No 2×2 block may stick out of the square.
		for by := y - 1; by <= y+2; by++ {
			for bx := x - 1; bx <= x+2; bx++ {
				inside := bx >= x && bx <= x+1 && by >= y && by <= y+1
				if !inside && block(bx, by) {
					return
				}
			}
		}
		for dy := 0; dy < 3; dy++ {
			for dx := 0; dx < 3; dx++ {
				source[image.Pt(x+dx, y+dy)] = bitmap.Power
			}
		}
	})
	// Rings: eight on pixels around an off centre.
	px.ForEach(func(x, y int) {
		if on(x+1, y+1) {
			return
		}
		for dy := 0; dy < 3; dy++ {
			for dx := 0; dx < 3; dx++ {
				if (dx != 1 || dy != 1) && !on(x+dx, y+dy) {
					return
				}
			}
		}
		for dy := 0; dy < 3; dy++ {
			for dx := 0; dx < 3; dx++ {
				p := image.Pt(x+dx, y+dy)
				if (dx != 1 || dy != 1) && source[p] == bitmap.Empty {
					source[p] = bitmap.Ground
				}
			}
		}
	})
	orth := [4]image.Point{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}
	px.ForEach(func(x, y int) {
		if k := source[image.Pt(x, y)]; k != bitmap.Empty {
			out.Put(x, y, k)
			return
		}
		thick := block(x-1, y-1) || block(x, y-1) || block(x-1, y) || block(x, y)
		n := 0
		for _, d := range orth {
			if on(x+d.X, y+d.Y) {
				n++
			}
		}
		if n == 3 && !thick {
			out.Put(x, y, bitmap.Transistor)
		} else {
			out.Put(x, y, bitmap.Wire)
		}
		// Bridge gaps: each is visited from its north arm.
		gx, gy := x, y+1
		if on(gx, gy) || !on(gx, gy+1) || !on(gx-1, gy) || !on(gx+1, gy) {
			return
		}
		for _, d := range [4]image.Point{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
			if on(gx+d.X, gy+d.Y) {
				return
			}
		}
		out.Put(gx, gy, bitmap.Bridge)
	})
	return out
}

// hasKinds reports whether any cell is not a wire.
func hasKinds(px *bitmap.Bitmap) bool {
	found := false
	px.ForEachCell(func(_, _ int, k bitmap.Kind) {
		if k != bitmap.Wire {
			found = true
		}
	})
	return found
}
