package editor

import (
	"image"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Castux/mipsim2/bitmap"
	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/internal/fixture"
)

func typeName(e *Editor, s string) {
	for range 40 {
		e.Do(ActBackspace)
	}
	for _, r := range s {
		e.TypeRune(r)
	}
	e.Do(ActEnter)
}

func TestMakeComponentAndExplode(t *testing.T) {
	e := withRows("##.", "#..", "..#")
	e.Doc.RootDef().Labels = []doc.Label{{X: 0, Y: 0, Name: "a"}}
	flatBefore := e.Doc.Flatten().Pixels.Clone()
	selectRect(e, pt(0, 0), pt(1, 1))
	before := e.Doc.Clone()
	e.Do(ActMakeComponent)
	if _, typing := e.Typing(); !typing {
		t.Fatal("make component did not ask for a name")
	}
	typeName(e, "corner")

	root := e.Doc.RootDef()
	if len(root.Instances) != 1 || root.Pixels.Count() != 1 || len(root.Labels) != 0 {
		t.Fatalf("root after make: %d instances, pixels\n%v", len(root.Instances), root.Pixels)
	}
	inst := root.Instances[0]
	def := e.Doc.Defs[inst.Def]
	if def.Name != "corner" || def.W != 2 || def.H != 2 || def.Pixels.Count() != 3 || len(def.Labels) != 1 {
		t.Fatalf("new definition %+v\n%v", def, def.Pixels)
	}
	if !e.Doc.Flatten().Pixels.Equal(flatBefore) {
		t.Error("making a component changed the circuit")
	}
	if _, ok := e.Netlist().Lookup(inst.Name + ".a"); !ok {
		t.Errorf("label not renamed hierarchically; names %v", e.Netlist().Nets[0].Names)
	}

	// Rotate the instance, then explode it: the pixels come back rotated.
	e.Do(ActRotate)
	rotated := e.Doc.Flatten().Pixels.Clone()
	e.Do(ActExplode)
	if len(e.Doc.RootDef().Instances) != 0 || !e.Doc.RootDef().Pixels.Equal(rotated) {
		t.Errorf("explode did not keep the rotated pixels:\n%v\nwant\n%v", e.Doc.RootDef().Pixels, rotated)
	}
	if len(e.Doc.Defs) != 2 {
		t.Error("explode removed the definition")
	}

	// Undo back to before making the component.
	for range 4 {
		e.Do(ActUndo)
	}
	if !e.Doc.Equal(before) {
		t.Error("undoing make, rename, rotate and explode did not restore the document")
	}
}

func TestPlaceFromPaletteAndSharedEdits(t *testing.T) {
	e := withRows("#")
	selectRect(e, pt(0, 0), pt(2, 1))
	e.Do(ActMakeComponent)
	e.Do(ActEnter) // keep the default name
	defs := e.Definitions()
	if len(defs) != 1 || defs[0].Instances != 1 {
		t.Fatalf("definitions %+v", defs)
	}
	e.PlaceDefinition(defs[0].ID)
	e.PointerMove(pt(10, 0), Mods{})
	if o := e.Overlay(); len(o.GhostRects) != 2 || o.GhostRects[1] != image.Rect(10, 0, 13, 2) {
		t.Errorf("placement preview %+v", o.GhostRects)
	}
	e.PointerDown(pt(10, 0), Left, Mods{})
	if got := e.Definitions()[0].Instances; got != 2 {
		t.Fatalf("%d instances after placing", got)
	}
	// Drawing inside one instance changes the definition, so both.
	e.Do(ActPencil)
	drag(e, Mods{}, pt(12, 1))
	flat := e.Doc.Flatten().Pixels
	if !flat.Get(2, 1) || !flat.Get(12, 1) {
		t.Errorf("shared edit not in both instances:\n%v", flat)
	}
	// The cue marks the hovered instance and its sibling.
	e.PointerMove(pt(11, 1), Mods{})
	var hovered, siblings int
	for _, c := range e.Instances() {
		if c.Hovered {
			hovered++
		}
		if c.Sibling {
			siblings++
		}
	}
	if hovered != 1 || siblings != 1 {
		t.Errorf("cues: %d hovered, %d siblings", hovered, siblings)
	}
}

