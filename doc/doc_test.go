package doc

import (
	"image"
	"strings"
	"testing"

	"github.com/Castux/mipsim2/bitmap"
)

// sample builds a small valid document: a 4x3 "cell" definition with a label,
// placed twice in the root (once rotated), plus some root pixels and a label.
func sample() *Document {
	d := New()
	cell := NewDefinition("cell", 4, 3)
	cell.Pixels = bitmap.MustFromRows(
		"#...",
		"###.",
		"...#",
	)
	cell.Labels = []Label{{X: 0, Y: 0, Name: "a"}}
	d.Defs["cell"] = cell

	root := d.RootDef()
	root.Pixels = bitmap.MustFromRows("##")
	root.Labels = []Label{{X: 0, Y: 0, Name: "clock"}}
	root.Instances = []Instance{
		{ID: "i1", Def: "cell", X: 5, Y: 0, Name: "g1"},
		{ID: "i2", Def: "cell", X: 10, Y: 0, Orient: Orient{Rot: 1}, Name: "g2"},
	}
	return d
}

func TestSampleIsValid(t *testing.T) {
	if err := sample().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestInvariantsReject(t *testing.T) {
	cases := []struct {
		name   string
		break_ func(d *Document)
		want   string
	}{
		{"missing root", func(d *Document) { d.Root = "nope" }, "root definition"},
		{"missing def", func(d *Document) { d.RootDef().Instances[0].Def = "nope" }, "does not exist"},
		{"root instanced", func(d *Document) {
			d.Defs["cell"].Instances = []Instance{{ID: "x", Def: "top", Name: "x"}}
		}, "root definition cannot be instanced"},
		{"cycle", func(d *Document) {
			d.Defs["cell"].W, d.Defs["cell"].H = 20, 20
			d.Defs["cell"].Instances = []Instance{{ID: "x", Def: "cell", X: 10, Y: 10, Name: "x"}}
		}, "contain themselves"},
		{"siblings overlap", func(d *Document) { d.RootDef().Instances[1].X = 7 }, "overlaps sibling"},
		{"parent pixel inside child", func(d *Document) { d.RootDef().Pixels.Set(6, 1, true) }, "parent has pixels inside"},
		{"parent label inside child", func(d *Document) {
			d.RootDef().Labels = append(d.RootDef().Labels, Label{X: 5, Y: 2, Name: "x"})
		}, "inside the instance rectangle"},
		{"child outside parent", func(d *Document) {
			outer := NewDefinition("outer", 5, 5)
			outer.Instances = []Instance{{ID: "x", Def: "cell", X: 3, Y: 0, Name: "x"}}
			d.Defs["outer"] = outer
		}, "not inside the parent"},
		{"pixels outside definition", func(d *Document) { d.Defs["cell"].Pixels.Set(4, 0, true) }, "pixels extend outside"},
		{"label outside definition", func(d *Document) {
			d.Defs["cell"].Labels = append(d.Defs["cell"].Labels, Label{X: 0, Y: 3, Name: "b"})
		}, "outside the 4x3"},
		{"duplicate instance name", func(d *Document) { d.RootDef().Instances[1].Name = "g1" }, "duplicate name"},
		{"duplicate instance ID", func(d *Document) { d.RootDef().Instances[1].ID = "i1" }, "duplicate ID"},
		{"dotted instance name", func(d *Document) { d.RootDef().Instances[1].Name = "g.2" }, "invalid name"},
		{"dotted label", func(d *Document) { d.RootDef().Labels[0].Name = "a.b" }, "invalid name"},
		{"two labels on one pixel", func(d *Document) {
			d.RootDef().Labels = append(d.RootDef().Labels, Label{X: 0, Y: 0, Name: "other"})
		}, "same position"},
		{"zero size", func(d *Document) { d.Defs["cell"].W = 0 }, "not positive"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := sample()
			c.break_(d)
			err := d.Validate()
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not mention %q", err, c.want)
			}
		})
	}
}

