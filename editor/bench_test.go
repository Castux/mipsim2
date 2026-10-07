package editor

import (
	"image"
	"testing"
	"time"

	"github.com/Castux/mipsim2/internal/synth"
	"github.com/Castux/mipsim2/netlist"
)

func bigEditor(b *testing.B) *Editor {
	e := New(synth.InverterChains(152, 152))
	e.Netlist()
	return e
}

func BenchmarkEdNew(b *testing.B) {
	d := synth.InverterChains(152, 152)
	for b.Loop() {
		New(d)
	}
}

func BenchmarkEdInstances(b *testing.B) {
	e := bigEditor(b)
	e.hover = image.Pt(900, 900)
	for b.Loop() {
		e.Instances()
	}
}

func BenchmarkEdHover(b *testing.B) {
	e := bigEditor(b)
	e.hover = image.Pt(12*50+1, 12*50+4)
	for b.Loop() {
		e.Hover()
	}
}

func BenchmarkEdLocate(b *testing.B) {
	e := bigEditor(b)
	for b.Loop() {
		e.Doc.Locate(image.Pt(1800, 1800))
	}
}

func BenchmarkEdDefinitions(b *testing.B) {
	e := bigEditor(b)
	for b.Loop() {
		e.Definitions()
	}
}

// One pencil click in empty space at the root plus release: paint + recompile.
func BenchmarkEdStrokeClick(b *testing.B) {
	e := bigEditor(b)
	p := image.Pt(-10, -10)
	for b.Loop() {
		e.PointerDown(p, Left, Mods{})
		e.PointerUp(p, Left)
	}
}

// A 100-pixel drag stroke (moves only, no recompile because compile > 8ms).
func BenchmarkEdStrokeDrag100(b *testing.B) {
	e := bigEditor(b)
	for b.Loop() {
		e.PointerDown(image.Pt(-20, -20), Left, Mods{})
		for x := 1; x <= 100; x++ {
			e.PointerMove(image.Pt(-20+x, -20), Mods{})
		}
		b.StopTimer()
		e.PointerUp(image.Pt(-20+100, -20), Left)
		e.Undo()
		e.Netlist()
		b.StartTimer()
	}
}

func BenchmarkEdStartSim(b *testing.B) {
	e := bigEditor(b)
	for b.Loop() {
		e.setMode(SimulateMode)
		e.setMode(EditMode)
	}
}

// Building the canvas id texture bytes as ui.canvas.buildIDs does.
func BenchmarkEdBuildIDs(b *testing.B) {
	e := bigEditor(b)
	nl := e.Netlist()
	for b.Loop() {
		r := nl.Bounds()
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
	}
}

// updateStates as the canvas does per frame (without the GPU upload).
func BenchmarkEdUpdateStates(b *testing.B) {
	e := bigEditor(b)
	e.setMode(SimulateMode)
	nl := e.Netlist()
	pix := make([]byte, 4*(len(nl.Nets)+1))
	for b.Loop() {
		for id := range len(nl.Nets) {
			n := netlist.NetID(id)
			pix[4*(id+1)] = byte(e.Value(n)) + 4*byte(e.Pinned(n))
		}
	}
}

func BenchmarkEdHalfTick(b *testing.B) {
	e := bigEditor(b)
	e.setMode(SimulateMode)
	in := e.run
	_ = in
	for b.Loop() {
		e.run.Sim().Pin(0, 0+1)
		e.run.HalfTick()
	}
}

func TestEdReport(t *testing.T) {
	e := New(synth.InverterChains(152, 152))
	t.Logf("lastCompile %v", e.lastCompile)
	start := time.Now()
	e.selectArea(image.Pt(0, 0), image.Pt(900, 900))
	t.Logf("selectArea %v", time.Since(start))
}

func BenchmarkEdInstancesMoving(b *testing.B) {
	e := bigEditor(b)
	i := 0
	for b.Loop() {
		i++
		e.hover = image.Pt(900+i%7, 900)
		e.Instances()
	}
}
