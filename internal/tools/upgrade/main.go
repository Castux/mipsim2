// Command upgrade rewrites .mip files in the current format version. Files
// from versions 1 and 2 (one-bit drawings in the old pattern language) are
// migrated to typed cells on load (doc.MigratePatterns).
//
//	go run ./internal/tools/upgrade file.mip...
package main

import (
	"fmt"
	"os"

	"github.com/Castux/mipsim2/internal/mipfile"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: upgrade file.mip...")
		os.Exit(2)
	}
	status := 0
	for _, path := range os.Args[1:] {
		if err := upgrade(path); err != nil {
			fmt.Fprintln(os.Stderr, err)
			status = 1
		}
	}
	os.Exit(status)
}

func upgrade(path string) error {
	d, err := mipfile.Load(path)
	if err != nil {
		return err
	}
	data, err := d.Save()
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return os.WriteFile(path, data, 0o644)
}