func TestFlattenPixelsAndNames(t *testing.T) {
	d := sample()
	f := d.Flatten()

	want := bitmap.New()
	want.Set(0, 0, true)
	want.Set(1, 0, true)
	want.Or(bitmap.MustFromRows("#...", "###.", "...#"), 5, 0)
	// cell rotated clockwise becomes 3 wide, 4 tall:
	want.Or(bitmap.MustFromRows(
		".##",
		".#.",
		".#.",
		"#..",
	), 10, 0)
	if !f.Pixels.Equal(want) {
		t.Fatalf("flattened pixels:\n%v\nwant:\n%v", f.Pixels, want)
	}

	wantLabels := []FlatLabel{
		{Pos: image.Pt(0, 0), Name: "clock", Scope: ""},
		{Pos: image.Pt(5, 0), Name: "g1.a", Scope: "g1"},
		{Pos: image.Pt(12, 0), Name: "g2.a", Scope: "g2"},
	}
	if len(f.Labels) != len(wantLabels) {
		t.Fatalf("labels = %v", f.Labels)
	}
	for i := range wantLabels {
		if f.Labels[i] != wantLabels[i] {
			t.Errorf("label %d = %+v, want %+v", i, f.Labels[i], wantLabels[i])
		}
	}
	if got := f.Instances[1].Rect; got != image.Rect(10, 0, 13, 4) {
		t.Errorf("rotated instance rect = %v", got)
	}
}

func TestNestedHierarchicalNames(t *testing.T) {
	d := sample()
	outer := NewDefinition("outer", 10, 10)
	outer.Instances = []Instance{{ID: "i1", Def: "cell", X: 1, Y: 1, Name: "inner"}}
	outer.Labels = []Label{{X: 0, Y: 0, Name: "b"}}
	d.Defs["outer"] = outer
	d.RootDef().Instances = append(d.RootDef().Instances, Instance{ID: "i3", Def: "outer", X: 20, Y: 0, Name: "alu"})
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, l := range d.Flatten().Labels {
		names = append(names, l.Name+"@"+l.Scope)
	}
	got := strings.Join(names, " ")
	want := "clock@ g1.a@g1 g2.a@g2 alu.b@alu alu.inner.a@alu.inner"
	if got != want {
		t.Errorf("names = %s\nwant    %s", got, want)
	}
}

func TestLocate(t *testing.T) {
	d := sample()
	cases := []struct {
		p     image.Point
		def   DefID
		local image.Point
		path  int
	}{
		{image.Pt(0, 0), "top", image.Pt(0, 0), 0},
		{image.Pt(4, 0), "top", image.Pt(4, 0), 0},
		{image.Pt(6, 1), "cell", image.Pt(1, 1), 1},
		{image.Pt(8, 2), "cell", image.Pt(3, 2), 1}, // inside the rectangle, on an off pixel
		{image.Pt(12, 0), "cell", image.Pt(0, 0), 1},
		{image.Pt(10, 3), "cell", image.Pt(3, 2), 1},
	}
	for _, c := range cases {
		loc := d.Locate(c.p)
		if loc.Def != c.def || loc.Local != c.local || len(loc.Path) != c.path {
			t.Errorf("Locate(%v) = %+v, want def %s local %v depth %d", c.p, loc, c.def, c.local, c.path)
		}
		if got := loc.Map.Apply(loc.Local); got != c.p {
			t.Errorf("Locate(%v): Map maps local back to %v", c.p, got)
		}
	}
}

func TestCloneAndEqual(t *testing.T) {
	a := sample()
	b := a.Clone()
	if !a.Equal(b) {
		t.Fatal("clone differs")
	}
	b.Defs["cell"].Pixels.Set(3, 0, true)
	b.RootDef().Instances[0].X = 4
	if a.Equal(b) {
		t.Fatal("modified clone still equal")
	}
	if a.Defs["cell"].Pixels.Get(3, 0) || a.RootDef().Instances[0].X != 5 {
		t.Fatal("modifying the clone changed the original")
	}
}
