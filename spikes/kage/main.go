// Command kage is the M0 spike for the canvas shader (PLAN.md section 7).
//
// It checks that a Kage shader can colour a large role/net-ID texture by
// looking up each net's state in a smaller state texture of a different size,
// which is the rendering approach the editor relies on. It renders off screen,
// reads the result back, compares every pixel with a CPU reference and prints
// PASS or FAIL with timings, then exits.
//
// Run with the default backend, then with OpenGL forced:
//
//	go run ./spikes/kage
//	EBITENGINE_GRAPHICS_LIBRARY=opengl go run ./spikes/kage
package main

import (
	"fmt"
	"math/rand/v2"
	"os"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	canvasSize = 4096 // role/ID texture is canvasSize x canvasSize
	stateSize  = 512  // state texture is stateSize x stateSize
	numNets    = stateSize * stateSize
	numRoles   = 5 // off, wire, high, low, transistor (bridge gaps are off pixels)
	timedDraws = 50
)

// Pixel encoding of the role/ID texture: 24 bits in R, G, B (A is 255).
// value = R | G<<8 | B<<16; role = value >> 21; net = value & (1<<21 - 1).
// Up to 2,097,152 nets and 8 roles.

const shaderSrc = `//kage:unit pixels

package main

func byteOf(x float) int {
	return int(floor(x*255 + 0.5))
}

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	c := imageSrc0UnsafeAt(srcPos)
	r := byteOf(c.r)
	g := byteOf(c.g)
	b := byteOf(c.b)
	role := b / 32
	net := r + g*256 + (b-role*32)*65536

	w := int(imageSrc1Size().x)
	row := net / w
	col := net - row*w
	// Positions for images 1..3 are given in image 0's coordinate space;
	// Ebitengine converts them with pos - origin0 + origin1. So address texel
	// (col, row) of image 1 relative to imageSrc0Origin, not imageSrc1Origin.
	s := imageSrc1At(imageSrc0Origin() + vec2(float(col), float(row)) + 0.5)
	state := byteOf(s.r) // 0 floating, 1 high, 2 low, 3 unstable

	if role == 0 {
		return vec4(0, 0, 0, 1)
	}
	// Colour channel layout chosen so the CPU reference is easy to compute:
	// R = role * 40, G = state * 60, B = 255.
	return vec4(float(role*40)/255, float(state*60)/255, 1, 1)
}
`

type spike struct {
	done   bool
	result error
}

func (s *spike) Update() error {
	if s.done {
		return ebiten.Termination
	}
	s.done = true
	s.result = run()
	return ebiten.Termination
}

func (s *spike) Draw(*ebiten.Image)         {}
func (s *spike) Layout(int, int) (int, int) { return 64, 64 }

func run() error {
	rng := rand.New(rand.NewPCG(1, 2))

	roles := make([]uint8, canvasSize*canvasSize)
	nets := make([]uint32, canvasSize*canvasSize)
	idPix := make([]byte, 4*canvasSize*canvasSize)
	for i := range roles {
		role := uint8(rng.IntN(numRoles))
		net := uint32(rng.IntN(numNets))
		// Make sure the highest net IDs and the texture corners are exercised.
		if i < 4 {
			net = numNets - 1 - uint32(i)
		}
		roles[i], nets[i] = role, net
		v := uint32(role)<<21 | net
		idPix[4*i+0] = byte(v)
		idPix[4*i+1] = byte(v >> 8)
		idPix[4*i+2] = byte(v >> 16)
		idPix[4*i+3] = 255
	}

	states := make([]uint8, numNets)
	statePix := make([]byte, 4*numNets)
	for i := range states {
		states[i] = uint8(rng.IntN(4))
		statePix[4*i+0] = states[i]
		statePix[4*i+3] = 255
	}

	shader, err := ebiten.NewShader([]byte(shaderSrc))
	if err != nil {
		return fmt.Errorf("compile shader: %w", err)
	}

	idImg := ebiten.NewImage(canvasSize, canvasSize)
	idImg.WritePixels(idPix)
	stateImg := ebiten.NewImage(stateSize, stateSize)
	stateImg.WritePixels(statePix)
	dst := ebiten.NewImage(canvasSize, canvasSize)

	draw := func() {
		w, h := float32(canvasSize), float32(canvasSize)
		vs := []ebiten.Vertex{
			{DstX: 0, DstY: 0, SrcX: 0, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			{DstX: w, DstY: 0, SrcX: w, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			{DstX: 0, DstY: h, SrcX: 0, SrcY: h, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			{DstX: w, DstY: h, SrcX: w, SrcY: h, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		}
		is := []uint16{0, 1, 2, 1, 2, 3}
		op := &ebiten.DrawTrianglesShaderOptions{}
		op.Images[0] = idImg
		op.Images[1] = stateImg
		dst.DrawTrianglesShader(vs, is, shader, op)
	}

	out := make([]byte, 4*canvasSize*canvasSize)

	// First draw and readback, checked against the CPU reference.
	start := time.Now()
	draw()
	dst.ReadPixels(out)
	first := time.Since(start)

	mismatches := 0
	firstBad := -1
	for i := range roles {
		var want [4]byte
		if roles[i] == 0 {
			want = [4]byte{0, 0, 0, 255}
		} else {
			want = [4]byte{roles[i] * 40, states[nets[i]] * 60, 255, 255}
		}
		got := [4]byte{out[4*i], out[4*i+1], out[4*i+2], out[4*i+3]}
		if got != want {
			if firstBad < 0 {
				firstBad = i
				fmt.Printf("first mismatch at (%d,%d): role %d net %d state %d: got %v want %v\n",
					i%canvasSize, i/canvasSize, roles[i], nets[i], states[nets[i]], got, want)
			}
			mismatches++
		}
	}

	// Timing: redraw after changing only the state texture, as the editor would.
	start = time.Now()
	for range timedDraws {
		statePix[0] ^= 1
		stateImg.WritePixels(statePix)
		draw()
	}
	dst.ReadPixels(out) // forces the GPU work to finish
	perDraw := time.Since(start) / timedDraws

	fmt.Printf("backend: %s\n", backendName())
	fmt.Printf("canvas %dx%d, state %dx%d, %d nets\n", canvasSize, canvasSize, stateSize, stateSize, numNets)
	fmt.Printf("first draw + readback: %v\n", first)
	fmt.Printf("state upload + full draw, average of %d: %v (16.7M pixels each)\n", timedDraws, perDraw)
	if mismatches > 0 {
		return fmt.Errorf("%d of %d pixels differ from the CPU reference", mismatches, len(roles))
	}
	return nil
}

func backendName() string {
	var info ebiten.DebugInfo
	ebiten.ReadDebugInfo(&info)
	name := info.GraphicsLibrary.String()
	if os.Getenv("EBITENGINE_GRAPHICS_LIBRARY") != "" {
		return name + " (forced)"
	}
	return name + " (default)"
}

func main() {
	ebiten.SetWindowSize(64, 64)
	ebiten.SetWindowTitle("kage spike")
	s := &spike{}
	if err := ebiten.RunGame(s); err != nil {
		fmt.Println("FAIL:", err)
		os.Exit(1)
	}
	if s.result != nil {
		fmt.Println("FAIL:", s.result)
		os.Exit(1)
	}
	fmt.Println("PASS")
}
