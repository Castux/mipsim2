// Package fonts holds the ui's pixel font: raylib's default font, a one-bit
// bitmap font by design (zlib licence, see raylib.go), stored as glyph rows.
// Being bitmap data, it is pixel-exact at any integer scale.
package fonts

// Bitmap is a one-bit font: each glyph is rows of '#' and '.', all Height
// rows tall, drawn left to right with Spacing empty columns between glyphs.
type Bitmap struct {
	Name    string
	Height  int
	Spacing int
	Glyphs  map[rune][]string
}
