// Package fonts holds the ui's pixel font, Pixelify Sans, Copyright 2021 The
// Pixelify Sans Project Authors (https://github.com/eifetx/Pixelify-Sans),
// licensed under the SIL Open Font License 1.1 (see pixelifysans/OFL.txt).
// It is rasterised once to a one-bit bitmap font at its pixel size.
package fonts

import (
	_ "embed"
	"image"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Bitmap is a one-bit font: each glyph is rows of '#' and '.', all Height
// rows tall, drawn left to right with Spacing empty columns between glyphs.
type Bitmap struct {
	Name    string
	Height  int
	Spacing int
	Glyphs  map[rune][]string
}

//go:embed pixelifysans/PixelifySans.ttf
var pixelify []byte

// pixelifySize is the size at which Pixelify Sans renders one design pixel
// per screen pixel closely enough to threshold cleanly (picked by eye: the
// font has a few off-grid details, so no exact size exists).
const pixelifySize = 11

// Pixelify returns Pixelify Sans as a bitmap font.
func Pixelify() (*Bitmap, error) {
	return rasterise("pixelify", pixelify, pixelifySize)
}

// rasterise renders a TTF font at the given size and thresholds every glyph
// from space to ÿ to one bit.
func rasterise(name string, ttf []byte, size float64) (*Bitmap, error) {
	f, err := opentype.Parse(ttf)
	if err != nil {
		return nil, err
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil, err
	}
	m := face.Metrics()
	ascent, descent := m.Ascent.Ceil(), m.Descent.Ceil()
	b := &Bitmap{Name: name, Height: ascent + descent, Glyphs: map[rune][]string{}}
	for r := rune(32); r < 256; r++ {
		adv, ok := face.GlyphAdvance(r)
		if !ok {
			continue
		}
		w := adv.Round()
		img := image.NewAlpha(image.Rect(0, 0, w, b.Height))
		d := &font.Drawer{Dst: img, Src: image.Opaque, Face: face, Dot: fixed.P(0, ascent)}
		d.DrawString(string(r))
		rows := make([]string, b.Height)
		for y := range b.Height {
			row := make([]byte, w)
			for x := range w {
				row[x] = '.'
				if img.AlphaAt(x, y).A >= 128 {
					row[x] = '#'
				}
			}
			rows[y] = string(row)
		}
		b.Glyphs[r] = rows
	}
	return b, nil
}