func TestRenameAndDeleteDefinitions(t *testing.T) {
	e := withRows("#")
	selectRect(e, pt(0, 0), pt(1, 1))
	e.Do(ActMakeComponent)
	typeName(e, "dot")
	id := e.Definitions()[0].ID

	e.Do(ActSelect)
	drag(e, Mods{}, pt(0, 0)) // click selects the instance
	e.Do(ActRename)
	typeName(e, "bad.name")
	if !strings.Contains(e.Status, "invalid instance name") {
		t.Errorf("status %q", e.Status)
	}
	drag(e, Mods{}, pt(0, 0))
	e.Do(ActRename)
	typeName(e, "d1")
	if e.Doc.RootDef().Instances[0].Name != "d1" {
		t.Errorf("instance %+v", e.Doc.RootDef().Instances[0])
	}

	e.StartRenameDefinition(id)
	typeName(e, "Dot cell")
	if e.Doc.Defs[id].Name != "Dot cell" {
		t.Errorf("definition name %q", e.Doc.Defs[id].Name)
	}

	e.DeleteDefinition(id)
	if e.Doc.Defs[id] == nil || !strings.Contains(e.Status, "used 1 time") {
		t.Errorf("deleted a used definition; status %q", e.Status)
	}
	drag(e, Mods{}, pt(0, 0))
	e.Do(ActDelete)
	e.DeleteDefinition(id)
	if e.Doc.Defs[id] != nil {
		t.Error("unused definition not deleted")
	}
}

// resizeDoc has a 3x2 definition placed twice: plain at (0,0) and rotated
// clockwise at (10,0).
func resizeDoc() *Editor {
	d := doc.New()
	cell := doc.NewDefinition("cell", 3, 2)
	cell.Pixels = bitmap.MustFromRows("#..", ".#.")
	d.Defs["cell"] = cell
	d.RootDef().Instances = []doc.Instance{
		{ID: "i1", Def: "cell", X: 0, Y: 0, Name: "a"},
		{ID: "i2", Def: "cell", X: 10, Y: 0, Orient: doc.Orient{Rot: 1}, Name: "b"},
	}
	return New(d)
}

