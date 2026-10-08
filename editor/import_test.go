package editor

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/internal/fixture"
)

func loadFix(t *testing.T, path ...string) *doc.Document {
	t.Helper()
	fx, err := fixture.ParseFile(filepath.Join(append([]string{".."}, path...)...))
	if err != nil {
		t.Fatal(err)
	}
	return fx.Doc
}

// TestImportSelection checks that picking a component forces everything it
// uses, at any depth, and that forced ones cannot be unpicked alone.
func TestImportSelection(t *testing.T) {
	src := loadFix(t, "testdata", "runner", "adder8.fix")
	s := NewImportSelection(src)
	var names []string
	for _, it := range s.Items {
		names = append(names, it.Name)
	}
	if strings.Join(names, " ") != "fa nand xor" {
		t.Fatalf("items %v", names)
	}
	id := func(name string) doc.DefID {
		for _, it := range s.Items {
			if it.Name == name {
				return it.ID
			}
		}
		t.Fatalf("no item %s", name)
		return ""
	}
	s.Toggle(id("fa"))
	if !s.Forced(id("xor")) || !s.Forced(id("nand")) || s.Forced(id("fa")) || len(s.Chosen()) != 3 {
		t.Errorf("after picking fa: chosen %v", s.Chosen())
	}
	s.Toggle(id("nand")) // forced: no effect
	if !s.Checked(id("nand")) {
		t.Error("a forced component was unpicked")
	}
	s.Toggle(id("fa"))
	if len(s.Chosen()) != 0 {
		t.Errorf("after unpicking fa: chosen %v", s.Chosen())
	}
	s.Toggle(id("xor"))
	if got := s.Chosen(); len(got) != 2 || !slices.Contains(got, id("nand")) {
		t.Errorf("xor alone: chosen %v", got)
	}
}

// TestImportRenamesAndRelinks imports adder8's full adder into a copy of
// adder8 itself, so every name conflicts: the copies get suffixes, their
// instances point at the imported copies (not the originals), and undo
// removes them all.
func TestImportRenamesAndRelinks(t *testing.T) {
	src := loadFix(t, "testdata", "runner", "adder8.fix")
	e := New(src.Clone())
	before := len(e.Definitions())
	s := NewImportSelection(src)
	for _, it := range s.Items {
		if it.Name == "fa" {
			s.Toggle(it.ID)
		}
	}
	if !e.Import(src, s.Chosen()) {
		t.Fatalf("import: %s", e.Status)
	}
	if !strings.Contains(e.Status, "fa as fa_2") {
		t.Errorf("status %q", e.Status)
	}
	byName := map[string]DefInfo{}
	for _, d := range e.Definitions() {
		byName[d.Name] = d
	}
	if len(byName) != before+3 {
		t.Fatalf("definitions %v", byName)
	}
	for _, inst := range e.Doc.Defs[byName["fa_2"].ID].Instances {
		if name := e.Doc.Defs[inst.Def].Name; name != "xor_2" && name != "nand_2" {
			t.Errorf("imported fa uses %q, not an imported copy", name)
		}
	}

	// The imported adder works when placed: same netlist shape as the original.
	e.PlaceDefinition(byName["fa_2"].ID)
	e.PointerDown(pt(2000, 2000), Left, Mods{})
	if e.Netlist().HasErrors() {
		t.Errorf("placed import has errors: %v", e.Netlist().Diagnostics)
	}

	e.Undo() // the placement
	e.Undo() // the import
	if len(e.Definitions()) != before {
		t.Errorf("%d definitions after undo, want %d", len(e.Definitions()), before)
	}
}

func TestImportNothing(t *testing.T) {
	e := New(doc.New())
	if e.Import(doc.New(), nil) || e.Status != "nothing to import" {
		t.Errorf("status %q", e.Status)
	}
}
