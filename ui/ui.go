// Package ui is the Ebitengine front end: game loop, shader canvas, panels and
// input mapping. It is the only package (with cmd) that imports graphics.
package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/editor"
	"github.com/Castux/mipsim2/netlist"
	"github.com/Castux/mipsim2/platform"
)

// Options configures Run.
type Options struct {
	Title string
	Doc   *doc.Document // document to edit; a new one if nil
	Path  string        // where ctrl+s saves; empty if unnamed

	// For scripted checks and demos:
	Screenshot string   // save the screen to this PNG after a few frames and quit
	Simulate   bool     // start in simulate mode
	Sets       []string // name=value pins applied after entering simulate mode
	Zoom       float64  // fixed zoom (screen pixels per circuit pixel); 0 fits the circuit
	Filter     int      // zoomed-out filter: 0 average, 1 contrast boost, 2 any-on
	Frames     int      // with Screenshot: render this many extra frames without vsync and print the average frame time
}

var background = color.RGBA{255, 255, 255, 255}

const statusLines = 2

// prompt is an in-app text field for a file path (open, save as).
type prompt struct {
	kind   uiAction
	buffer string
}

type app struct {
	opts   Options
	ed     *editor.Editor
	view   editor.View
	canvas *canvas
	face   *text.GoTextFace

	scale       float64 // device scale factor
	w, h        int
	fitted      bool
	frames      int
	err         error
	prompt      *prompt
	panelScroll int

	shotPath string // save the next frame at or after shotAt here
	shotAt   int
	shotQuit bool // quit after saving (the -screenshot flag)
	timeFrom time.Time

	panning   bool
	panFrom   [2]int
	pressed   [3]bool
	lastWorld image.Point
}

func newApp(opts Options) (*app, error) {
	d := opts.Doc
	if d == nil {
		d = doc.New()
	}
	c, err := newCanvas()
	if err != nil {
		return nil, fmt.Errorf("canvas shader: %w", err)
	}
	c.filter = opts.Filter
	src, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		return nil, err
	}
	a := &app{opts: opts, ed: editor.New(d), view: editor.NewView(), canvas: c, face: &text.GoTextFace{Source: src}}
	if opts.Screenshot != "" {
		a.shotPath, a.shotAt, a.shotQuit = opts.Screenshot, 5+opts.Frames, true
		if opts.Frames > 0 {
			ebiten.SetVsyncEnabled(false)
		}
	}
	if opts.Simulate {
		a.ed.Do(editor.ActToggleSimulate)
		if r := a.ed.Runner(); r != nil {
			for _, kv := range opts.Sets {
				name, value, _ := strings.Cut(kv, "=")
				if err := r.Set(name, value); err != nil {
					return nil, err
				}
			}
			r.Settle()
		}
	}
	return a, nil
}

func (a *app) statusHeight() int {
	return int(float64(statusLines)*a.lineHeight()) + int(8*a.scale)
}

func (a *app) lineHeight() float64 { return 18 * a.scale }

func (a *app) canvasArea() image.Rectangle {
	return image.Rect(0, 0, a.w-a.panelWidth(), a.h-a.statusHeight())
}

func (a *app) fit() {
	area := a.canvasArea()
	a.view.Fit(a.ed.Netlist().Bounds().Inset(2), float64(area.Dx()), float64(area.Dy()))
}

func (a *app) Update() error {
	if a.err != nil {
		return a.err
	}
	if !a.fitted && a.w > 0 {
		a.fitted = true
		a.fit()
		if a.opts.Zoom > 0 {
			c := a.canvasArea().Size().Div(2)
			for a.view.Scale < a.opts.Zoom {
				a.view.Zoom(1, float64(c.X), float64(c.Y))
			}
			for a.view.Scale > a.opts.Zoom {
				a.view.Zoom(-1, float64(c.X), float64(c.Y))
			}
		}
	}
	if !a.handleTyping() {
		a.handleKeys()
	}
	a.handlePointer()
	return nil
}

func (a *app) mods() editor.Mods {
	return editor.Mods{
		Shift: ebiten.IsKeyPressed(ebiten.KeyShift),
		Ctrl:  ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta),
		Alt:   ebiten.IsKeyPressed(ebiten.KeyAlt),
	}
}

