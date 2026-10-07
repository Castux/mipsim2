// Package ui is the Ebitengine front end: game loop, shader canvas, panels and
// input mapping. It is the only package (with cmd) that imports graphics.
package ui

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
)

// Options configures Run.
type Options struct {
	Title string
	// Screenshot, if set, saves the screen to this PNG path after a few
	// frames and quits. Used to check rendering without a person watching.
	Screenshot string
}

var background = color.RGBA{24, 24, 28, 255}

type app struct {
	opts   Options
	frames int
	err    error
}

func (a *app) Update() error {
	if a.err != nil {
		return a.err
	}
	return nil
}

func (a *app) Draw(screen *ebiten.Image) {
	screen.Fill(background)

	a.frames++
	if a.opts.Screenshot != "" && a.frames == 5 {
		if err := savePNG(screen, a.opts.Screenshot); err != nil {
			a.err = err
			return
		}
		a.err = ebiten.Termination
	}
}

func (a *app) Layout(w, h int) (int, int) { return w, h }

// Run opens the editor window and blocks until it is closed.
func Run(opts Options) error {
	if opts.Title == "" {
		opts.Title = "MiPSim"
	}
	ebiten.SetWindowSize(1280, 800)
	ebiten.SetWindowTitle(opts.Title)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	err := ebiten.RunGame(&app{opts: opts})
	if err == ebiten.Termination {
		return nil
	}
	return err
}

func savePNG(img *ebiten.Image, path string) error {
	rgba := image.NewRGBA(img.Bounds())
	img.ReadPixels(rgba.Pix)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, rgba); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("save screenshot: %w", err)
	}
	return nil
}
