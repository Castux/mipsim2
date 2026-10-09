// Package netlist is the circuit compiler: it checks the cells of a flat
// document, builds nets and transistors, names nets from labels, and reports
// located diagnostics. See docs/SPEC.md, "Cells" and "Netlist".
package netlist

import (
	"fmt"
	"image"
	"strings"

	"github.com/Castux/mipsim2/bitmap"
)

// NetID identifies a net. Nets are numbered in raster order of their first pixel.
type NetID int32

// NoNet marks pixels that belong to no net (transistor centres, off pixels).
const NoNet NetID = -1

// Drive is what the sources in a net drive it to.
type Drive uint8

const (
	DriveNone Drive = iota
	DriveHigh
	DriveLow // also when the net has both kinds of source: low wins
)

func (d Drive) String() string {
	return [...]string{"none", "high", "low"}[d]
}

// Net is one connected set of wire and source pixels.
type Net struct {
	Drive Drive
	Names []string    // hierarchical labels, in flatten order
	First image.Point // its first pixel in raster order
}

// Transistor is an n-MOS switch: A and B conduct when Gate is high.
type Transistor struct {
	Gate NetID
	A, B NetID       // channel ends, interchangeable
	Pos  image.Point // the centre pixel
}

// Role is what a pixel was classified as.
type Role uint8

const (
	RoleOff        Role = iota
	RoleWire            // on pixel joining its orthogonal neighbours
	RoleHigh            // power cell
	RoleLow             // ground cell
	RoleTransistor      // transistor cell
	RoleBridge          // bridge cell, joining north–south and west–east
	RoleInvalid         // transistor or bridge cell breaking its neighbour rule
)

// Letter returns the character used in classification goldens.
func (r Role) Letter() byte { return ".wHLTBX"[r] }

// Level is the severity of a diagnostic.
type Level uint8

const (
	Error Level = iota // blocks simulation
	Warning
)

func (l Level) String() string {
	if l == Error {
		return "error"
	}
	return "warning"
}

// Diagnostic is a located compile error or warning.
type Diagnostic struct {
	Code  string // e.g. "E_TRANSISTOR_ARMS"
	Level Level
	Pos   image.Point // world coordinates
	Msg   string
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%d,%d: %s %s: %s", d.Pos.X, d.Pos.Y, d.Level, d.Code, d.Msg)
}

// Options configures Compile. There are none yet.
type Options struct{}

// Netlist is the compiled circuit.
type Netlist struct {
	Nets        []Net
	Transistors []Transistor
	GatedBy     [][]int32 // net -> indices of transistors it gates
	ChannelOf   [][]int32 // net -> indices of transistors it is a channel end of
	Diagnostics []Diagnostic

	names  map[string]NetID
	grid   *bitmap.Dense
	roles  []Role  // per on pixel, by grid index
	pixNet []NetID // per on pixel, by grid index
}

// HasErrors reports whether any diagnostic is an error.
func (n *Netlist) HasErrors() bool {
	for _, d := range n.Diagnostics {
		if d.Level == Error {
			return true
		}
	}
	return false
}

// Lookup returns the net carrying the given full label name.
func (n *Netlist) Lookup(name string) (NetID, bool) {
	id, ok := n.names[name]
	return id, ok
}

// RoleAt returns the role of the pixel at (x, y).
func (n *Netlist) RoleAt(x, y int) Role {
	if i := n.grid.Index(x, y); i >= 0 {
		return n.roles[i]
	}
	return RoleOff
}

// NetAt returns the net of the pixel at (x, y), or NoNet.
func (n *Netlist) NetAt(x, y int) NetID {
	if i := n.grid.Index(x, y); i >= 0 {
		return n.pixNet[i]
	}
	return NoNet
}

// Bounds returns a rectangle containing every classified pixel (on pixels
// and bridge gaps).
func (n *Netlist) Bounds() image.Rectangle {
	return n.grid.Rect
}

// ForEachPixel calls fn for every non-empty cell in raster order. net is
// NoNet for transistor and bridge cells.
func (n *Netlist) ForEachPixel(fn func(x, y int, role Role, net NetID)) {
	n.grid.ForEach(func(i, x, y int) { fn(x, y, n.roles[i], n.pixNet[i]) })
}

// RoleMap renders the roles inside r as one string per row, using
// Role.Letter. This is the format of classification goldens.
func (n *Netlist) RoleMap(r image.Rectangle) []string {
	rows := make([]string, 0, r.Dy())
	var b strings.Builder
	for y := r.Min.Y; y < r.Max.Y; y++ {
		b.Reset()
		for x := r.Min.X; x < r.Max.X; x++ {
			b.WriteByte(n.RoleAt(x, y).Letter())
		}
		rows = append(rows, b.String())
	}
	return rows
}
