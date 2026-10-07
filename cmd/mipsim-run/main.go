// Command mipsim-run runs circuits headless.
//
//	mipsim-run file.mip --ticks N --watch a,b --set x=1 --trace
//
// Implemented in M4; until then it only reports that.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "mipsim-run: not implemented yet (M4)")
	os.Exit(2)
}
