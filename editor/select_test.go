package editor

import (
	"image"
	"strings"
	"testing"

	"github.com/Castux/mipsim2/bitmap"
	"github.com/Castux/mipsim2/doc"
)

// withRows returns an editor whose root holds the given rows at (0,0).
func withRows(rows ...string) *Editor {
	d := doc.New()
	d.RootDef().Pixels = bitmap.MustFromRows(rows...)
	return New(d)
}

func rows(e *Editor) string {
	px := e.Doc.RootDef().Pixels
	r := px.Bounds()
	if r.Empty() {
		return ""
	}
	return strings.Join(px.EncodeRows(r), "|")
}

// selectRect drags out a selection with the select tool.
func selectRect(e *Editor, a, b image.Point) {
	e.Do(ActSelect)
	drag(e, Mods{}, a, b)
}

// checkUndoRedo undoes the last command, checks the document matches before,
// redoes it and checks it matches after.
func checkUndoRedo(t *testing.T, e *Editor, before *doc.Document) {
	t.Helper()
	after := e.Doc.Clone()
	e.Do(ActUndo)
	if !e.Doc.Equal(before) {
		t.Fatalf("undo did not restore the document; root now %q", rows(e))
	}
	e.Do(ActRedo)
	if !e.Doc.Equal(after) {
		t.Fatalf("redo did not reapply the command; root now %q", rows(e))
	}
}

func TestCopyPaste(t *testing.T) {
	e := withRows("##.", "#..")
	selectRect(e, pt(0, 0), pt(1, 1))
	if e.Selected() != image.Rect(0, 0, 2, 2) {
		t.Fatalf("selected %v", e.Selected())
	}
	e.Do(ActCopy)
	before := e.Doc.Clone()
	e.Do(ActPaste)
	e.PointerMove(pt(5, 0), Mods{})
	if o := e.Overlay(); len(o.Ghost) != 3 || o.GhostRects[0] != image.Rect(5, 0, 7, 2) {
		t.Errorf("paste preview %+v", o)
	}
	e.PointerDown(pt(5, 0), Left, Mods{})
	if got := rows(e); got != "##...##|#....#" {
		t.Fatalf("after paste %q", got)
	}
	if e.Selected() != image.Rect(5, 0, 7, 2) {
		t.Errorf("pasted area not selected: %v", e.Selected())
	}
	checkUndoRedo(t, e, before)
}

func TestPasteIsOpaque(t *testing.T) {
	e := withRows("#..", "...", "###")
	selectRect(e, pt(0, 0), pt(2, 1)) // "#..", "..."
	e.Do(ActCopy)
	e.Do(ActPaste)
	e.PointerDown(pt(0, 1), Left, Mods{}) // over the bottom row
	if got := rows(e); got != "#|#" {
		t.Errorf("after opaque paste %q", strings.ReplaceAll(rows(e), "|", "\n"))
	}
}

func TestCutAndDelete(t *testing.T) {
	e := withRows("###", "#.#")
	selectRect(e, pt(1, 0), pt(2, 1))
	before := e.Doc.Clone()
	e.Do(ActCut)
	if got := rows(e); got != "#|#" {
		t.Fatalf("after cut %q", got)
	}
	checkUndoRedo(t, e, before)
	e.Do(ActPaste)
	e.PointerDown(pt(4, 0), Left, Mods{})
	if got := rows(e); got != "#...##|#....#" {
		t.Errorf("after pasting the cut %q", got)
	}

	selectRect(e, pt(0, 0), pt(0, 1))
	before = e.Doc.Clone()
	e.Do(ActDelete)
	if got := rows(e); got != "##|.#" {
		t.Errorf("after delete %q", got)
	}
	checkUndoRedo(t, e, before)
}

func TestMoveByDragging(t *testing.T) {
	e := withRows("##", "#.")
	selectRect(e, pt(0, 0), pt(1, 1))
	before := e.Doc.Clone()
	e.PointerDown(pt(1, 1), Left, Mods{}) // inside the selection: move
	e.PointerMove(pt(4, 3), Mods{})
	if o := e.Overlay(); o.Selection != image.Rect(3, 2, 5, 4) || len(o.Ghost) != 3 {
		t.Errorf("move preview %+v", o)
	}
	e.PointerUp(pt(4, 3), Left)
	px := e.Doc.RootDef().Pixels
	if px.Count() != 3 || !px.Get(3, 2) || !px.Get(4, 2) || !px.Get(3, 3) {
		t.Fatalf("after move:\n%v", px)
	}
	if e.Selected() != image.Rect(3, 2, 5, 4) {
		t.Errorf("selection did not follow: %v", e.Selected())
	}
	checkUndoRedo(t, e, before)
}

