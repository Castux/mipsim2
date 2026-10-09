// Command mipsim is the editor.
//
//	mipsim [flags] [file.mip]
//
// The file is saved back in place with ctrl+s.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/internal/mipfile"
	"github.com/Castux/mipsim2/ui"
)

func main() {
	screenshot := flag.String("screenshot", "", "save a screenshot to this PNG after a few frames and exit")
	simulate := flag.Bool("simulate", false, "start in simulate mode")
	set := flag.String("set", "", "pins to apply in simulate mode: name=value,...")
	zoom := flag.Float64("zoom", 0, "fixed zoom, screen pixels per circuit pixel (0 fits the circuit)")
	filter := flag.Int("filter", 1, "zoomed-out filter: 0 average, 1 contrast boost, 2 any-on")
	frames := flag.Int("frames", 0, "with -screenshot: time this many frames without vsync first")
	tool := flag.String("tool", "", "start with this tool: draw, select or label")
	browse := flag.String("browse", "", "start with the file dialog open: open or save")
	click := flag.String("click", "", "for screenshots: click once at world pixel X,Y with the start tool")
	tab := flag.String("tab", "", "panel tab to show first: watch, memory, components, devices or diagnostics")
	flag.Parse()

	opts := ui.Options{Screenshot: *screenshot, Simulate: *simulate, Zoom: *zoom, Filter: *filter, Frames: *frames, Tool: *tool, Browse: *browse, Click: parseClick(*click), Tab: *tab}
	if *set != "" {
		opts.Sets = strings.Split(*set, ",")
	}
	if flag.NArg() > 0 {
		path := flag.Arg(0)
		d, savePath, err := open(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mipsim:", err)
			os.Exit(1)
		}
		opts.Doc, opts.Path = d, savePath
	}
	if err := ui.Run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "mipsim:", err)
		os.Exit(1)
	}
}

// open loads a .mip file. A missing file starts a new document that will be
// saved there.
func open(path string) (*doc.Document, string, error) {
	d, err := mipfile.Load(path)
	if errors.Is(err, fs.ErrNotExist) {
		return doc.New(), path, nil
	}
	return d, path, err
}

// parseClick parses "X,Y" for the -click flag.
func parseClick(s string) []int {
	var x, y int
	if _, err := fmt.Sscanf(s, "%d,%d", &x, &y); err != nil {
		return nil
	}
	return []int{x, y}
}
