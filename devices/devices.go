// Package devices defines the Device interface and the devices that read and
// pin named buses between settles, starting with memory. Devices see the
// circuit only through the Bus interface, by net name, so this package
// imports no other package of the module. See docs/SPEC.md, "Memory devices
// and buses".
package devices

import (
	"encoding/json"
	"fmt"
)

// Level is the value of a net as a device sees it.
type Level uint8

const (
	Floating Level = iota
	High
	Low
	Unstable
)

func (l Level) String() string {
	return [...]string{"floating", "high", "low", "unstable"}[l]
}

// Bus is a device's view of the circuit. Pins are per device: the runner
// combines them with the user's pins and other devices' pins.
type Bus interface {
	Has(name string) bool
	Value(name string) Level
	Pin(name string, l Level) // High or Low
	Unpin(name string)
	Pinned(name string) Level // this device's own pin, Floating if none
}

// Device reads and pins named nets between settles.
type Device interface {
	Name() string
	Kind() string
	// Attach checks that every net the device uses exists.
	Attach(bus Bus) error
	// Service is called after the circuit settles. It reads nets and pins or
	// unpins them, and reports whether it changed any pin (so the runner
	// settles again). An error stops the run.
	Service(bus Bus) (changed bool, err error)
}

// ReadFile loads a file a device configuration refers to (memory init data),
// relative to the document. It is supplied by the caller, which owns file
// access.
type ReadFile func(name string) ([]byte, error)

// Parse builds a device from a document's device configuration: its kind
// and the whole JSON object.
func Parse(kind string, raw json.RawMessage, read ReadFile) (Device, error) {
	switch kind {
	case "memory":
		return parseMemory(raw, read)
	}
	return nil, fmt.Errorf("unknown device kind %q", kind)
}