func TestRotateAndMirrorSelection(t *testing.T) {
	e := withRows("###", "#..")
	selectRect(e, pt(0, 0), pt(2, 1))
	before := e.Doc.Clone()
	e.Do(ActRotate)
	// Clockwise: the 3x2 block becomes 2x3 with its top-left kept.
	if got := rows(e); got != "##|.#|.#" {
		t.Fatalf("after rotate %q", got)
	}
	if e.Selected() != image.Rect(0, 0, 2, 3) {
		t.Errorf("selection after rotate %v", e.Selected())
	}
	checkUndoRedo(t, e, before)
	selectRect(e, pt(0, 0), pt(1, 2)) // undo and redo clear the selection
	e.Do(ActMirror)
	if got := rows(e); got != "##|#|#" {
		t.Errorf("after mirror %q", got)
	}
	for range 3 {
		e.Do(ActMirror)
	}
	e.Do(ActRotate)
	e.Do(ActRotate)
	e.Do(ActRotate)
	if got := rows(e); got != "###|#" {
		t.Errorf("four rotations and two mirrors did not return: %q", got)
	}
}

// componentDoc returns a root with one 3x2 instance of "cell" at (2,0) and a
// root pixel at (0,0).
func componentDoc() *doc.Document {
	d := doc.New()
	cell := doc.NewDefinition("cell", 3, 2)
	cell.Pixels = bitmap.MustFromRows("##.", "...")
	d.Defs["cell"] = cell
	d.RootDef().Pixels.Set(0, 0, true)
	d.RootDef().Instances = []doc.Instance{{ID: "i1", Def: "cell", X: 2, Y: 0, Name: "c"}}
	return d
}

func TestClickSelectsInstanceAndRotateComposes(t *testing.T) {
	e := New(componentDoc())
	e.Do(ActSelect)
	drag(e, Mods{}, pt(3, 1)) // a click
	if e.Selected() != image.Rect(2, 0, 5, 2) {
		t.Fatalf("click selected %v", e.Selected())
	}
	before := e.Doc.Clone()
	e.Do(ActRotate)
	inst := e.Doc.RootDef().Instances[0]
	if inst.Orient != (doc.Orient{Rot: 1}) || inst.X != 2 || inst.Y != 0 {
		t.Errorf("instance after rotate %+v", inst)
	}
	if !e.Doc.Defs["cell"].Pixels.Equal(bitmap.MustFromRows("##.", "...")) {
		t.Error("rotating an instance changed its definition")
	}
	checkUndoRedo(t, e, before)
}

func TestCopyPasteInstanceKeepsLink(t *testing.T) {
	e := New(componentDoc())
	e.Do(ActSelect)
	drag(e, Mods{}, pt(3, 1))
	e.Do(ActCopy)
	e.Do(ActPaste)
	e.Do(ActMirror) // transform the preview
	e.PointerDown(pt(10, 5), Left, Mods{})
	insts := e.Doc.RootDef().Instances
	if len(insts) != 2 {
		t.Fatalf("instances %+v", insts)
	}
	n := insts[1]
	if n.Def != "cell" || n.ID == insts[0].ID || n.Name == insts[0].Name || n.Orient != (doc.Orient{Flip: true}) {
		t.Errorf("pasted instance %+v", n)
	}
	// Editing the definition through either instance shows in both.
	e.Do(ActPencil)
	drag(e, Mods{}, pt(2, 1))
	flat := e.Doc.Flatten().Pixels
	if !flat.Get(2, 1) || !flat.Get(12, 6) { // mirrored copy: local (0,1) -> (2,1) + (10,5)
		t.Errorf("shared edit not visible in both instances:\n%v", flat)
	}
}

func TestEditsBreakingInvariantsAreRejected(t *testing.T) {
	e := New(componentDoc()) // root pixel at (0,0), instance c at (2,0)-(5,2)
	rejected := func(what string) {
		t.Helper()
		if !strings.Contains(e.Status, "rejected") {
			t.Errorf("%s not rejected; status %q", what, e.Status)
		}
	}

	// Moving a root pixel into the instance's rectangle (a zero-size drag is a
	// click, so select a 1x2 area).
	selectRect(e, pt(0, 0), pt(0, 1))
	before := e.Doc.Clone()
	e.PointerDown(pt(0, 0), Left, Mods{})
	e.PointerMove(pt(3, 0), Mods{})
	e.PointerUp(pt(3, 0), Left)
	rejected("moving a pixel into an instance")
	if !e.Doc.Equal(before) {
		t.Fatal("rejected move changed the document")
	}

	// Pasting a copy of the instance overlapping it.
	e.Do(ActEscape)
	drag(e, Mods{}, pt(3, 1)) // click selects the instance
	e.Do(ActCopy)
	e.Do(ActPaste)
	e.PointerDown(pt(0, 1), Left, Mods{})
	rejected("overlapping paste")

	// Pasting the instance inside itself makes a definition contain itself.
	e.Do(ActPaste)
	e.PointerDown(pt(3, 0), Left, Mods{})
	rejected("pasting a definition into itself")
	if !e.Doc.Equal(before) {
		t.Error("rejected edits changed the document")
	}
}

