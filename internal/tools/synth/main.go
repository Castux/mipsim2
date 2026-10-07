// Command synth writes a synthetic inverter-chain circuit as a .mip file, for
// benchmarks and rendering checks:
//
//	go run ./internal/tools/synth -rows 152 -cols 152 -o chains.mip
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Castux/mipsim2/internal/synth"
)

func main() {
	rows := flag.Int("rows", 152, "number of chains")
	cols := flag.Int("cols", 152, "inverters per chain")
	out := flag.String("o", "chains.mip", "output file")
	flag.Parse()
	data, err := synth.InverterChains(*rows, *cols).Save()
	if err == nil {
		err = os.WriteFile(*out, data, 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "synth:", err)
		os.Exit(1)
	}
}
