package ui

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/Castux/mipsim2/ui/fonts"
)

// pixelText draws one bitmap font at one integer scale, so every glyph pixel
// lands on whole screen pixels. All ui text uses it, at a single size.
type pixelText struct {
	font   *fonts.Bitmap
	glyphs map[rune]*ebiten.Image // white on transparent; lookups only
}

func newPixelText() (*pixelText, error) {
	f, err := fonts.Pixelify()
	if err != nil {
		return nil, err
	}
	return &pixelText{font: f, glyphs: map[rune]*ebiten.Image{}}, nil
}

func (t *pixelText) glyph(r rune) (*ebiten.Image, int) {
	rows, ok := t.font.Glyphs[r]
	if !ok {
		r = '?'
		rows = t.font.Glyphs[r]
	}
	w := 0
	if len(rows) > 0 {
		w = len(rows[0])
	}
	if img, ok := t.glyphs[r]; ok {
		return img, w
	}
	if w == 0 {
		return nil, 0
	}
	img := ebiten.NewImage(w, t.font.Height)
	for y, row := range rows {
		for x := 0; x < len(row); x++ {
			if row[x] == '#' {
				img.Set(x, y, color.White)
			}
		}
	}
	t.glyphs[r] = img
	return img, w
}

// textTarget is the design height of a line of text, in logical pixels.
const textTarget = 18

// textScale is the integer scale for the current device scale factor.
func (a *app) textScale() int {
	return max(1, int(math.Round(textTarget*a.scale/float64(a.text.font.Height))))
}

func (a *app) lineHeight() float64 {
	return float64(a.text.font.Height * a.textScale())
}

// drawText draws s with its top-left at (x, y) and returns its width. The
// size argument is ignored: all text is one size.
func (a *app) drawText(dst *ebiten.Image, s string, x, y, _ float64, clr color.Color) float64 {
	k := float64(a.textScale())
	x0 := math.Round(x)
	y = math.Round(y)
	cx := x0
	for _, r := range s {
		img, w := a.text.glyph(r)
		if img != nil {
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Scale(k, k)
			op.GeoM.Translate(cx, y)
			op.ColorScale.ScaleWithColor(clr)
			dst.DrawImage(img, op)
		}
		cx += float64(w+a.text.font.Spacing) * k
	}
	return cx - x0
}

// textWidth measures s; the size argument is ignored.
func (a *app) textWidth(s string, _ float64) float64 {
	k := a.textScale()
	n := 0
	for _, r := range s {
		_, w := a.text.glyph(r)
		n += w + a.text.font.Spacing
	}
	return float64(n * k)
}
