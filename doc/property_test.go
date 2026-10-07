package doc

import (
	"fmt"
	"image"
	"math/rand/v2"
	"testing"
)

// randomDoc builds a valid random document: a few leaf definitions with random
// pixels, middle definitions that place leaves, and a root that places both,
// all with random orientations and non-overlapping rectangles.
func randomDoc(rng *rand.Rand) *Document {
	d := New()
	var leaves, middles []DefID

	for i := range 1 + rng.IntN(3) {
		def := NewDefinition(DefID(fmt.Sprintf("leaf%d", i)), 1+rng.IntN(7), 1+rng.IntN(7))
		for y := range def.H {
			for x := range def.W {
				if rng.IntN(2) == 0 {
					def.Pixels.Set(x, y, true)
				}
			}
		}
		d.Defs[def.ID] = def
		leaves = append(leaves, def.ID)
	}

	place := func(parent *Definition, choices []DefID, n int, area image.Rectangle) {
		var taken []image.Rectangle
		for k := 0; k < n; k++ {
			for range 30 { // a few attempts to find a free spot
				inst := Instance{
					ID:     InstID(fmt.Sprintf("i%d", len(parent.Instances))),
					Def:    choices[rng.IntN(len(choices))],
					Orient: AllOrients[rng.IntN(8)],
					Name:   fmt.Sprintf("n%d", len(parent.Instances)),
				}
				r0 := d.PlacedRect(inst)
				if r0.Dx() > area.Dx() || r0.Dy() > area.Dy() {
					continue
				}
				inst.X = area.Min.X + rng.IntN(area.Dx()-r0.Dx()+1)
				inst.Y = area.Min.Y + rng.IntN(area.Dy()-r0.Dy()+1)
				r := d.PlacedRect(inst)
				free := true
				for _, t := range taken {
					if t.Overlaps(r) {
						free = false
					}
				}
				if free {
					taken = append(taken, r)
					parent.Instances = append(parent.Instances, inst)
					break
				}
			}
		}
		// Parent pixels only outside child rectangles.
		for range area.Dx() * area.Dy() / 3 {
			p := image.Pt(area.Min.X+rng.IntN(area.Dx()), area.Min.Y+rng.IntN(area.Dy()))
			inside := false
			for _, t := range taken {
				if p.In(t) {
					inside = true
				}
			}
			if !inside {
				parent.Pixels.Set(p.X, p.Y, true)
			}
		}
	}

	for i := range 1 + rng.IntN(2) {
		def := NewDefinition(DefID(fmt.Sprintf("mid%d", i)), 12+rng.IntN(10), 12+rng.IntN(10))
		d.Defs[def.ID] = def
		place(def, leaves, 1+rng.IntN(4), def.Rect())
		middles = append(middles, def.ID)
	}
	place(d.RootDef(), append(append([]DefID{}, leaves...), middles...), 2+rng.IntN(6), image.Rect(-40, -40, 40, 40))
	return d
}

// pixelCount is the number of world pixels a definition contributes, counting
// all instances below it.
func pixelCount(d *Document, id DefID) int {
	def := d.Defs[id]
	n := def.Pixels.Count()
	for _, inst := range def.Instances {
		n += pixelCount(d, inst.Def)
	}
	return n
}

// TestFlattenResplit checks that flattening then re-splitting random instance
// trees preserves pixels: every world pixel resolves, through Locate, to an
// on pixel of the definition it came from, and the number of world pixels
// equals the sum over all placements (so no two placements overlap).
func TestFlattenResplit(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 7))
	for iter := range 300 {
		d := randomDoc(rng)
		if err := d.Validate(); err != nil {
			t.Fatalf("iteration %d: random document invalid: %v", iter, err)
		}
		f := d.Flatten()
		if got, want := f.Pixels.Count(), pixelCount(d, d.Root); got != want {
			t.Fatalf("iteration %d: %d world pixels, want %d", iter, got, want)
		}
		f.Pixels.ForEach(func(x, y int) {
			loc := d.Locate(image.Pt(x, y))
			if !d.Defs[loc.Def].Pixels.Get(loc.Local.X, loc.Local.Y) {
				t.Fatalf("iteration %d: world pixel (%d,%d) resolves to %s %v, which is off", iter, x, y, loc.Def, loc.Local)
			}
		})
	}
}

// TestSaveLoadRandom round-trips random documents through the file format.
func TestSaveLoadRandom(t *testing.T) {
	rng := rand.New(rand.NewPCG(8, 8))
	for iter := range 100 {
		d := randomDoc(rng)
		data, err := d.Save()
		if err != nil {
			t.Fatalf("iteration %d: save: %v", iter, err)
		}
		back, err := Load(data)
		if err != nil {
			t.Fatalf("iteration %d: load: %v\n%s", iter, err, data)
		}
		if !d.Equal(back) {
			t.Fatalf("iteration %d: round trip changed the document\n%s", iter, data)
		}
	}
}