func TestResizeKeepsContentFixed(t *testing.T) {
	for _, c := range []struct {
		name     string
		handle   image.Point // for instance a at (0,0)-(3,2)
		to       image.Point
		wantRect image.Rectangle
	}{
		{"grow right", pt(3, 1), pt(5, 1), image.Rect(0, 0, 5, 2)},
		{"grow left", pt(-1, 1), pt(-3, 1), image.Rect(-2, 0, 3, 2)},
		{"grow up", pt(1, -1), pt(1, -4), image.Rect(0, -3, 3, 2)},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := resizeDoc()
			before := e.Doc.Flatten().Pixels.Clone()
			e.Do(ActSelect)
			drag(e, Mods{}, pt(1, 1)) // select instance a
			if o := e.Overlay(); len(o.Handles) != 4 {
				t.Fatalf("handles %v", o.Handles)
			}
			drag(e, Mods{}, c.handle, c.to)
			if got := e.Doc.PlacedRect(e.Doc.RootDef().Instances[0]); got != c.wantRect {
				t.Errorf("instance a rect %v, want %v (status %q)", got, c.wantRect, e.Status)
			}
			if after := e.Doc.Flatten().Pixels; !after.Equal(before) {
				t.Errorf("resize moved content:\n%v\nwant\n%v", after, before)
			}
			if err := e.Doc.Validate(); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestResizeCuttingContentIsRejected(t *testing.T) {
	e := resizeDoc()
	before := e.Doc.Clone()
	e.Do(ActSelect)
	drag(e, Mods{}, pt(1, 1))
	drag(e, Mods{}, pt(3, 1), pt(2, 1)) // handle just outside the edge: edge to x=2, 2 wide, cuts nothing
	if e.Doc.Defs["cell"].W != 2 {
		t.Fatalf("shrink to the content failed: %q", e.Status)
	}
	drag(e, Mods{}, pt(2, 1), pt(1, 1)) // to 1 wide: cuts pixel (1,1)
	if e.Doc.Defs["cell"].W != 2 || !strings.Contains(e.Status, "cut off") {
		t.Errorf("cutting resize not rejected: width %d, status %q", e.Doc.Defs["cell"].W, e.Status)
	}
	e.Do(ActUndo)
	if !e.Doc.Equal(before) {
		t.Error("undo did not restore the size")
	}
}

// TestAdder8SharedEdit is the M7 completion check: an 8-bit adder built
// from 8 instances of one full adder adds correctly, and editing one instance
// changes all 8, in the circuit and in simulation.
func TestAdder8SharedEdit(t *testing.T) {
	fx, err := fixture.ParseFile(filepath.Join("..", "testdata", "runner", "adder8.fix"))
	if err != nil {
		t.Fatal(err)
	}
	e := New(fx.Doc)
	if n := e.Definitions(); len(n) != 3 {
		t.Fatalf("definitions %+v", n)
	}
	for _, info := range e.Definitions() {
		if info.ID == "fa" && info.Instances != 8 {
			t.Fatalf("fa placed %d times", info.Instances)
		}
	}
	sum := func() string {
		e.Do(ActToggleSimulate)
		defer e.Do(ActToggleSimulate)
		r := e.Runner()
		if r == nil {
			t.Fatalf("cannot simulate: %s", e.Status)
		}
		r.Set("a", "100")
		r.Set("b", "55")
		r.Set("cin", "0")
		r.Settle()
		return r.Format("sum")
	}
	if got := sum(); got != "155" {
		t.Fatalf("100+55 = %s", got)
	}

	// Erase the sum output pixel inside the first full adder: all 8 lose it.
	e.Do(ActPencil)
	drag(e, Mods{}, pt(126, 18))
	flat := e.Doc.Flatten().Pixels
	for i := range 8 {
		if flat.Get(126, 18+64*i) {
			t.Errorf("full adder %d still has its sum output pixel", i)
		}
	}
	if got := sum(); got != "?" {
		t.Errorf("after the shared edit, sum = %s; want every bit disconnected", got)
	}
	e.Do(ActUndo)
	if got := sum(); got != "155" {
		t.Errorf("after undo, sum = %s", got)
	}
}

// TestClickSelectsDeepestInstance: a click selects the most nested
// instance, so it can be moved or deleted inside its parent component (which
// changes that component everywhere); clicking again goes one level up.
func TestClickSelectsDeepestInstance(t *testing.T) {
	fx, err := fixture.ParseFile(filepath.Join("..", "testdata", "netlist", "hier.fix"))
	if err != nil {
		t.Fatal(err)
	}
	e := New(fx.Doc)
	e.Do(ActSelect)
	inner := image.Rect(0, 0, 9, 11) // p1.g1: inverter g1 inside pair p1 at (0,0)
	outer := image.Rect(0, 0, 21, 11)

	drag(e, Mods{}, pt(2, 8))
	if e.Selected() != inner || e.sel.def != "pair" {
		t.Fatalf("first click selected %v in %s, want %v in pair (%q)", e.Selected(), e.sel.def, inner, e.Status)
	}
	drag(e, Mods{}, pt(2, 8))
	if e.Selected() != outer || e.sel.def != e.Doc.Root {
		t.Fatalf("second click selected %v in %s, want %v at the top level", e.Selected(), e.sel.def, outer)
	}
	drag(e, Mods{}, pt(2, 8))
	if e.Selected() != inner {
		t.Fatalf("third click selected %v, want to wrap back to %v", e.Selected(), inner)
	}

	// Deleting the inner instance edits pair, so both p1 and p2 lose it.
	pixels := e.Doc.Flatten().Pixels.Count()
	invPixels := e.Doc.Defs["inv"].Pixels.Count()
	e.Do(ActDelete)
	if got := len(e.Doc.Defs["pair"].Instances); got != 1 {
		t.Fatalf("pair has %d instances after deleting g1", got)
	}
	if got := e.Doc.Flatten().Pixels.Count(); got != pixels-2*invPixels {
		t.Errorf("flattened pixels %d, want %d (g1 gone from both pairs)", got, pixels-2*invPixels)
	}
	e.Do(ActUndo)
	if got := e.Doc.Flatten().Pixels.Count(); got != pixels {
		t.Errorf("undo left %d pixels, want %d", got, pixels)
	}
}
