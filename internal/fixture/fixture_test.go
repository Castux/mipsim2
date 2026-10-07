package fixture

import (
	"image"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Castux/mipsim2/bitmap"
	"github.com/Castux/mipsim2/doc"
)

func TestInverterFixture(t *testing.T) {
	f, err := ParseFile(filepath.Join("..", "..", "testdata", "doc", "inverter.fix"))
	if err != nil {
		t.Fatal(err)
	}
	root := f.Doc.RootDef()
	want := bitmap.MustFromRows(
		"###",
		"###",
		"###",
		".#.",
		"#########",
		".#.",
		"##.",
		".#.",
		"###",
		"#.#",
		"###",
	)
	if !root.Pixels.Equal(want) {
		t.Fatalf("pixels:\n%v", root.Pixels)
	}
	if got := f.Inputs; len(got) != 1 || got[0] != "in" {
		t.Errorf("inputs = %v", got)
	}
	if got := f.Outputs; len(got) != 1 || got[0] != "out" {
		t.Errorf("outputs = %v", got)
	}
	if len(root.Labels) != 2 || root.Labels[0] != (doc.Label{X: 0, Y: 6, Name: "in"}) {
		t.Errorf("labels = %v", root.Labels)
	}
	exp := f.Find("expect")
	if len(exp) != 2 || strings.Join(exp[1].Args, " ") != "in=high out=low" {
		t.Errorf("expect directives = %+v", exp)
	}
	if title := f.Directives[0]; title.Word != "inverter.fix" || title.Line != 1 {
		t.Errorf("first directive = %+v", title)
	}
}

func TestDefinitionsAndPlacement(t *testing.T) {
	f, err := Parse(`
# def buf 3x2
###
#..
# label a 0,0
# root
#....
# place buf b1 2,0
# place buf b2 2,3 r90
.....
`)
	if err != nil {
		t.Fatal(err)
	}
	d := f.Doc
	buf := d.Defs["buf"]
	if buf.W != 3 || buf.H != 2 || buf.Pixels.Count() != 4 {
		t.Fatalf("buf = %+v\n%v", buf, buf.Pixels)
	}
	root := d.RootDef()
	if root.Pixels.Count() != 1 || !root.Pixels.Get(0, 0) {
		t.Errorf("root pixels:\n%v", root.Pixels)
	}
	if len(root.Instances) != 2 || root.Instances[1].Orient != (doc.Orient{Rot: 1}) || root.Instances[1].ID != "i2" {
		t.Errorf("instances = %+v", root.Instances)
	}
	flat := d.Flatten()
	if !flat.Pixels.Get(4, 0) || flat.Labels[0].Name != "b1.a" || flat.Labels[1].Pos != image.Pt(3, 3) {
		t.Errorf("flat:\n%v\nlabels %+v", flat.Pixels, flat.Labels)
	}
}

func TestLineAndPixel(t *testing.T) {
	f, err := Parse("# line 0,0 3,0\n# line 3,1 3,2\n# px -1,5")
	if err != nil {
		t.Fatal(err)
	}
	want := bitmap.MustFromRows("####", "...#", "...#")
	want.Set(-1, 5, true)
	if !f.Doc.RootDef().Pixels.Equal(want) {
		t.Errorf("pixels:\n%v", f.Doc.RootDef().Pixels)
	}
	if _, err := Parse("# line 0,0 2,2"); err == nil || !strings.Contains(err.Error(), "horizontal or vertical") {
		t.Errorf("diagonal line: %v", err)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{"#x#", "not a drawing row"},
		{"# def a 3by2", "bad size"},
		{"# def a 2x1\n###", "pixels extend outside"},
		{"# def a 2x1\n##\n##", "2 rows drawn for height 1"},
		{"# label in 0", "want X,Y"},
		{"# place nope g 0,0", "does not exist"},
		{"# def a 1x1\n# def a 1x1", "declared twice"},
		{"# def a 1x1\n#\n# root\n# place a g 0,0 r45", "unknown orientation"},
	}
	for _, c := range cases {
		_, err := Parse(c.src)
		if err == nil {
			t.Errorf("%q: accepted", c.src)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: error %q does not mention %q", c.src, err, c.want)
		}
	}
}
