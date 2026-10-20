package main

import (
	"encoding/json"

	"github.com/Castux/mipsim2/doc"
)

// ProcessorDocument lays out the processor and adds its memories: the
// program memory on the program counter, and the data memory on Y and the
// bus. program is the program memory's init file, relative to the
// document.
func ProcessorDocument(program string) (*doc.Document, *processor, *Layout, error) {
	p := BuildProcessor()
	layout, err := p.LayOut()
	if err != nil {
		return nil, nil, nil, err
	}
	d := layout.Document()
	if _, err := p.Verify(d, layout); err != nil {
		return nil, nil, nil, err
	}
	for _, m := range []map[string]any{
		{"kind": "memory", "name": "program", "addr": "program_counter", "data": "instruction_from_memory", "read": "program_memory_read", "write": "program_memory_write", "words": 65536, "width": 16, "init": program, "readonly": true},
		{"kind": "memory", "name": "data", "addr": "register_y", "data": "bus", "read": "memory_read", "write": "memory_write", "words": 65536, "width": 16},
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
	return d, p, layout, nil
}

// programImage encodes words as the program memory's init file:
// little-endian.
func programImage(words []uint16) []byte {
	b := make([]byte, 0, 2*len(words))
	for _, w := range words {
		b = append(b, byte(w), byte(w>>8))
	}
	return b
}

// processorDocumentOnly is ProcessorDocument without the design and
// layout.
func processorDocumentOnly(program string) (*doc.Document, error) {
	d, _, _, err := ProcessorDocument(program)
	return d, err
}
