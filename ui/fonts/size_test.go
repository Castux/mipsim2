package fonts

import "testing"

func TestPixelify(t *testing.T) {
	b, err := Pixelify()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range "AZaz09·×_?" {
		rows, ok := b.Glyphs[r]
		if !ok || len(rows) != b.Height {
			t.Errorf("glyph %q missing or wrong height", r)
		}
	}
	if b.Height < 10 || b.Height > 20 {
		t.Errorf("height %d", b.Height)
	}
}
