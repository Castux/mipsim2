package doc

import (
	"image"
	"testing"
)

// reference applies o to cell (x, y) of a w×h rectangle step by step, exactly
// as the spec words it: mirror first, then clockwise quarter turns.
func reference(o Orient, w, h, x, y int) (int, int) {
	if o.Flip {
		x = w - 1 - x
	}
	for range o.Rot {
		// Clockwise turn of a w×h rectangle gives an h×w rectangle.
		x, y = h-1-y, x
		w, h = h, w
	}
	return x, y
}

func TestOrientMatchesReference(t *testing.T) {
	for _, o := range AllOrients {
		for _, sz := range [][2]int{{1, 1}, {3, 2}, {2, 5}, {4, 4}} {
			w, h := sz[0], sz[1]
			f := o.Affine(w, h)
			pw, ph := o.Size(w, h)
			for y := range h {
				for x := range w {
					rx, ry := reference(o, w, h, x, y)
					got := f.Apply(image.Pt(x, y))
					if got != image.Pt(rx, ry) {
						t.Fatalf("%v on %dx%d: cell (%d,%d) -> %v, want (%d,%d)", o, w, h, x, y, got, rx, ry)
					}
					if !got.In(image.Rect(0, 0, pw, ph)) {
						t.Fatalf("%v on %dx%d: cell (%d,%d) -> %v, outside %dx%d", o, w, h, x, y, got, pw, ph)
					}
				}
			}
		}
	}
}

func TestOrientThenAndInverse(t *testing.T) {
	const w, h = 3, 5
	for _, a := range AllOrients {
		if a.Then(a.Inverse()) != Identity || a.Inverse().Then(a) != Identity {
			t.Errorf("%v: inverse does not cancel", a)
		}
		for _, b := range AllOrients {
			ab := a.Then(b)
			fa := a.Affine(w, h)
			aw, ah := a.Size(w, h)
			fb := b.Affine(aw, ah)
			fab := ab.Affine(w, h)
			for y := range h {
				for x := range w {
					p := image.Pt(x, y)
					if got, want := fab.Apply(p), fb.Apply(fa.Apply(p)); got != want {
						t.Fatalf("%v then %v = %v: cell %v -> %v, want %v", a, b, ab, p, got, want)
					}
				}
			}
			if got := fa.Then(fb).Orient(); got != ab {
				t.Errorf("Affine.Then orientation %v, want %v", got, ab)
			}
		}
	}
}

func TestFourTurnsAndTwoFlipsAreIdentity(t *testing.T) {
	r := Orient{Rot: 1}
	if r.Then(r).Then(r).Then(r) != Identity {
		t.Error("four quarter turns are not the identity")
	}
	f := Orient{Flip: true}
	if f.Then(f) != Identity {
		t.Error("two flips are not the identity")
	}
}

func TestAffineInverseAndRect(t *testing.T) {
	f := Orient{Flip: true, Rot: 3}.Affine(4, 7).Then(Translate(10, -3))
	g := f.Inverse()
	for y := -5; y < 5; y++ {
		for x := -5; x < 5; x++ {
			p := image.Pt(x, y)
			if got := g.Apply(f.Apply(p)); got != p {
				t.Fatalf("inverse: %v -> %v", p, got)
			}
		}
	}
	r := f.ApplyRect(image.Rect(0, 0, 4, 7))
	if r != image.Rect(10, -3, 17, 1) {
		t.Errorf("ApplyRect = %v, want (10,-3)-(17,1)", r)
	}
}

func TestOrientNames(t *testing.T) {
	seen := map[string]bool{}
	for _, o := range AllOrients {
		s := o.String()
		if seen[s] {
			t.Errorf("duplicate name %s", s)
		}
		seen[s] = true
		back, err := ParseOrient(s)
		if err != nil || back != o {
			t.Errorf("ParseOrient(%q) = %v, %v", s, back, err)
		}
	}
	if o, err := ParseOrient(""); err != nil || o != Identity {
		t.Error("empty orientation should parse as identity")
	}
	if _, err := ParseOrient("r45"); err == nil {
		t.Error("r45 accepted")
	}
}
