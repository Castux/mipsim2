package bitmap

import (
	"fmt"
	"image"
	"strings"
)

// Row-string codec: one string per row, one character per cell: '.' empty,
// '#' wire, 'H' power, 'L' ground, 'T' transistor, 'B' bridge. Used by the
// .mip file format and the clipboard.

// KindChars maps each kind to its character in rows.
const KindChars = ".#HLTB"

// KindOf returns the kind written as c, or false.
func KindOf(c byte) (Kind, bool) {
	for k := range numKinds {
		if KindChars[k] == c {
			return k, true
		}
	}
	return Empty, false
}

// EncodeRows returns the rows of r as strings, with trailing '.' characters
// and trailing empty rows omitted. Pixels outside r are ignored.
func (b *Bitmap) EncodeRows(r image.Rectangle) []string {
	r = r.Canon()
	rows := make([][]byte, r.Dy())
	b.ForEachCellIn(r, func(x, y int, k Kind) {
		row := &rows[y-r.Min.Y]
		lx := x - r.Min.X
		for len(*row) <= lx {
			*row = append(*row, '.')
		}
		(*row)[lx] = KindChars[k]
	})
	n := len(rows)
	for n > 0 && len(rows[n-1]) == 0 {
		n--
	}
	out := make([]string, n)
	for i := range n {
		out[i] = string(rows[i])
	}
	return out
}

// DecodeRows sets the cells described by rows, with the first character of
// the first row at origin. Empty cells ('.') are left unchanged.
func (b *Bitmap) DecodeRows(rows []string, origin image.Point) error {
	for y, row := range rows {
		for x := 0; x < len(row); x++ {
			k, ok := KindOf(row[x])
			if !ok {
				return fmt.Errorf("row %d, column %d: unexpected character %q (want one of %q)", y, x, row[x], KindChars)
			}
			if k != Empty {
				b.Put(origin.X+x, origin.Y+y, k)
			}
		}
	}
	return nil
}

// FromRows builds a bitmap from rows with the top-left character at (0, 0).
func FromRows(rows ...string) (*Bitmap, error) {
	b := New()
	if err := b.DecodeRows(rows, image.Point{}); err != nil {
		return nil, err
	}
	return b, nil
}

// MustFromRows is FromRows for tests and literals; it panics on bad input.
func MustFromRows(rows ...string) *Bitmap {
	b, err := FromRows(rows...)
	if err != nil {
		panic(err)
	}
	return b
}

// String renders the bitmap's bounding box as rows, for debugging and test failures.
func (b *Bitmap) String() string {
	r := b.Bounds()
	if r.Empty() {
		return "(empty)"
	}
	return fmt.Sprintf("origin %d,%d\n%s", r.Min.X, r.Min.Y, strings.Join(b.EncodeRows(r), "\n"))
}
