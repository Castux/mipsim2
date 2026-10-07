package fonts

import "testing"

func TestRaylib(t *testing.T) {
	if len(Raylib.Glyphs) != 224 {
		t.Errorf("%d glyphs, want 224", len(Raylib.Glyphs))
	}
	for r, rows := range Raylib.Glyphs {
		if len(rows) != Raylib.Height {
			t.Errorf("glyph %q has %d rows", r, len(rows))
		}
		for _, row := range rows {
			if len(row) != len(rows[0]) {
				t.Errorf("glyph %q is ragged", r)
			}
		}
	}
	want := []string{"......", "######", "#....#", "#....#", "######", "#....#", "#....#", "#....#", "......", "......"}
	for i, row := range Raylib.Glyphs['A'] {
		if row != want[i] {
			t.Fatalf("A row %d = %q, want %q", i, row, want[i])
		}
	}
}
