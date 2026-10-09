// Command classify prints the role map, nets and diagnostics of a document.
// It is a development aid for drawing fixtures by hand:
//
//	go run ./internal/tools/classify testdata/sim/nand.mip
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Castux/mipsim2/internal/mipfile"
	"github.com/Castux/mipsim2/netlist"
)

func main() {
	isolated := flag.Bool("isolated", false, "use the isolated-sources variant")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: classify [-isolated] file.mip")
		os.Exit(2)
	}
	d, err := mipfile.Load(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	opts := netlist.Options{}
	if *isolated {
		opts.Variant = netlist.IsolatedSources
	}
	flat := d.Flatten()
	nl := netlist.Compile(flat, opts)
	r := flat.Pixels.Bounds().Inset(-1)

	fmt.Printf("origin %d,%d\n", r.Min.X, r.Min.Y)
	for i, row := range nl.RoleMap(r) {
		fmt.Printf("%3d %s\n", r.Min.Y+i, row)
	}
	fmt.Printf("%d nets, %d transistors\n", len(nl.Nets), len(nl.Transistors))
	for id, n := range nl.Nets {
		if len(n.Names) > 0 || n.Drive != netlist.DriveNone {
			fmt.Printf("  net %d at %d,%d drive %v names %s\n", id, n.First.X, n.First.Y, n.Drive, strings.Join(n.Names, " "))
		}
	}
	for i, t := range nl.Transistors {
		fmt.Printf("  transistor %d at %d,%d: gate %d, channel %d-%d\n", i, t.Pos.X, t.Pos.Y, t.Gate, t.A, t.B)
	}
	for _, d := range nl.Diagnostics {
		fmt.Println(d)
	}
}
