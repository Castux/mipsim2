// Package synth generates large synthetic circuits for benchmarks and scale
// tests, until real processor-scale designs exist (PLAN.md, M9).
package synth

import (
	"fmt"

	"github.com/Castux/mipsim2/bitmap"
	"github.com/Castux/mipsim2/doc"
)

// Inverter is the plan's inverter in the old pattern language (migrated to
// typed cells when used): input at (0,6), output at (8,4).
var Inverter = []string{
	"###......",
	"###......",
	"###......",
	".#.......",
	"#########",
	".#.......",
	"##.......",
	".#.......",
	"###......",
	"#.#......",
	"###......",
}

const (
	pitchX = 12 // inverter width 9 plus a 3-column connecting wire
	pitchY = 12 // inverter height 11 plus a blank row
)

// InverterChains returns a document with `rows` chains of `cols` inverter
// instances each. In chain r, each inverter's output drives the next one's
// input; the first input is labelled in_r and the last output out_r. The
// root holds the connecting wires; the inverters are instances of one
// definition, so flattening is exercised too. Each inverter cell is 44 on
// pixels, so 152×152 is about a million pixels.
func InverterChains(rows, cols int) *doc.Document {
	d := doc.New()
	inv := doc.NewDefinition("inv", 9, 11)
	inv.Pixels = doc.MigratePatterns(bitmap.MustFromRows(Inverter...))
	d.Defs["inv"] = inv

	root := d.RootDef()
	px := root.Pixels
	for r := range rows {
		y0 := r * pitchY
		// Input stub left of the first inverter.
		px.Set(-1, y0+6, true)
		root.Labels = append(root.Labels, doc.Label{X: -1, Y: y0 + 6, Name: fmt.Sprintf("in_%d", r)})
		for c := range cols {
			x0 := c * pitchX
			root.Instances = append(root.Instances, doc.Instance{
				ID:   doc.InstID(fmt.Sprintf("i%d_%d", r, c)),
				Def:  "inv",
				X:    x0,
				Y:    y0,
				Name: fmt.Sprintf("g%d_%d", r, c),
			})
			if c == cols-1 {
				px.Set(x0+9, y0+4, true)
				root.Labels = append(root.Labels, doc.Label{X: x0 + 9, Y: y0 + 4, Name: fmt.Sprintf("out_%d", r)})
				continue
			}
			// Output (8,4) to the next input (12,6): down then right.
			for _, p := range [5][2]int{{9, 4}, {9, 5}, {9, 6}, {10, 6}, {11, 6}} {
				px.Set(x0+p[0], y0+p[1], true)
			}
		}
	}
	return d
}
