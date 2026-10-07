package runner

import (
	"fmt"

	"github.com/Castux/mipsim2/devices"
	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/netlist"
	"github.com/Castux/mipsim2/sim"
)

// deviceBus is one device's view of the circuit: names to nets, and the
// device's own pin layer.
type deviceBus struct {
	r   *Runner
	idx int
	err error // a pin conflict found during the current service
}

func (b *deviceBus) id(name string) (netlist.NetID, bool) { return b.r.nl.Lookup(name) }

func (b *deviceBus) Has(name string) bool {
	_, ok := b.id(name)
	return ok
}

func (b *deviceBus) Value(name string) devices.Level {
	id, ok := b.id(name)
	if !ok {
		return devices.Floating
	}
	return toLevel(b.r.sim.Value(id))
}

func (b *deviceBus) Pin(name string, l devices.Level) {
	id, ok := b.id(name)
	if !ok {
		return
	}
	for j, pins := range b.r.devPins {
		if j != b.idx && pins[id] != sim.Floating && b.err == nil {
			b.err = fmt.Errorf("devices %q and %q both drive %s", b.r.devs[j].Name(), b.r.devs[b.idx].Name(), name)
		}
	}
	b.r.devPins[b.idx][id] = fromLevel(l)
	b.r.apply(id)
}

func (b *deviceBus) Unpin(name string) {
	if id, ok := b.id(name); ok {
		b.r.devPins[b.idx][id] = sim.Floating
		b.r.apply(id)
	}
}

func (b *deviceBus) Pinned(name string) devices.Level {
	if id, ok := b.id(name); ok {
		return toLevel(b.r.devPins[b.idx][id])
	}
	return devices.Floating
}

func toLevel(v sim.Value) devices.Level {
	return [...]devices.Level{devices.Floating, devices.High, devices.Low, devices.Unstable}[v]
}

func fromLevel(l devices.Level) sim.Value {
	return [...]sim.Value{sim.Floating, sim.High, sim.Low, sim.Unstable}[l]
}

// DevicesFromDoc builds the devices a document configures, in order. read
// loads files they refer to (memory init data), relative to the document.
func DevicesFromDoc(d *doc.Document, read devices.ReadFile) ([]devices.Device, error) {
	var out []devices.Device
	for _, c := range d.Devices {
		dev, err := devices.Parse(c.Kind, c.Raw, read)
		if err != nil {
			return nil, err
		}
		out = append(out, dev)
	}
	return out, nil
}

// namesBus answers only which nets exist, for checking a configuration
// against a circuit without simulating it.
type namesBus struct{ nl *netlist.Netlist }

func (b namesBus) Has(name string) bool {
	_, ok := b.nl.Lookup(name)
	return ok
}
func (namesBus) Value(string) devices.Level  { return devices.Floating }
func (namesBus) Pin(string, devices.Level)   {}
func (namesBus) Unpin(string)                {}
func (namesBus) Pinned(string) devices.Level { return devices.Floating }

// Check reports whether a device can attach to the circuit: the same check
// New makes, without building a simulation.
func Check(nl *netlist.Netlist, d devices.Device) error {
	return d.Attach(namesBus{nl})
}
