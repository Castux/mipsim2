package editor

import "image"

// line4 returns the cells from a to b (both included) as a 4-connected path:
// consecutive cells share an edge. Drawing with 8-connected steps would leave
// diagonal joints, and diagonals never connect.
func line4(a, b image.Point) []image.Point {
	dx, dy := abs(b.X-a.X), abs(b.Y-a.Y)
	sx, sy := sign(b.X-a.X), sign(b.Y-a.Y)
	pts := []image.Point{a}
	p := a
	// Walk the grid, choosing at each step the axis whose next crossing of the
	// ideal line comes first (ties go to x, deterministically).
	ix, iy := 0, 0
	for ix < dx || iy < dy {
		// Compare (ix+0.5)/dx with (iy+0.5)/dy without division.
		if iy == dy || (ix < dx && (2*ix+1)*dy <= (2*iy+1)*dx) {
			p.X += sx
			ix++
		} else {
			p.Y += sy
			iy++
		}
		pts = append(pts, p)
	}
	return pts
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func sign(x int) int {
	switch {
	case x > 0:
		return 1
	case x < 0:
		return -1
	}
	return 0
}
