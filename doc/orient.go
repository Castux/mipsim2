package doc

import (
	"fmt"
	"image"
)

// Orient is one of the 8 rotations and mirrorings of the square. Applying it
// to a W×H rectangle first mirrors left-right if Flip is set (x -> W-1-x), then
// rotates clockwise by Rot quarter turns. The result is again placed with its
// top-left corner at the origin.
type Orient struct {
	Flip bool
	Rot  uint8 // 0..3
}

// Identity is the orientation that changes nothing.
var Identity = Orient{}

// AllOrients lists the 8 orientations in a fixed order.
var AllOrients = func() []Orient {
	var all []Orient
	for _, f := range []bool{false, true} {
		for r := range uint8(4) {
			all = append(all, Orient{Flip: f, Rot: r})
		}
	}
	return all
}()

// matrix is a signed permutation matrix acting on (x, y) with y pointing down.
type matrix struct{ a, b, c, d int } // x' = a*x + b*y; y' = c*x + d*y

func (m matrix) mul(n matrix) matrix { // m·n: apply n first, then m
	return matrix{
		m.a*n.a + m.b*n.c, m.a*n.b + m.b*n.d,
		m.c*n.a + m.d*n.c, m.c*n.b + m.d*n.d,
	}
}

var (
	flipM = matrix{-1, 0, 0, 1}
	rotM  = matrix{0, -1, 1, 0} // clockwise on screen: right -> down -> left -> up
)

func (o Orient) matrix() matrix {
	m := matrix{1, 0, 0, 1}
	if o.Flip {
		m = flipM
	}
	for range o.Rot % 4 {
		m = rotM.mul(m)
	}
	return m
}

func orientOf(m matrix) Orient {
	for _, o := range AllOrients {
		if o.matrix() == m {
			return o
		}
	}
	panic(fmt.Sprintf("doc: %v is not an orientation matrix", m))
}

// Then returns the orientation that applies o first, then p.
func (o Orient) Then(p Orient) Orient { return orientOf(p.matrix().mul(o.matrix())) }

// Inverse returns the orientation that undoes o.
func (o Orient) Inverse() Orient {
	m := o.matrix()
	return orientOf(matrix{m.a, m.c, m.b, m.d}) // transpose
}

// Size returns the size of a w×h rectangle after applying o.
func (o Orient) Size(w, h int) (int, int) {
	if o.Rot%2 == 1 {
		return h, w
	}
	return w, h
}

// Affine returns the map from local cell coordinates of a w×h rectangle to
// coordinates in the oriented rectangle, whose top-left cell is (0, 0).
func (o Orient) Affine(w, h int) Affine {
	m := o.matrix()
	// Shift so the smallest transformed cell coordinate is 0.
	tx := -(min(m.a, 0)*(w-1) + min(m.b, 0)*(h-1))
	ty := -(min(m.c, 0)*(w-1) + min(m.d, 0)*(h-1))
	return Affine{m: m, tx: tx, ty: ty}
}

// String returns the file-format name: r0, r90, r180, r270, f0, f90, f180, f270.
func (o Orient) String() string {
	p := "r"
	if o.Flip {
		p = "f"
	}
	return fmt.Sprintf("%s%d", p, int(o.Rot%4)*90)
}

// ParseOrient parses the file-format name of an orientation. The empty string
// is the identity.
func ParseOrient(s string) (Orient, error) {
	if s == "" {
		return Identity, nil
	}
	for _, o := range AllOrients {
		if o.String() == s {
			return o, nil
		}
	}
	return Identity, fmt.Errorf("unknown orientation %q (want r0, r90, r180, r270, f0, f90, f180 or f270)", s)
}

// Affine is an integer map p -> M·p + t where M is an orientation matrix. It
// maps cell coordinates (not corners) between frames.
type Affine struct {
	m      matrix
	tx, ty int
}

// IdentityAffine maps every point to itself.
var IdentityAffine = Affine{m: matrix{1, 0, 0, 1}}

// Translate returns the pure translation by (dx, dy).
func Translate(dx, dy int) Affine { return Affine{m: matrix{1, 0, 0, 1}, tx: dx, ty: dy} }

// Apply maps a point.
func (f Affine) Apply(p image.Point) image.Point {
	return image.Pt(f.m.a*p.X+f.m.b*p.Y+f.tx, f.m.c*p.X+f.m.d*p.Y+f.ty)
}

// Then returns the map that applies f first, then g.
func (f Affine) Then(g Affine) Affine {
	t := g.Apply(image.Pt(f.tx, f.ty))
	return Affine{m: g.m.mul(f.m), tx: t.X, ty: t.Y}
}

// Inverse returns the map that undoes f.
func (f Affine) Inverse() Affine {
	inv := matrix{f.m.a, f.m.c, f.m.b, f.m.d}
	g := Affine{m: inv}
	t := g.Apply(image.Pt(f.tx, f.ty))
	g.tx, g.ty = -t.X, -t.Y
	return g
}

// Orient returns the orientation part of f.
func (f Affine) Orient() Orient { return orientOf(f.m) }

// ApplyRect maps a rectangle of cells to the rectangle covering their images.
func (f Affine) ApplyRect(r image.Rectangle) image.Rectangle {
	if r.Empty() {
		return image.Rectangle{}
	}
	a := f.Apply(r.Min)
	b := f.Apply(r.Max.Sub(image.Pt(1, 1)))
	return image.Rect(min(a.X, b.X), min(a.Y, b.Y), max(a.X, b.X)+1, max(a.Y, b.Y)+1)
}
