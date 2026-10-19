package main

import (
	"encoding/json"
	"image"

	"github.com/Castux/mipsim2/doc"
)

// CPUDocument lays out the processor and adds its memories: the program
// ROM on the PC, and the data RAM on Y and the bus. prog is the ROM's init
// file, relative to the document.
func CPUDocument(prog string) (*doc.Document, *cpu, *Layout, error) {
	c := BuildCPU()
	l, err := c.Lay()
	if err != nil {
		return nil, nil, nil, err
	}
	d := l.Document(image.Point{})
	if _, err := c.Verify(d, l, image.Point{}); err != nil {
		return nil, nil, nil, err
	}
	for _, m := range []map[string]any{
		{"kind": "memory", "name": "rom", "addr": "pc", "data": "rom", "read": "rom_read", "write": "rom_write", "words": 65536, "width": 16, "init": prog, "readonly": true},
		{"kind": "memory", "name": "ram", "addr": "y", "data": "bus", "read": "mem_read", "write": "mem_write", "words": 65536, "width": 16},
	} {
		raw, err := json.Marshal(m)
		if err != nil {
			return nil, nil, nil, err
		}
		dc, err := doc.NewDeviceConfig(raw)
		if err != nil {
			return nil, nil, nil, err
		}
		d.Devices = append(d.Devices, dc)
	}
	return d, c, l, nil
}

// romImage encodes words as the ROM's init file: little-endian.
func romImage(words []uint16) []byte {
	b := make([]byte, 0, 2*len(words))
	for _, w := range words {
		b = append(b, byte(w), byte(w>>8))
	}
	return b
}

// CPUDocumentOnly is CPUDocument without the design and layout.
func CPUDocumentOnly(prog string) (*doc.Document, error) {
	d, _, _, err := CPUDocument(prog)
	return d, err
}
