package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/Castux/mipsim2/editor"
	"github.com/Castux/mipsim2/netlist"
	"github.com/Castux/mipsim2/sim"
)

// The canvas is drawn by one shader pass. Image 0 holds every classified
// pixel of the circuit: role and net packed in R, G, B (24 bits: role in the
// top 3, net ID + 1 in the low 21, 0 meaning no net). Transistor centres
// carry their gate's net, so they light up when conducting. Image 1 holds one
// state byte per net, indexed the same way. Only image 1 changes while
// simulating. See docs/progress.md, M0 and M5, for the measurements.
const canvasShader = `//kage:unit pixels

package main

var Scale float      // screen pixels per circuit pixel
var Samples float    // supersampling grid per axis when zoomed out (1..4)
var Simulating float // 1 in simulate mode
var Hover float      // net ID + 1 under the pointer, 0 for none
var Filter float     // zoomed out: 0 average, 1 contrast boost, 2 any-on
var StateWidth float // width of the state texture

// The palette is MiPSim v1's (style.css): white background, silver wires,
// red power, blue ground, purple transistors; pink high, light blue low,
// brown unstable; conducting transistors extend purple triangles into their
// channel arms (owner's request, replacing v1's green outline); pinned
// wires outlined #ff7d7d (high) or #6d6dff (low).

func byteOf(x float) int {
	return int(floor(x*255 + 0.5))
}

func background() vec3 {
	return vec3(1, 1, 1)
}

// idAt returns a key identifying what pixel pos belongs to, for outlines:
// the net ID + 1, or minus the role for pixels without a net.
func idAt(pos vec2) int {
	c := imageSrc0At(pos)
	r := byteOf(c.r)
	g := byteOf(c.g)
	b := byteOf(c.b)
	role := b / 32
	net := r + g*256 + (b-role*32)*65536
	if role == 4 || net == 0 {
		return -role - 10
	}
	return net
}

// roleAt returns the role of the pixel at pos.
func roleAt(pos vec2) int {
	return byteOf(imageSrc0At(pos).b) / 32
}

// isOn reports whether the pixel at pos is on (bridge gaps are off pixels).
func isOn(pos vec2) bool {
	r := roleAt(pos)
	return r != 0 && r != 5
}

// stateAt returns the simulated state of the net of the pixel at pos.
func stateAt(pos vec2) int {
	c := imageSrc0At(pos)
	r := byteOf(c.r)
	g := byteOf(c.g)
	b := byteOf(c.b)
	role := b / 32
	net := r + g*256 + (b-role*32)*65536
	if net == 0 {
		return 0
	}
	w := int(StateWidth)
	row := net / w
	col := net - row*w
	v := byteOf(imageSrc1At(imageSrc0Origin() + vec2(float(col), float(row)) + 0.5).r)
	return v - (v/4)*4
}

// wireColor is a wire's colour for a net state: silver in edit mode; in
// simulate mode silver floating, pink high, light blue low, brown unstable.
func wireColor(state int) vec3 {
	if Simulating > 0.5 {
		if state == 1 {
			return vec3(1, 0.753, 0.796)
		} else if state == 2 {
			return vec3(0.678, 0.847, 0.902)
		} else if state == 3 {
			return vec3(0.647, 0.165, 0.165)
		}
	}
	return vec3(0.753, 0.753, 0.753)
}

// beamColor is the colour of a bridge beam joining the arm pixel at pos,
// darkened like its wire when that net is hovered.
func beamColor(pos vec2) vec3 {
	c := wireColor(stateAt(pos))
	if Hover > 0.5 && float(idAt(pos)) == Hover {
		c = c * 0.8
	}
	return c
}

// inChannelTriangle reports whether point f of the cell at pos lies in the
// flat purple triangle a conducting transistor extends into each of its two
// channel arms: the base is the edge shared with the centre, the apex half
// way across the arm. Transistor centres carry their gate's net, and the
// channel runs across the axis of the centre's one off neighbour.
func inChannelTriangle(pos vec2, f vec2) bool {
	for i := 0; i < 4; i++ {
		d := vec2(1, 0)
		if i == 1 {
			d = vec2(-1, 0)
		} else if i == 2 {
			d = vec2(0, 1)
		} else if i == 3 {
			d = vec2(0, -1)
		}
		c := pos + d
		if roleAt(c) != 4 || stateAt(c) != 1 {
			continue
		}
		missingVertical := !isOn(c+vec2(0, 1)) || !isOn(c-vec2(0, 1))
		armHorizontal := d.x != 0
		if armHorizontal != missingVertical {
			continue // this arm is the gate
		}
		// t: distance from the shared edge; s: offset along it.
		t := f.x
		s := f.y - 0.5
		if i == 0 {
			t = 1 - f.x
		} else if i == 2 {
			t = 1 - f.y
			s = f.x - 0.5
		} else if i == 3 {
			t = f.y
			s = f.x - 0.5
		}
		if t <= 0.5*(1-2*abs(s)) {
			return true
		}
	}
	return false
}

// colorAt returns the colour of the circuit pixel containing pos (texture
// coordinates including the image origin), and whether it is on. With
// borders set, outlines (pins, conducting, errors) are drawn as a band
// inside the cell; f is the position within the cell.
func colorAt(pos vec2, borders bool, f vec2) vec4 {
	c := imageSrc0At(pos)
	r := byteOf(c.r)
	g := byteOf(c.g)
	b := byteOf(c.b)
	role := b / 32
	net := r + g*256 + (b-role*32)*65536
	if role == 0 {
		return vec4(background(), 0)
	}

	state := 0
	pin := 0
	if net > 0 {
		w := int(StateWidth)
		row := net / w
		col := net - row*w
		s := imageSrc1At(imageSrc0Origin() + vec2(float(col), float(row)) + 0.5)
		v := byteOf(s.r)
		pin = v / 4
		state = v - pin*4
	}

	silver := vec3(0.753, 0.753, 0.753)
	col := silver
	if role == 2 {
		col = vec3(1, 0, 0) // power (high source)
	} else if role == 3 {
		col = vec3(0, 0, 1) // ground (low source)
	} else if role == 4 {
		col = vec3(0.502, 0, 0.502) // transistor centre: purple
	} else if role == 5 {
		col = vec3(0.88, 0.88, 0.88) // bridge gap: half-transparent silver
	}

	if role == 1 {
		col = wireColor(state)
	}

	// A bridge gap, zoomed in, is a cross: each beam half a wire wide, coloured
	// like the net it joins (north–south, then west–east on top).
	if role == 5 && borders {
		col = background()
		if abs(f.x-0.5) < 0.25 {
			col = beamColor(pos - vec2(0, 1))
		}
		if abs(f.y-0.5) < 0.25 {
			col = beamColor(pos - vec2(1, 0))
		}
	}

	if Hover > 0.5 && float(net) == Hover && role != 4 && role != 5 {
		col = col * 0.8
	}

	// Outlines, as a band inside the cell when zoomed in enough.
	band := vec3(-1)
	if role == 6 {
		band = vec3(1, 0, 0) // malformed thick region
	} else if Simulating > 0.5 && role == 4 && state == 3 {
		band = vec3(0.647, 0.165, 0.165) // gate unstable
	} else if Simulating > 0.5 && role != 4 && pin == 1 {
		band = vec3(1, 0.49, 0.49) // pinned high
	} else if Simulating > 0.5 && role != 4 && pin == 2 {
		band = vec3(0.427, 0.427, 1) // pinned low
	}
	if band.r >= 0 {
		if !borders {
			if role == 4 || role == 6 {
				col = band // too small for a band: show the outline colour
			}
		} else {
			// Outline the shape, not each pixel: only edges facing a pixel
			// that is not part of the same net (or region).
			w := max(0.18, 1.5/Scale)
			key := idAt(pos)
			if role == 4 {
				key = -1 // a transistor centre is outlined on its own
			}
			if (f.x < w && idAt(pos-vec2(1, 0)) != key) || (f.x > 1-w && idAt(pos+vec2(1, 0)) != key) ||
				(f.y < w && idAt(pos-vec2(0, 1)) != key) || (f.y > 1-w && idAt(pos+vec2(0, 1)) != key) {
				col = band
			}
		}
	}
	// A conducting transistor's purple flows into its channel arms.
	if borders && Simulating > 0.5 && role != 4 && inChannelTriangle(pos, f) {
		col = vec3(0.502, 0, 0.502)
	}
	return vec4(col, 1)
}

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	bg := background()
	if Scale >= 1 {
		f := fract(srcPos - imageSrc0Origin())
		c := colorAt(floor(srcPos)+0.5, Scale >= 4, f)
		col := c.rgb
		if Scale >= 8 && c.a < 0.5 {
			// Light grid on empty cells.
			edge := 1 / Scale
			if f.x < edge || f.y < edge {
				col = vec3(0.93, 0.93, 0.94)
			}
		}
		return vec4(col, 1)
	}

	// Zoomed out: colour a grid of samples across the screen pixel's
	// footprint, then average. Averaging happens after colouring because
	// roles and nets cannot be interpolated.
	foot := 1 / Scale
	sum := vec3(0)
	on := 0.0
	total := 0.0
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			if float(i) < Samples && float(j) < Samples {
				off := (vec2(float(i), float(j)) + 0.5) / Samples * foot - foot/2
				c := colorAt(floor(srcPos + off) + 0.5, false, vec2(0.5))
				if c.a > 0.5 {
					sum += c.rgb
					on += 1
				}
				total += 1
			}
		}
	}
	if on == 0 {
		return vec4(bg, 1)
	}
	avg := sum / on
	coverage := on / total
	k := coverage
	if Filter > 1.5 {
		k = 1
	} else if Filter > 0.5 {
		k = sqrt(coverage)
	}
	return vec4(mix(bg, avg, k), 1)
}
`