// handleTyping feeds keyboard input to a file prompt or a label being typed,
// and reports whether it took the keyboard this frame.
func (a *app) handleTyping() bool {
	chars := ebiten.AppendInputChars(nil)
	pressed := inpututil.IsKeyJustPressed
	if p := a.prompt; p != nil {
		p.buffer += string(chars)
		switch {
		case pressed(ebiten.KeyEnter) || pressed(ebiten.KeyNumpadEnter):
			a.prompt = nil
			a.runPrompt(p)
		case pressed(ebiten.KeyEscape):
			a.prompt = nil
			a.ed.Status = "cancelled"
		case pressed(ebiten.KeyBackspace):
			if r := []rune(p.buffer); len(r) > 0 {
				p.buffer = string(r[:len(r)-1])
			}
		}
		return true
	}
	if _, typing := a.ed.Typing(); typing {
		for _, r := range chars {
			a.ed.TypeRune(r)
		}
		switch {
		case pressed(ebiten.KeyEnter) || pressed(ebiten.KeyNumpadEnter):
			a.ed.Do(editor.ActEnter)
		case pressed(ebiten.KeyEscape):
			a.ed.Do(editor.ActEscape)
		case pressed(ebiten.KeyBackspace):
			a.ed.Do(editor.ActBackspace)
		}
		return true
	}
	return false
}

func (a *app) handleKeys() {
	m := a.mods()
	for _, k := range inpututil.AppendJustPressedKeys(nil) {
		b, ok := lookupKey(k, m, a.ed.Mode())
		if !ok {
			continue
		}
		switch b.ui {
		case uiNone:
			a.ed.Do(b.action)
		case uiSave:
			if a.opts.Path == "" {
				a.prompt = &prompt{kind: uiSaveAs, buffer: "circuit.mip"}
			} else {
				a.save(a.opts.Path)
			}
		case uiSaveAs:
			a.prompt = &prompt{kind: uiSaveAs, buffer: a.opts.Path}
		case uiOpen:
			a.prompt = &prompt{kind: uiOpen, buffer: a.opts.Path}
		case uiFit:
			a.fit()
		case uiFilter:
			a.canvas.filter = (a.canvas.filter + 1) % 3
			a.ed.Status = "zoomed-out filter: " + [...]string{"average", "contrast boost", "any-on"}[a.canvas.filter]
		case uiScreenshot:
			a.shotPath, a.shotAt = fmt.Sprintf("mipsim-%d.png", a.frames), a.frames+1
		}
	}
}

func (a *app) runPrompt(p *prompt) {
	path := strings.TrimSpace(p.buffer)
	if path == "" {
		a.ed.Status = "no file name given"
		return
	}
	switch p.kind {
	case uiSaveAs:
		a.save(path)
	case uiOpen:
		platform.ReadFile(path, func(data []byte, err error) {
			if err != nil {
				a.ed.Status = "open failed: " + err.Error()
				return
			}
			d, err := doc.Load(data)
			if err != nil {
				a.ed.Status = "open failed: " + firstLine(err.Error())
				return
			}
			a.ed.ReplaceDocument(d)
			a.setPath(path)
			a.fit()
			a.ed.Status = "opened " + path
		})
	}
}

func (a *app) setPath(path string) {
	a.opts.Path = path
	ebiten.SetWindowTitle("MiPSim — " + path)
}

func (a *app) save(path string) {
	data, err := a.ed.Doc.Save()
	if err != nil {
		a.ed.Status = "save failed: " + firstLine(err.Error())
		return
	}
	platform.WriteFile(path, data, func(err error) {
		if err != nil {
			a.ed.Status = "save failed: " + err.Error()
			return
		}
		a.setPath(path)
		a.ed.Status = "saved " + path
	})
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return s
}

var buttons = [3]ebiten.MouseButton{ebiten.MouseButtonLeft, ebiten.MouseButtonRight, ebiten.MouseButtonMiddle}

func (a *app) handlePointer() {
	cx, cy := ebiten.CursorPosition()
	cur := image.Pt(cx, cy)
	area := a.canvasArea()
	inCanvas := cur.In(area)

	if _, wy := ebiten.Wheel(); wy != 0 {
		n := 1
		if wy < 0 {
			n = -1
		}
		switch {
		case inCanvas:
			a.view.Zoom(n, float64(cx), float64(cy))
		case cur.In(a.panelArea()):
			a.panelScroll -= n
		}
	}
	if cur.In(a.panelArea()) && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		a.panelClick(cur)
		return
	}

	// Space+drag pans.
	if ebiten.IsKeyPressed(ebiten.KeySpace) && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		a.panning, a.panFrom = true, [2]int{cx, cy}
	}
	if a.panning {
		if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
			a.panning = false
		} else {
			a.view.Pan(float64(cx-a.panFrom[0]), float64(cy-a.panFrom[1]))
			a.panFrom = [2]int{cx, cy}
		}
		return
	}

	p := a.view.ToWorld(float64(cx), float64(cy))
	m := a.mods()
	for i, b := range buttons {
		switch {
		case inpututil.IsMouseButtonJustPressed(b) && inCanvas:
			a.pressed[i] = true
			a.ed.PointerDown(p, editor.Button(i), m)
		case a.pressed[i] && inpututil.IsMouseButtonJustReleased(b):
			a.pressed[i] = false
			a.ed.PointerUp(p, editor.Button(i))
		}
	}
	if p != a.lastWorld || a.pressed[0] {
		a.lastWorld = p
		a.ed.PointerMove(p, m)
	}
}

