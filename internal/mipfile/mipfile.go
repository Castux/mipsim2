// Package mipfile reads .mip documents from disk, for the commands and tests.
// (The doc package itself does no file access.)
package mipfile

import (
	"fmt"
	"os"

	"github.com/Castux/mipsim2/doc"
)

// Load reads and decodes a .mip file. Errors name the file.
func Load(path string) (*doc.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	d, err := doc.Load(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return d, nil
}