// maxCanvasTexture is the largest circuit extent drawn in one texture.
// Tiling larger circuits is future work (see progress.md).
const maxCanvasTexture = 8192

const stateWidth = 1024

type canvas struct {
	shader *ebiten.Shader

	compiles int // editor compile count the textures were built from
	texRect  image.Rectangle
	ids      *ebiten.Image
	tooBig   bool

	states   *ebiten.Image
	statePix []byte
	nets     int

	filter int // zoomed-out filter, see the shader
}

func newCanvas() (*canvas, error) {
	sh, err := ebiten.NewShader([]byte(canvasShader))
	if err != nil {
		return nil, err
	}
	return &canvas{shader: sh, compiles: -1, filter: 1}, nil
}

// sync rebuilds the textures after a compile and refreshes net states.
func (c *canvas) sync(e *editor.Editor) {
	nl := e.Netlist()
	if e.Compiles() != c.compiles {
		c.compiles = e.Compiles()
		c.buildIDs(nl)
		c.buildStates(len(nl.Nets))
	}
	c.updateStates(e, nl)
}

func (c *canvas) buildIDs(nl *netlist.Netlist) {
	r := nl.Bounds()
	if r.Empty() {
		r = image.Rect(0, 0, 1, 1)
	}
	c.tooBig = r.Dx() > maxCanvasTexture || r.Dy() > maxCanvasTexture
	if c.tooBig {
		return
	}
	if c.ids == nil || c.texRect.Size() != r.Size() {
		if c.ids != nil {
			c.ids.Deallocate()
		}
		c.ids = ebiten.NewImage(r.Dx(), r.Dy())
	}
	c.texRect = r
	w := r.Dx()
	pix := make([]byte, 4*r.Dx()*r.Dy())
	put := func(x, y int, role netlist.Role, net netlist.NetID) {
		v := uint32(role)<<21 | uint32(net+1)
		i := 4 * ((y-r.Min.Y)*w + (x - r.Min.X))
		pix[i], pix[i+1], pix[i+2], pix[i+3] = byte(v), byte(v>>8), byte(v>>16), 255
	}
	nl.ForEachPixel(put)
	for _, t := range nl.Transistors {
		put(t.Pos.X, t.Pos.Y, netlist.RoleTransistor, t.Gate)
	}
	c.ids.WritePixels(pix)
}

