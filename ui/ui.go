// Package ui is the Ebitengine front end: game loop, shader canvas, panels and
// input mapping. It is the only package (with cmd) that imports graphics.
//
// Layout: a top bar (edit/simulate switch, file name, history and file
// buttons), a left column of tool and action buttons for the current mode, the
// canvas, a right panel with tabs when there is something to show, and two
// status lines (gesture hints, then position and messages). Every button
// shows its key, and every key and button comes from the keymap table.
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
	Tool       string   // start with this tool selected: draw, select or label
}

var background = color.RGBA{255, 255, 255, 255} // v1's white canvas

const statusLines = 2

// prompt is an in-app text field: a file path, or a value for a watched bus.
type prompt struct {
	kind   uiAction
	target string // the bus, for uiSetWatch
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
	tab         panelTab
	panelScroll int
	lastFrame   time.Time

	shotPath string // save the next frame at or after shotAt here
	shotAt   int
	shotQuit bool // quit after saving (the -screenshot flag)
	timeFrom time.Time

	panning     bool
	spaceDown   bool
	spacePanned bool // space was used to pan, so its release does not run/pause
	panFrom     [2]int
	pressed     [3]bool
	lastWorld   image.Point
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
	a := &app{opts: opts, ed: editor.New(d), view: editor.NewView(), canvas: c, face: &text.GoTextFace{Source: src}, tab: tabWatch}
	if opts.Screenshot != "" {
		a.shotPath, a.shotAt, a.shotQuit = opts.Screenshot, 5+opts.Frames, true
		if opts.Frames > 0 {
			ebiten.SetVsyncEnabled(false)
		}
	}
	switch opts.Tool {
	case "select":
		a.ed.Do(editor.ActSelect)
	case "label":
		a.ed.Do(editor.ActLabel)
	}
	a.ed.Status = ""
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
	return int(float64(statusLines)*a.lineHeight()) + a.u(10)
}

func (a *app) lineHeight() float64 { return 18 * a.scale }

func (a *app) fit() {
	a.view.Fit(a.ed.Netlist().Bounds().Inset(2), a.computeLayout().canvas)
}

func (a *app) Update() error {
	if a.err != nil {
		return a.err
	}
	now := time.Now()
	if !a.lastFrame.IsZero() {
		a.ed.Advance(now.Sub(a.lastFrame))
	}
	a.lastFrame = now

	if !a.fitted && a.w > 0 {
		a.fitted = true
		a.fit()
		if a.opts.Zoom > 0 {
			c := a.computeLayout().canvas
			mid := c.Min.Add(c.Size().Div(2))
			for a.view.Scale < a.opts.Zoom {
				a.view.Zoom(1, float64(mid.X), float64(mid.Y))
			}
			for a.view.Scale > a.opts.Zoom {
				a.view.Zoom(-1, float64(mid.X), float64(mid.Y))
			}
		}
	}
	l := a.computeLayout()
	if !a.handleTyping() {
		a.handleKeys()
	}
	a.handlePointer(l)
	return nil
}

func (a *app) mods() editor.Mods {
	return editor.Mods{
		Shift: ebiten.IsKeyPressed(ebiten.KeyShift),
		Ctrl:  ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta),
		Alt:   ebiten.IsKeyPressed(ebiten.KeyAlt),
	}
}

// handleTyping feeds keyboard input to a prompt or a label being typed,
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
		if b, ok := lookupKey(k, m, a.ed.Mode()); ok {
			a.trigger(b)
		}
	}
	// Space: held for panning; a tap (no pan) runs or pauses in simulate mode.
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		a.spaceDown, a.spacePanned = true, false
	}
	if a.spaceDown && inpututil.IsKeyJustReleased(ebiten.KeySpace) {
		a.spaceDown = false
		if !a.spacePanned {
			for _, b := range keymap {
				if b.onRelease && b.key == ebiten.KeySpace && b.applies(a.ed.Mode()) {
					a.trigger(b)
				}
			}
		}
	}
}

func (a *app) runPrompt(p *prompt) {
	value := strings.TrimSpace(p.buffer)
	if value == "" {
		a.ed.Status = "nothing entered"
		return
	}
	switch p.kind {
	case uiSaveAs:
		a.save(value)
	case uiOpen:
		platform.ReadFile(value, func(data []byte, err error) {
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
			a.setPath(value)
			a.fit()
			a.ed.Status = "opened " + value
		})
	case uiSetWatch:
		if r := a.ed.Runner(); r != nil {
			if err := r.Set(p.target, value); err != nil {
				a.ed.Status = err.Error()
				return
			}
			r.Settle()
			a.ed.Status = p.target + " = " + value
		}
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
		a.ed.MarkSaved()
		a.ed.Status = "saved " + path
	})
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return s
}

