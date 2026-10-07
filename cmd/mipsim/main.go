// Command mipsim is the editor.
//
//	mipsim [file.mip]
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Castux/mipsim2/ui"
)

func main() {
	screenshot := flag.String("screenshot", "", "save a screenshot to this PNG file after a few frames and exit")
	flag.Parse()

	err := ui.Run(ui.Options{Screenshot: *screenshot})
	if err != nil {
		fmt.Fprintln(os.Stderr, "mipsim:", err)
		os.Exit(1)
	}
}