func (c *canvas) buildStates(nets int) {
	rows := (nets+1)/stateWidth + 1
	c.nets = nets
	if c.states != nil {
		c.states.Deallocate()
	}
	c.states = ebiten.NewImage(stateWidth, rows)
	c.statePix = make([]byte, 4*stateWidth*rows)
}

func (c *canvas) updateStates(e *editor.Editor, nl *netlist.Netlist) {
	for id := range c.nets {
		i := 4 * (id + 1)
		n := netlist.NetID(id)
		var pin byte
		switch e.Pinned(n) {
		case sim.High:
			pin = 1
		case sim.Low:
			pin = 2
		}
		c.statePix[i] = byte(e.Value(n)) + 4*pin
		c.statePix[i+3] = 255
	}
	c.states.WritePixels(c.statePix)
}

// draw renders the circuit into area of dst.
func (c *canvas) draw(dst *ebiten.Image, area image.Rectangle, v *editor.View, hover netlist.NetID, simulating bool) {
	if c.tooBig || c.ids == nil {
		dst.Fill(color.RGBA{60, 20, 20, 255})
		return
	}
	// Screen corner -> texture coordinates: world - texRect.Min.
	toTex := func(sx, sy float64) (float32, float32) {
		wx := (sx-v.Offset[0])/v.Scale - float64(c.texRect.Min.X)
		wy := (sy-v.Offset[1])/v.Scale - float64(c.texRect.Min.Y)
		return float32(wx), float32(wy)
	}
	x0, y0, x1, y1 := float64(area.Min.X), float64(area.Min.Y), float64(area.Max.X), float64(area.Max.Y)
	vs := make([]ebiten.Vertex, 4)
	for i, p := range [4][2]float64{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}} {
		sx, sy := toTex(p[0], p[1])
		vs[i] = ebiten.Vertex{DstX: float32(p[0]), DstY: float32(p[1]), SrcX: sx, SrcY: sy, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}
	}
	samples := 1.0
	if v.Scale < 1 {
		samples = min(4, 1/v.Scale)
	}
	sim := float32(0)
	if simulating {
		sim = 1
	}
	op := &ebiten.DrawTrianglesShaderOptions{}
	op.Images[0] = c.ids
	op.Images[1] = c.states
	op.Uniforms = map[string]any{
		"Scale":      float32(v.Scale),
		"Samples":    float32(samples),
		"Simulating": sim,
		"Hover":      float32(hover + 1),
		"Filter":     float32(c.filter),
		"StateWidth": float32(stateWidth),
	}
	dst.DrawTrianglesShader(vs, []uint16{0, 1, 2, 1, 2, 3}, c.shader, op)
}

// drawStroke shows pixels changed by a stroke that has not been compiled yet.
func drawStroke(dst *ebiten.Image, v *editor.View, cells []image.Point, value bool) {
	clr := color.RGBA{192, 192, 192, 255}
	if !value {
		clr = color.RGBA{255, 255, 255, 255}
	}
	s := float32(max(v.Scale, 1))
	for _, p := range cells {
		x, y := v.ToScreen(p)
		vector.FillRect(dst, float32(x), float32(y), s, s, clr, false)
	}
}