func (a *app) Draw(screen *ebiten.Image) {
	screen.Fill(background)
	area := a.canvasArea()
	nl := a.ed.Netlist()
	a.canvas.sync(a.ed)
	_, hoverNet, hoverText := a.ed.Hover()
	canvasImg := screen.SubImage(area).(*ebiten.Image)
	a.canvas.draw(canvasImg, area, &a.view, hoverNet, a.ed.Mode() == editor.SimulateMode)
	if a.ed.Stale() {
		cells, value := a.ed.Stroke()
		drawStroke(canvasImg, &a.view, cells, value)
	}
	a.drawLabels(canvasImg)
	a.drawDiagnosticMarkers(canvasImg, nl)
	a.drawOverlay(canvasImg, a.ed.Overlay())
	a.drawPanel(screen)
	a.drawStatus(screen, hoverText)
	a.screenshot(screen)
}

func (a *app) screenshot(screen *ebiten.Image) {
	a.frames++
	if a.frames == 5 {
		a.timeFrom = time.Now()
	}
	if a.shotQuit && a.opts.Frames > 0 && a.frames == a.shotAt {
		fmt.Printf("average frame time over %d frames: %v\n", a.opts.Frames, time.Since(a.timeFrom)/time.Duration(a.opts.Frames))
	}
	if a.shotPath == "" || a.frames < a.shotAt {
		return
	}
	path := a.shotPath
	a.shotPath = ""
	data, err := encodePNG(screen)
	if err != nil {
		a.err = err
		return
	}
	platform.WriteFile(path, data, func(err error) {
		switch {
		case err != nil && a.shotQuit:
			a.err = err
		case err != nil:
			a.ed.Status = "screenshot failed: " + err.Error()
		case a.shotQuit:
			a.err = ebiten.Termination
		default:
			a.ed.Status = "screenshot saved: " + path
		}
	})
}

func (a *app) drawStatus(screen *ebiten.Image, hover string) {
	y0 := float64(a.h - a.statusHeight())
	bar := screen.SubImage(image.Rect(0, int(y0), a.w, a.h)).(*ebiten.Image)
	bar.Fill(color.RGBA{238, 238, 238, 255})

	a.face.Size = 14 * a.scale
	nl := a.ed.Netlist()
	errs, warns := 0, 0
	for _, d := range nl.Diagnostics {
		if d.Level == netlist.Error {
			errs++
		} else {
			warns++
		}
	}
	zoom := fmt.Sprintf("%gx", a.view.Scale)
	if a.view.Scale < 1 {
		zoom = fmt.Sprintf("1/%gx", 1/a.view.Scale)
	}
	file := a.opts.Path
	if file == "" {
		file = "untitled"
	}
	line1 := fmt.Sprintf("%s | %s | %s | %s | %d nets, %d transistors, %d errors, %d warnings | %s",
		file, a.ed.Mode(), a.ed.Tool(), zoom, len(nl.Nets), len(nl.Transistors), errs, warns, hover)
	line2 := a.ed.Status
	if p := a.prompt; p != nil {
		line2 = map[uiAction]string{uiOpen: "open: ", uiSaveAs: "save as: "}[p.kind] + p.buffer + "_   (enter to confirm, escape to cancel)"
	} else if line2 == "" {
		line2 = helpLine(a.ed.Mode())
	}
	for i, s := range []string{line1, line2} {
		op := &text.DrawOptions{}
		op.GeoM.Translate(8*a.scale, y0+4*a.scale+float64(i)*a.lineHeight())
		op.ColorScale.ScaleWithColor(color.RGBA{20, 20, 20, 255})
		text.Draw(screen, s, a.face, op)
	}
}

func (a *app) Layout(w, h int) (int, int) {
	a.scale = ebiten.Monitor().DeviceScaleFactor()
	a.w, a.h = int(float64(w)*a.scale), int(float64(h)*a.scale)
	return a.w, a.h
}

// Run opens the editor window and blocks until it is closed.
func Run(opts Options) error {
	if opts.Title == "" {
		opts.Title = "MiPSim"
		if opts.Path != "" {
			opts.Title = "MiPSim — " + opts.Path
		}
	}
	a, err := newApp(opts)
	if err != nil {
		return err
	}
	ebiten.SetWindowSize(1280, 800)
	ebiten.SetWindowTitle(opts.Title)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	err = ebiten.RunGame(a)
	if err == ebiten.Termination {
		return nil
	}
	return err
}

func encodePNG(img *ebiten.Image) ([]byte, error) {
	rgba := image.NewRGBA(img.Bounds())
	img.ReadPixels(rgba.Pix)
	var b bytes.Buffer
	if err := png.Encode(&b, rgba); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
