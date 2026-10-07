// Command ebitenui is the M0 spike for UI widgets (PLAN.md section 7).
//
// It lays out a tool panel (labels, buttons, a text input, a status line)
// next to a custom-drawn pixel canvas, which is the shape of the editor.
// Clicking the canvas toggles pixels unless the cursor is over a widget.
//
//	go run ./spikes/ebitenui                       # interactive
//	go run ./spikes/ebitenui -shot out.png         # render a few frames, save a screenshot, exit
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/themes"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
)

const (
	gridW, gridH = 64, 48
	scale        = 10
	panelWidth   = 220
)

type game struct {
	ui     *ebitenui.UI
	canvas *widget.Container
	status *widget.Text
	tool   string
	pixels [gridW * gridH]bool

	shot   string
	frames int
}

func newGame(shot string) *game {
	g := &game{tool: "pencil", shot: shot}

	// Seed the canvas with an inverter-like shape so the screenshot shows something.
	art := []string{
		"###",
		"###",
		"###",
		".#.",
		"#########",
		".#.",
		"##.",
		".#.",
		"###",
		"#.#",
		"###",
	}
	for y, row := range art {
		for x, c := range row {
			g.pixels[(y+4)*gridW+x+4] = c == '#'
		}
	}

	root := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Stretch([]bool{false, true}, []bool{true}),
		)),
	)

	panel := widget.NewPanel(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(8),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(12)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(panelWidth, 0)),
	)
	root.AddChild(panel)

	stretch := widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})

	panel.AddChild(widget.NewLabel(widget.LabelOpts.Text("Tools", nil, nil)))
	for _, name := range []string{"pencil", "select", "label", "simulate"} {
		panel.AddChild(widget.NewButton(
			widget.ButtonOpts.WidgetOpts(stretch),
			widget.ButtonOpts.TextLabel(name),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) {
				g.tool = name
				g.updateStatus()
			}),
		))
	}

	panel.AddChild(widget.NewLabel(widget.LabelOpts.Text("Net name", nil, nil)))
	panel.AddChild(widget.NewTextInput(
		widget.TextInputOpts.WidgetOpts(stretch),
		widget.TextInputOpts.Placeholder("e.g. alu.sum_3"),
		widget.TextInputOpts.SubmitHandler(func(args *widget.TextInputChangedEventArgs) {
			g.status.Label = "label: " + args.InputText
		}),
	))

	g.status = widget.NewText(widget.TextOpts.Text("", nil, color.White))
	panel.AddChild(g.status)

	g.canvas = widget.NewContainer()
	root.AddChild(g.canvas)

	g.ui = &ebitenui.UI{Container: root, PrimaryTheme: themes.GetBasicDarkTheme()}
	g.updateStatus()
	return g
}

func (g *game) updateStatus() {
	g.status.Label = "tool: " + g.tool
}

func (g *game) canvasOrigin() image.Point {
	return g.canvas.GetWidget().Rect.Min.Add(image.Pt(16, 16))
}

func (g *game) Update() error {
	g.ui.Update()

	if g.tool == "pencil" && !input.UIHovered && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		o := g.canvasOrigin()
		x, y := (mx-o.X)/scale, (my-o.Y)/scale
		if mx >= o.X && my >= o.Y && x < gridW && y < gridH {
			g.pixels[y*gridW+x] = true
		}
	}
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{24, 24, 28, 255})

	o := g.canvasOrigin()
	cell := ebiten.NewImage(scale-1, scale-1)
	defer cell.Deallocate()
	on, off := color.RGBA{230, 200, 80, 255}, color.RGBA{40, 40, 48, 255}
	for y := range gridH {
		for x := range gridW {
			if g.pixels[y*gridW+x] {
				cell.Fill(on)
			} else {
				cell.Fill(off)
			}
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(float64(o.X+x*scale), float64(o.Y+y*scale))
			screen.DrawImage(cell, op)
		}
	}

	g.ui.Draw(screen)

	g.frames++
	if g.shot != "" && g.frames == 10 {
		if err := savePNG(screen, g.shot); err != nil {
			log.Fatal(err)
		}
		fmt.Println("saved", g.shot)
		os.Exit(0)
	}
}

func (g *game) Layout(w, h int) (int, int) { return w, h }

func savePNG(img *ebiten.Image, path string) error {
	b := img.Bounds()
	rgba := image.NewRGBA(b)
	img.ReadPixels(rgba.Pix)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, rgba)
}

func main() {
	shot := flag.String("shot", "", "save a screenshot after a few frames and exit")
	flag.Parse()

	ebiten.SetWindowSize(panelWidth+gridW*scale+32, gridH*scale+32)
	ebiten.SetWindowTitle("ebitenui spike")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	if err := ebiten.RunGame(newGame(*shot)); err != nil {
		log.Fatal(err)
	}
}