var mouseButtons = [3]ebiten.MouseButton{ebiten.MouseButtonLeft, ebiten.MouseButtonRight, ebiten.MouseButtonMiddle}

func (a *app) handlePointer(l layout) {
	cx, cy := ebiten.CursorPosition()
	cur := image.Pt(cx, cy)
	inCanvas := cur.In(l.canvas)

	if _, wy := ebiten.Wheel(); wy != 0 {
		n := 1
		if wy < 0 {
			n = -1
		}
		switch {
		case inCanvas:
			a.view.Zoom(n, float64(cx), float64(cy))
		case cur.In(l.right):
			a.panelScroll -= n
		}
	}

	// Space+drag pans.
	if ebiten.IsKeyPressed(ebiten.KeySpace) && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		a.panning, a.panFrom, a.spacePanned = true, [2]int{cx, cy}, true
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
	for i, b := range mouseButtons {
		switch {
		case inpututil.IsMouseButtonJustPressed(b) && !inCanvas:
			a.clickChrome(l, cur, b)
		case inpututil.IsMouseButtonJustPressed(b):
			a.pressed[i] = true
			a.ed.PointerDown(p, editor.Button(i), m)
		case a.pressed[i] && inpututil.IsMouseButtonJustReleased(b):
			a.pressed[i] = false
			a.ed.PointerUp(p, editor.Button(i))
		}
	}
	if (p != a.lastWorld && inCanvas) || a.pressed[0] {
		a.lastWorld = p
		a.ed.PointerMove(p, m)
	}
}

func (a *app) Draw(screen *ebiten.Image) {
	screen.Fill(background)
	l := a.computeLayout()
	nl := a.ed.Netlist()
	a.canvas.sync(a.ed)
	_, hoverNet, hoverText := a.ed.Hover()
	if cx, cy := ebiten.CursorPosition(); !image.Pt(cx, cy).In(l.canvas) {
		hoverNet, hoverText = netlist.NoNet, ""
	}
	canvasImg := screen.SubImage(l.canvas).(*ebiten.Image)
	a.canvas.draw(canvasImg, l.canvas, &a.view, hoverNet, a.ed.Mode() == editor.SimulateMode)
	if a.ed.Stale() {
		cells, value := a.ed.Stroke()
		drawStroke(canvasImg, &a.view, cells, value)
	}
	a.drawLabels(canvasImg)
	a.drawDiagnosticMarkers(canvasImg, nl)
	a.drawOverlay(canvasImg, a.ed.Overlay())
	a.drawChrome(screen, l)
	a.drawPanel(screen, l)
	a.drawStatus(screen, l, hoverText)
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

// drawStatus writes the two status lines: what the mouse and modifiers do
// now (or the prompt being typed), then position, counts and messages.
func (a *app) drawStatus(screen *ebiten.Image, l layout, hover string) {
	fillRect(screen, l.bottom, chromeBg)
	fillRect(screen, image.Rect(0, l.bottom.Min.Y, a.w, l.bottom.Min.Y+1), chromeLine)

	cx, cy := ebiten.CursorPosition()
	hint := a.buttonHint(l, image.Pt(cx, cy))
	if hint == "" {
		hint = a.ed.Hint()
	}
	if p := a.prompt; p != nil {
		label := map[uiAction]string{uiOpen: "open: ", uiSaveAs: "save as: ", uiSetWatch: "set " + p.target + " = "}[p.kind]
		hint = label + p.buffer + "_   (enter to confirm, esc to cancel)"
	}

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
	info := fmt.Sprintf("%d nets, %d transistors · %d errors, %d warnings · zoom %s (f fits)",
		len(nl.Nets), len(nl.Transistors), errs, warns, zoom)
	if hover != "" {
		info = hover + " · " + info
	}
	if a.ed.Status != "" {
		info = a.ed.Status + "   |   " + info
	}
	y := float64(l.bottom.Min.Y) + 5*a.scale
	a.drawText(screen, hint, 8*a.scale, y, 14, chromeText)
	a.drawText(screen, info, 8*a.scale, y+a.lineHeight(), 13, chromeDim)
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
