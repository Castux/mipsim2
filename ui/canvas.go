package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/Castux/mipsim2/editor"
	"github.com/Castux/mipsim2/netlist"
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

func byteOf(x float) int {
	return int(floor(x*255 + 0.5))
}

func background() vec3 {
	return vec3(0.09, 0.09, 0.11)
}

// colorAt returns the colour of the circuit pixel containing pos (texture
// coordinates including the image origin), and whether it is on.
func colorAt(pos vec2) vec4 {
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
	if net > 0 {
		w := int(StateWidth)
		row := net / w
		col := net - row*w
		s := imageSrc1At(imageSrc0Origin() + vec2(float(col), float(row)) + 0.5)
		state = byteOf(s.r)
	}

	col := vec3(0.55, 0.6, 0.7) // wire, edit mode
	if role == 2 {
		col = vec3(0.9, 0.5, 0.35) // high source
	} else if role == 3 {
		col = vec3(0.35, 0.5, 0.9) // low source
	} else if role == 4 {
		col = vec3(0.85, 0.8, 0.4) // transistor centre
	} else if role == 5 {
		col = vec3(0.16, 0.16, 0.2) // bridge gap
	} else if role == 6 {
		col = vec3(1, 0.1, 0.1) // malformed thick region
	}

	if Simulating > 0.5 && role != 5 && role != 6 {
		if role == 4 {
			col = vec3(0.25, 0.35, 0.3) // gate not high: open
			if state == 1 {
				col = vec3(0.4, 1, 0.5) // conducting
			}
		} else if state == 1 {
			col = vec3(1, 0.85, 0.3) // high
		} else if state == 2 {
			col = vec3(0.22, 0.28, 0.45) // low
		} else if state == 3 {
			col = vec3(1, 0.2, 0.8) // unstable
		} else {
			col = vec3(0.45, 0.45, 0.5) // floating
		}
		// Keep sources recognisable: tint by kind.
		if role == 2 {
			col = mix(col, vec3(0.9, 0.5, 0.35), 0.3)
		} else if role == 3 {
			col = mix(col, vec3(0.35, 0.5, 0.9), 0.3)
		}
	}

	if Hover > 0.5 && float(net) == Hover && role != 4 {
		col = mix(col, vec3(1, 1, 1), 0.4)
	}
	return vec4(col, 1)
}

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	bg := background()
	if Scale >= 1 {
		c := colorAt(floor(srcPos) + 0.5)
		col := c.rgb
		if Scale >= 8 {
			// Faint grid on cell borders.
			f := fract(srcPos - imageSrc0Origin())
			edge := 1 / Scale
			if f.x < edge || f.y < edge {
				col = col * 0.82
				if c.a < 0.5 {
					col = bg + vec3(0.03, 0.03, 0.04)
				}
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
				c := colorAt(floor(srcPos + off) + 0.5)
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
		c.statePix[i] = byte(e.Value(netlist.NetID(id)))
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
	clr := color.RGBA{140, 150, 175, 255}
	if !value {
		clr = color.RGBA{23, 23, 28, 255}
	}
	s := float32(max(v.Scale, 1))
	for _, p := range cells {
		x, y := v.ToScreen(p)
		vector.FillRect(dst, float32(x), float32(y), s, s, clr, false)
	}
}
