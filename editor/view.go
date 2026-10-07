package editor

import (
	"image"
	"math"
)

// zoomLevels are the screen pixels per circuit pixel the view snaps to.
// Above 1 they are integers so pixels stay crisp; below 1 the renderer
// filters (averages) several circuit pixels per screen pixel.
var zoomLevels = []float64{
	1.0 / 32, 1.0 / 24, 1.0 / 16, 1.0 / 12, 1.0 / 8, 1.0 / 6, 1.0 / 4, 1.0 / 3, 1.0 / 2,
	1, 2, 3, 4, 6, 8, 12, 16, 24, 32, 48, 64,
}

// View maps between screen and world (circuit) coordinates: screen =
// world*Scale + Offset.
type View struct {
	Scale  float64
	Offset [2]float64
	level  int
}

// NewView returns a view at the given zoom level index into zoomLevels,
// with world (0,0) at the screen origin.
func NewView() View {
	v := View{}
	v.setLevel(indexOf(8))
	return v
}

func indexOf(scale float64) int {
	for i, z := range zoomLevels {
		if z == scale {
			return i
		}
	}
	return 0
}

func (v *View) setLevel(i int) {
	v.level = max(0, min(i, len(zoomLevels)-1))
	v.Scale = zoomLevels[v.level]
}

// ToWorld returns the world pixel under a screen position.
func (v *View) ToWorld(sx, sy float64) image.Point {
	return image.Pt(int(math.Floor((sx-v.Offset[0])/v.Scale)), int(math.Floor((sy-v.Offset[1])/v.Scale)))
}

// ToScreen returns the screen position of a world pixel's top-left corner.
func (v *View) ToScreen(p image.Point) (float64, float64) {
	return float64(p.X)*v.Scale + v.Offset[0], float64(p.Y)*v.Scale + v.Offset[1]
}

// Pan moves the view by a screen-space delta.
func (v *View) Pan(dx, dy float64) {
	v.Offset[0] += dx
	v.Offset[1] += dy
}

// Zoom steps the zoom level by n (positive zooms in) keeping the world point
// under the screen position (sx, sy) fixed.
func (v *View) Zoom(n int, sx, sy float64) {
	wx := (sx - v.Offset[0]) / v.Scale
	wy := (sy - v.Offset[1]) / v.Scale
	v.setLevel(v.level + n)
	v.Offset[0] = math.Round(sx - wx*v.Scale)
	v.Offset[1] = math.Round(sy - wy*v.Scale)
}

// Fit centres the rectangle r in a screen of size w×h at the largest zoom
// level that shows all of it.
func (v *View) Fit(r image.Rectangle, w, h float64) {
	if r.Empty() {
		r = image.Rect(-8, -8, 8, 8)
	}
	r = r.Inset(-2)
	i := 0
	for j, z := range zoomLevels {
		if float64(r.Dx())*z <= w && float64(r.Dy())*z <= h {
			i = j
		}
	}
	v.setLevel(i)
	cx, cy := float64(r.Min.X+r.Max.X)/2, float64(r.Min.Y+r.Max.Y)/2
	v.Offset[0] = math.Round(w/2 - cx*v.Scale)
	v.Offset[1] = math.Round(h/2 - cy*v.Scale)
}
