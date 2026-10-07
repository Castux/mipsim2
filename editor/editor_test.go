package editor

import (
	"image"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Castux/mipsim2/bitmap"
	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/internal/fixture"
	"github.com/Castux/mipsim2/sim"
)

func pt(x, y int) image.Point { return image.Pt(x, y) }

// drag presses at the first point, moves through the rest and releases.
func drag(e *Editor, m Mods, pts ...image.Point) {
	e.PointerDown(pts[0], Left, m)
	for _, p := range pts[1:] {
		e.PointerMove(p, m)
	}
	e.PointerUp(pts[len(pts)-1], Left)
}

func rootRows(e *Editor) []string {
	px := e.Doc.RootDef().Pixels
	return px.EncodeRows(px.Bounds())
}

func TestLine4IsConnected(t *testing.T) {
	for _, c := range [][2]image.Point{{pt(0, 0), pt(5, 2)}, {pt(3, 3), pt(-4, 1)}, {pt(0, 0), pt(0, -4)}, {pt(2, 2), pt(2, 2)}, {pt(0, 0), pt(3, 3)}} {
		pts := line4(c[0], c[1])
		if pts[0] != c[0] || pts[len(pts)-1] != c[1] {
			t.Errorf("%v: endpoints %v..%v", c, pts[0], pts[len(pts)-1])
		}
		for i := 1; i < len(pts); i++ {
			d := pts[i].Sub(pts[i-1])
			if abs(d.X)+abs(d.Y) != 1 {
				t.Errorf("%v: step %v -> %v is not orthogonal", c, pts[i-1], pts[i])
			}
		}
		if want := abs(c[1].X-c[0].X) + abs(c[1].Y-c[0].Y) + 1; len(pts) != want {
			t.Errorf("%v: %d cells, want %d", c, len(pts), want)
		}
	}
}

func TestPencilClickToggles(t *testing.T) {
	e := New(doc.New())
	drag(e, Mods{}, pt(3, 4))
	if !e.Doc.RootDef().Pixels.Get(3, 4) {
		t.Fatal("click did not set the pixel")
	}
	drag(e, Mods{}, pt(3, 4))
	if e.Doc.RootDef().Pixels.Get(3, 4) {
		t.Fatal("second click did not clear it")
	}
}

func TestPencilDragPaintsFirstValue(t *testing.T) {
	e := New(doc.New())
	e.Doc.RootDef().Pixels.Set(2, 0, true)
	// Starting on an off pixel paints on, even over pixels already on.
	drag(e, Mods{}, pt(0, 0), pt(4, 0))
	if got := strings.Join(rootRows(e), "|"); got != "#####" {
		t.Errorf("rows %q", got)
	}
	// Starting on an on pixel erases.
	drag(e, Mods{}, pt(1, 0), pt(3, 0))
	if got := strings.Join(rootRows(e), "|"); got != "#...#" {
		t.Errorf("rows %q", got)
	}
}

func TestDiagonalDragStaysConnected(t *testing.T) {
	e := New(doc.New())
	drag(e, Mods{}, pt(0, 0), pt(3, 3))
	nl := e.Netlist()
	if len(nl.Nets) != 1 {
		t.Errorf("a diagonal drag made %d nets; strokes must be 4-connected\n%s", len(nl.Nets), strings.Join(rootRows(e), "\n"))
	}
}

func TestAltLocksAxis(t *testing.T) {
	e := New(doc.New())
	alt := Mods{Alt: true}
	// Wobbly mostly-horizontal movement draws a straight horizontal line.
	drag(e, alt, pt(0, 5), pt(1, 6), pt(3, 6), pt(6, 4), pt(8, 7))
	if got := strings.Join(rootRows(e), "|"); got != "#########" {
		t.Errorf("rows %q", got)
	}
	if b := e.Doc.RootDef().Pixels.Bounds(); b != image.Rect(0, 5, 9, 6) {
		t.Errorf("bounds %v", b)
	}

	e = New(doc.New())
	drag(e, alt, pt(0, 0), pt(1, 3), pt(-1, 6))
	if b := e.Doc.RootDef().Pixels.Bounds(); b != image.Rect(0, 0, 1, 7) {
		t.Errorf("vertical lock bounds %v", b)
	}
}

func TestStrokeIsOneUndoableCommand(t *testing.T) {
	e := New(doc.New())
	drag(e, Mods{}, pt(0, 0), pt(5, 0))
	drag(e, Mods{}, pt(0, 2), pt(0, 4))
	before := e.Doc.RootDef().Pixels.Clone()
	e.Undo()
	if e.Doc.RootDef().Pixels.Count() != 6 {
		t.Errorf("after undo: %v", e.Doc.RootDef().Pixels)
	}
	e.Redo()
	if !e.Doc.RootDef().Pixels.Equal(before) {
		t.Error("redo did not restore")
	}
	e.Undo()
	e.Undo()
	if !e.Doc.RootDef().Pixels.IsEmpty() {
		t.Error("undoing everything did not empty the document")
	}
	e.Undo()
	if e.Status != "nothing to undo" {
		t.Errorf("status %q", e.Status)
	}
}

