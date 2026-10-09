package bitmap

import (
	"image"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

func TestSetGetNegativeAndChunkEdges(t *testing.T) {
	b := New()
	pts := []image.Point{{0, 0}, {-1, -1}, {63, 63}, {64, 64}, {-64, 0}, {-65, 7}, {1000, -1000}}
	for _, p := range pts {
		b.Set(p.X, p.Y, true)
	}
	for _, p := range pts {
		if !b.Get(p.X, p.Y) {
			t.Errorf("pixel %v not set", p)
		}
		if b.Get(p.X+1, p.Y) && !slices.Contains(pts, image.Pt(p.X+1, p.Y)) {
			t.Errorf("neighbour of %v unexpectedly set", p)
		}
	}
	if got := b.Count(); got != len(pts) {
		t.Errorf("Count = %d, want %d", got, len(pts))
	}
	b.Set(-1, -1, false)
	if b.Get(-1, -1) {
		t.Error("pixel (-1,-1) still set after clearing")
	}
	before := len(b.keys)
	b.Set(5000, 5000, false) // clearing in an absent chunk must not create it
	if len(b.keys) != before {
		t.Errorf("clearing an absent pixel created a chunk")
	}
}

func randomBitmap(rng *rand.Rand, n, spread int) (*Bitmap, map[image.Point]bool) {
	b := New()
	set := map[image.Point]bool{}
	for range n {
		p := image.Pt(rng.IntN(2*spread)-spread, rng.IntN(2*spread)-spread)
		b.Set(p.X, p.Y, true)
		set[p] = true
	}
	return b, set
}

func TestForEachIsRasterOrder(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 1))
	b, set := randomBitmap(rng, 5000, 300)
	var want []image.Point
	for p := range set {
		want = append(want, p)
	}
	slices.SortFunc(want, func(a, b image.Point) int {
		if a.Y != b.Y {
			return a.Y - b.Y
		}
		return a.X - b.X
	})
	var got []image.Point
	b.ForEach(func(x, y int) { got = append(got, image.Pt(x, y)) })
	if !slices.Equal(got, want) {
		t.Fatalf("ForEach order differs: got %d points, want %d", len(got), len(want))
	}
}

func TestBounds(t *testing.T) {
	b := New()
	if !b.Bounds().Empty() {
		t.Error("empty bitmap has non-empty bounds")
	}
	b.Set(-70, 3, true)
	b.Set(130, -2, true)
	b.Set(5, 200, true)
	want := image.Rect(-70, -2, 131, 201)
	if got := b.Bounds(); got != want {
		t.Errorf("Bounds = %v, want %v", got, want)
	}
	b.Set(130, -2, false)
	if got, want := b.Bounds(), image.Rect(-70, 3, 6, 201); got != want {
		t.Errorf("after clear, Bounds = %v, want %v", got, want)
	}
}

func TestEqualIgnoresEmptyChunks(t *testing.T) {
	a := MustFromRows("#.#", ".#.")
	b := a.Clone()
	b.Set(500, 500, true)
	b.Set(500, 500, false)
	if !a.Equal(b) || !b.Equal(a) {
		t.Error("bitmaps differing only by an empty chunk compare unequal")
	}
	b.Compact()
	if !a.Equal(b) {
		t.Error("Compact changed the pixels")
	}
	b.Set(1, 0, true)
	if a.Equal(b) || b.Equal(a) {
		t.Error("different bitmaps compare equal")
	}
}

func TestCloneIsIndependent(t *testing.T) {
	a := MustFromRows("##")
	b := a.Clone()
	b.Set(0, 0, false)
	if !a.Get(0, 0) {
		t.Error("modifying the clone changed the original")
	}
}

func TestRowsRoundTrip(t *testing.T) {
	rows := []string{"..#", "", "#.#.#", "."}
	b, err := FromRows(rows...)
	if err != nil {
		t.Fatal(err)
	}
	got := b.EncodeRows(image.Rect(0, 0, 10, 10))
	want := []string{"..#", "", "#.#.#"}
	if !slices.Equal(got, want) {
		t.Errorf("EncodeRows = %q, want %q", got, want)
	}
	if _, err := FromRows("#x#"); err == nil {
		t.Error("bad character accepted")
	}
}

func TestEncodeRowsOffsetAndClip(t *testing.T) {
	b := New()
	b.Set(-3, -3, true)
	b.Set(-1, -2, true)
	b.Set(10, 10, true) // outside, ignored
	got := b.EncodeRows(image.Rect(-3, -3, 2, 2))
	want := []string{"#", "..#"}
	if !slices.Equal(got, want) {
		t.Errorf("EncodeRows = %q, want %q", got, want)
	}
}

func TestOrCropClearAnyIn(t *testing.T) {
	src := MustFromRows("##", "#.")
	b := New()
	b.Or(src, 100, -5)
	if !b.Get(100, -5) || !b.Get(101, -5) || !b.Get(100, -4) || b.Get(101, -4) {
		t.Errorf("Or placed pixels wrongly:\n%v", b)
	}
	r := image.Rect(100, -5, 101, -3)
	c := b.Crop(r)
	if c.Count() != 2 || !c.Get(100, -5) || !c.Get(100, -4) {
		t.Errorf("Crop wrong:\n%v", c)
	}
	if !b.AnyIn(r) || b.AnyIn(image.Rect(101, -4, 200, 0)) {
		t.Error("AnyIn wrong")
	}
	b.ClearRect(r)
	if b.Count() != 1 || !b.Get(101, -5) {
		t.Errorf("ClearRect wrong:\n%v", b)
	}
}

func TestAnyInMatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewPCG(2, 2))
	b, set := randomBitmap(rng, 300, 150)
	for range 2000 {
		x0, y0 := rng.IntN(400)-200, rng.IntN(400)-200
		r := image.Rect(x0, y0, x0+rng.IntN(80), y0+rng.IntN(80))
		want := false
		for p := range set {
			if p.In(r) {
				want = true
				break
			}
		}
		if got := b.AnyIn(r); got != want {
			t.Fatalf("AnyIn(%v) = %v, want %v", r, got, want)
		}
	}
}

func TestDenseMatchesBitmap(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 3))
	b, _ := randomBitmap(rng, 4000, 200)
	r := image.Rect(-150, -170, 130, 90)
	d := b.ToDense(r)
	for y := r.Min.Y - 2; y < r.Max.Y+2; y++ {
		for x := r.Min.X - 2; x < r.Max.X+2; x++ {
			want := b.Get(x, y) && image.Pt(x, y).In(r)
			if got := d.Get(x, y); got != want {
				t.Fatalf("Dense.Get(%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

func TestDenseIndex(t *testing.T) {
	rng := rand.New(rand.NewPCG(4, 4))
	b, _ := randomBitmap(rng, 3000, 120)
	r := b.Bounds()
	d := b.ToDense(r)
	if d.Count() != b.Count() {
		t.Fatalf("Count = %d, want %d", d.Count(), b.Count())
	}
	next := 0
	b.ForEach(func(x, y int) {
		if got := d.Index(x, y); got != next {
			t.Fatalf("Index(%d,%d) = %d, want %d", x, y, got, next)
		}
		next++
	})
	if d.Index(r.Min.X-1, r.Min.Y) != -1 {
		t.Error("index outside the rectangle is not -1")
	}
	for range 1000 {
		x, y := r.Min.X+rng.IntN(r.Dx()), r.Min.Y+rng.IntN(r.Dy())
		if !b.Get(x, y) && d.Index(x, y) != -1 {
			t.Fatalf("off pixel (%d,%d) has an index", x, y)
		}
	}
	want := 0
	d.ForEach(func(i, x, y int) {
		if i != want || d.Index(x, y) != i {
			t.Fatalf("Dense.ForEach gave index %d at (%d,%d), want %d", i, x, y, want)
		}
		want++
	})
	if want != d.Count() {
		t.Fatalf("Dense.ForEach visited %d pixels, want %d", want, d.Count())
	}
}

func BenchmarkSetRaster1M(b *testing.B) {
	for range b.N {
		bm := New()
		for y := range 1000 {
			for x := range 1000 {
				bm.Set(x, y, true)
			}
		}
	}
}

func BenchmarkForEach1M(b *testing.B) {
	bm := New()
	for y := range 1000 {
		for x := range 1000 {
			bm.Set(x, y, true)
		}
	}
	b.ResetTimer()
	for range b.N {
		n := 0
		bm.ForEach(func(int, int) { n++ })
	}
}

func TestOrWithItself(t *testing.T) {
	b := MustFromRows("#")
	b.Or(b, 0, 1)
	if got := b.EncodeRows(b.Bounds()); strings.Join(got, "/") != "#/#" {
		t.Errorf("b | b shifted: %v", got)
	}
}

func TestKinds(t *testing.T) {
	b := New()
	b.Put(1, 1, Transistor)
	b.Put(70, -3, Power) // another chunk
	b.Set(2, 1, true)
	for _, c := range []struct {
		x, y int
		want Kind
	}{{1, 1, Transistor}, {70, -3, Power}, {2, 1, Wire}, {0, 0, Empty}} {
		if got := b.At(c.x, c.y); got != c.want {
			t.Errorf("At(%d,%d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
	// Overwriting and erasing.
	b.Put(1, 1, Ground)
	b.Put(2, 1, Bridge)
	b.Put(2, 1, Wire)
	if b.At(1, 1) != Ground || b.At(2, 1) != Wire {
		t.Errorf("after overwrite: %v %v", b.At(1, 1), b.At(2, 1))
	}
	c := b.Clone()
	c.Put(1, 1, Power)
	if b.At(1, 1) != Ground || b.Equal(c) || c.Equal(b) {
		t.Error("clone shares kinds, or Equal ignores them")
	}
	// Erasing a typed cell then setting a wire there must not resurrect the type.
	b.Put(1, 1, Empty)
	b.Set(1, 1, true)
	if b.At(1, 1) != Wire {
		t.Errorf("re-set cell is %v", b.At(1, 1))
	}
	// Rows round trip with every kind.
	r := MustFromRows(".#HLTB", "B..#")
	if got := strings.Join(r.EncodeRows(r.Bounds().Union(image.Rect(0, 0, 1, 1))), "|"); got != ".#HLTB|B..#" {
		t.Errorf("rows %q", got)
	}
	d := r.ToDense(r.Bounds())
	if d.At(4, 0) != Transistor || d.Kind(d.Index(0, 1)) != Bridge || d.At(1, 1) != Empty {
		t.Error("dense kinds wrong")
	}
	if _, err := FromRows("#x"); err == nil {
		t.Error("bad character accepted")
	}
}
