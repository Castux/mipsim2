package doc

import (
	"slices"
	"testing"

	"github.com/Castux/mipsim2/bitmap"
)

func TestMigratePatterns(t *testing.T) {
	cases := []struct {
		name     string
		old, new []string
	}{
		{"inverter",
			[]string{"###", "###", "###", ".#.", "#########", ".#.", "##.", ".#.", "###", "#.#", "###"},
			[]string{"HHH", "HHH", "HHH", ".#", "#########", ".#", "#T", ".#", "LLL", "L.L", "LLL"}},
		{"bridge", []string{".#.", "#.#", ".#."}, []string{".#", "#B#", ".#"}},
		{"bridge with a diagonal is not one", []string{".#.", "#.#", ".##"}, []string{".#", "#.#", ".##"}},
		{"thick block was an error, now wire", []string{"###", "###", "###", "###"}, []string{"###", "###", "###", "###"}},
		{"square with a wire attached", []string{"###.", "####", "###."}, []string{"HHH", "HHH#", "HHH"}},
		{"square with a block sticking out", []string{"###.", "####", "####"}, []string{"###", "####", "####"}},
	}
	for _, c := range cases {
		got := MigratePatterns(bitmap.MustFromRows(c.old...))
		rows := got.EncodeRows(got.Bounds())
		if !slices.Equal(rows, c.new) {
			t.Errorf("%s: got %q, want %q", c.name, rows, c.new)
		}
	}
}

func TestLoadMigratesVersion2(t *testing.T) {
	d, err := LoadString(`{"format": "mipsim", "version": 2, "root": "top",
		"defs": {"top": {"rows": ["###", "###", "###", ".#.", ".#.", "##.", ".#."]}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if k := d.RootDef().Pixels.At(1, 5); k != bitmap.Transistor {
		t.Errorf("T centre migrated to %v", k)
	}
	if _, err := LoadString(`{"format": "mipsim", "version": 2, "root": "top", "defs": {"top": {"rows": ["#T#"]}}}`); err == nil {
		t.Error("a version 2 file with typed rows was accepted")
	}
}