func TestEditInsideInstanceEditsDefinition(t *testing.T) {
	d := doc.New()
	cell := doc.NewDefinition("cell", 3, 3)
	d.Defs["cell"] = cell
	d.RootDef().Instances = []doc.Instance{
		{ID: "i1", Def: "cell", X: 0, Y: 0, Name: "a"},
		{ID: "i2", Def: "cell", X: 10, Y: 0, Orient: doc.Orient{Rot: 1}, Name: "b"},
	}
	e := New(d)
	drag(e, Mods{}, pt(0, 0)) // top-left of instance a
	if !cell.Pixels.Get(0, 0) || !d.RootDef().Pixels.IsEmpty() {
		t.Fatal("click inside an instance did not edit its definition")
	}
	flat := d.Flatten().Pixels
	// Rotated clockwise, local (0,0) of a 3x3 lands at (2,0).
	want := bitmap.New()
	want.Set(0, 0, true)
	want.Set(12, 0, true)
	if !flat.Equal(want) {
		t.Errorf("flattened:\n%v", flat)
	}
}

func TestSimulateInverter(t *testing.T) {
	fx, err := fixture.ParseFile(filepath.Join("..", "testdata", "sim", "inverter.fix"))
	if err != nil {
		t.Fatal(err)
	}
	e := New(fx.Doc)
	e.Do(ActToggleSimulate)
	if e.Mode() != SimulateMode {
		t.Fatalf("not simulating: %s", e.Status)
	}
	nl := e.Netlist()
	in, _ := nl.Lookup("in")
	out, _ := nl.Lookup("out")
	if e.Value(out) != sim.High {
		t.Fatalf("out = %v at start", e.Value(out))
	}
	e.PointerDown(pt(0, 6), Left, Mods{}) // pin input high
	if e.Value(in) != sim.High || e.Value(out) != sim.Low {
		t.Errorf("after pinning in high: in=%v out=%v", e.Value(in), e.Value(out))
	}
	e.PointerMove(pt(0, 6), Mods{})
	if _, n, desc := e.Hover(); n != in || !strings.Contains(desc, "in = high (pinned high)") {
		t.Errorf("hover = %d %q", n, desc)
	}
	e.PointerDown(pt(0, 6), Right, Mods{})
	if e.Value(out) != sim.High {
		t.Errorf("after pinning in low: out=%v", e.Value(out))
	}
	e.PointerDown(pt(0, 6), Middle, Mods{})
	if e.Value(in) != sim.Floating {
		t.Errorf("after release: in=%v", e.Value(in))
	}

	// Choosing the pencil leaves simulate mode; editing then works.
	e.Do(ActPencil)
	if e.Mode() != EditMode || e.Runner() != nil {
		t.Error("pencil did not return to edit mode")
	}
}

func TestSimulateRefusesErrors(t *testing.T) {
	e := New(doc.New())
	drag(e, Mods{}, pt(0, 0), pt(3, 0))
	drag(e, Mods{}, pt(0, 1), pt(3, 1)) // 2-pixel-thick wire: E_THICK
	e.Do(ActToggleSimulate)
	if e.Mode() != EditMode || !strings.Contains(e.Status, "E_THICK") {
		t.Errorf("mode %v, status %q", e.Mode(), e.Status)
	}
}

func TestViewZoomKeepsPointFixed(t *testing.T) {
	v := NewView()
	v.Pan(100, 50)
	before := v.ToWorld(400, 300)
	for _, n := range []int{1, 1, -3, -4, 2} {
		v.Zoom(n, 400, 300)
		if got := v.ToWorld(400, 300); abs(got.X-before.X) > 1 || abs(got.Y-before.Y) > 1 {
			t.Fatalf("zoom %d moved the point under the cursor from %v to %v (scale %v)", n, before, got, v.Scale)
		}
	}
	v.Fit(image.Rect(0, 0, 1000, 500), image.Rect(100, 50, 900, 650))
	if got := v.ToWorld(500, 350); got.X < 495 || got.X > 505 || got.Y < 245 || got.Y > 255 {
		t.Errorf("fit does not centre: area centre shows %v", got)
	}
	if v.Scale != 0.5 {
		t.Errorf("fit scale %v, want 0.5", v.Scale)
	}
}

// TestReplaceDocumentChangesCompiles guards the canvas cache: it rebuilds
// textures when Compiles changes, so opening a document must never repeat the
// previous document's count (an untitled start then Open used to show only
// labels and boxes, with the old empty pixels).
func TestReplaceDocumentChangesCompiles(t *testing.T) {
	e := New(doc.New())
	e.Netlist()
	before := e.Compiles()
	fx, err := fixture.ParseFile(filepath.Join("..", "testdata", "sim", "crossing.fix"))
	if err != nil {
		t.Fatal(err)
	}
	e.ReplaceDocument(fx.Doc)
	if e.Compiles() == before {
		t.Errorf("Compiles still %d after ReplaceDocument", before)
	}
}