func TestLabels(t *testing.T) {
	e := withRows("###")
	e.Do(ActLabel)
	e.PointerDown(pt(1, 0), Left, Mods{})
	for _, r := range "out_1" {
		e.TypeRune(r)
	}
	if s, ok := e.Typing(); !ok || s != "out_1" {
		t.Fatalf("typing %q %v", s, ok)
	}
	before := e.Doc.Clone()
	e.Do(ActEnter)
	if l := e.Doc.RootDef().Labels; len(l) != 1 || l[0] != (doc.Label{X: 1, Y: 0, Name: "out_1"}) {
		t.Fatalf("labels %+v", l)
	}
	if _, ok := e.Netlist().Lookup("out_1"); !ok {
		t.Error("label not compiled")
	}
	checkUndoRedo(t, e, before)

	// Clicking the label edits it; an invalid name is refused.
	e.PointerDown(pt(1, 0), Left, Mods{})
	e.Do(ActBackspace)
	e.TypeRune('.')
	e.Do(ActEnter)
	if !strings.Contains(e.Status, "invalid label") || e.Doc.RootDef().Labels[0].Name != "out_1" {
		t.Errorf("invalid name: status %q labels %+v", e.Status, e.Doc.RootDef().Labels)
	}
	// Clearing the name removes the label.
	e.PointerDown(pt(1, 0), Left, Mods{})
	for range 5 {
		e.Do(ActBackspace)
	}
	e.Do(ActEnter)
	if len(e.Doc.RootDef().Labels) != 0 {
		t.Errorf("label not removed: %+v", e.Doc.RootDef().Labels)
	}
	// Escape cancels.
	e.PointerDown(pt(0, 0), Left, Mods{})
	e.TypeRune('a')
	e.Do(ActEscape)
	if len(e.Doc.RootDef().Labels) != 0 {
		t.Error("escape did not cancel")
	}
}

func TestASCIIClipboard(t *testing.T) {
	e := New(doc.New())
	if err := e.SetClipboardRows(".#.\n###\n"); err != nil {
		t.Fatal(err)
	}
	e.Do(ActPaste)
	e.PointerDown(pt(0, 0), Left, Mods{})
	if got := rows(e); got != ".#|###" {
		t.Errorf("pasted ASCII %q", got)
	}
	selectRect(e, pt(0, 0), pt(2, 1))
	e.Do(ActCopy)
	if got := e.ClipboardRows(); got != ".#\n###" {
		t.Errorf("copied ASCII %q", got)
	}
	if err := e.SetClipboardRows("#x#"); err == nil {
		t.Error("bad ASCII accepted")
	}
}

// rightClick clicks the right button at p.
func rightClick(e *Editor, p image.Point) {
	e.PointerDown(p, Right, Mods{})
	e.PointerUp(p, Right)
}

// TestCellKinds checks the pencil's gestures: left paints wire or erases,
// right cycles a cell through the kinds its neighbours allow, keys 1 to 5
// set the hovered cell, and undo, copy, paste and rotate keep kinds.
func TestCellKinds(t *testing.T) {
	e := withRows("###", ".#.", "...", "...", ".#.", "###", ".#.")
	e.Do(ActPencil)
	before := e.Doc.Clone()
	rightClick(e, pt(1, 0)) // three neighbours
	if got := rows(e); got != "#T#|.#|||.#|###|.#" {
		t.Fatalf("after one right click %q", got)
	}
	checkUndoRedo(t, e, before)
	var seen []string
	for range 4 {
		rightClick(e, pt(1, 0))
		seen = append(seen, e.Doc.RootDef().Pixels.At(1, 0).String())
	}
	if got := strings.Join(seen, " "); got != "power ground wire transistor" {
		t.Errorf("three-neighbour cycle %q", got)
	}
	rightClick(e, pt(1, 5)) // four neighbours
	if k := e.Doc.RootDef().Pixels.At(1, 5); k.String() != "bridge" {
		t.Errorf("four-neighbour cell became %v", k)
	}
	rightClick(e, pt(4, 3)) // empty
	rightClick(e, pt(0, 0)) // one neighbour: wire -> power
	if got := rows(e); got != "HT#|.#||....H|.#|#B#|.#" {
		t.Fatalf("after cycling %q", got)
	}
	// Left: a stroke from an empty cell paints wire, from any cell erases.
	drag(e, Mods{}, pt(3, 2), pt(4, 2))
	drag(e, Mods{}, pt(1, 0))
	if got := rows(e); got != "H.#|.#|...##|....H|.#|#B#|.#" {
		t.Fatalf("after left strokes %q", got)
	}
	// Keys set the hovered cell.
	e.PointerMove(pt(3, 2), Mods{})
	e.Do(ActGround)
	if k := e.Doc.RootDef().Pixels.At(3, 2); k.String() != "ground" {
		t.Errorf("key set %v", k)
	}
	// Copy, paste and rotate keep kinds.
	selectRect(e, pt(0, 0), pt(1, 0))
	e.Do(ActCopy)
	e.Do(ActPaste)
	e.Do(ActRotate)
	e.PointerDown(pt(6, 0), Left, Mods{})
	if got := rows(e); got != "H.#...H|.#|...L#|....H|.#|#B#|.#" {
		t.Errorf("after rotated paste %q", got)
	}
}
