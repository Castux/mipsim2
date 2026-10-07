package editor

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	"github.com/Castux/mipsim2/devices"
	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/runner"
)

// Device editing: the document's device list is edited from the Devices tab
// as undoable commands, and each device is checked against the circuit.

// deviceEdit replaces the document's device list.
type deviceEdit struct {
	name          string
	before, after []doc.DeviceConfig
}

func (c *deviceEdit) Name() string         { return c.name }
func (c *deviceEdit) Do(d *doc.Document)   { d.Devices = cloneDevices(c.after) }
func (c *deviceEdit) Undo(d *doc.Document) { d.Devices = cloneDevices(c.before) }

func cloneDevices(ds []doc.DeviceConfig) []doc.DeviceConfig {
	out := make([]doc.DeviceConfig, len(ds))
	for i, d := range ds {
		d.Raw = slices.Clone(d.Raw)
		out[i] = d
	}
	return out
}

// setDevices records a new device list as one undoable command.
func (e *Editor) setDevices(name string, after []doc.DeviceConfig) {
	c := &deviceEdit{name: name, before: cloneDevices(e.Doc.Devices), after: cloneDevices(after)}
	c.Do(e.Doc)
	e.undo = append(e.undo, c)
	e.redo = nil
	e.Status = ""
	e.changed(name)
}

// DeviceInfo describes one configured device for the Devices tab.
type DeviceInfo struct {
	Kind, Name string
	Memory     *devices.MemoryConfig // nil unless a memory with a readable configuration
	Problem    string                // why it cannot run, or "" if it can
	Summary    string                // what it attaches to, when it can
}

// Devices describes the document's devices, checked against the circuit.
// The result is cached until the document or the circuit changes.
func (e *Editor) Devices() []DeviceInfo {
	key := [2]int{e.version, e.Compiles()}
	if e.devInfo != nil && e.devKey == key {
		return e.devInfo
	}
	nl := e.Netlist()
	infos := make([]DeviceInfo, len(e.Doc.Devices))
	seen := map[string]bool{}
	for i, dc := range e.Doc.Devices {
		info := DeviceInfo{Kind: dc.Kind, Name: dc.Name}
		if dc.Kind == "memory" {
			var mc devices.MemoryConfig
			if json.Unmarshal(dc.Raw, &mc) == nil {
				info.Memory = &mc
			}
		}
		// Read the init file separately, so a missing one does not hide
		// missing nets: every problem is listed.
		var problems []string
		read := func(name string) ([]byte, error) {
			var err error = errors.New("no file access")
			var b []byte
			if e.ReadFile != nil {
				b, err = e.ReadFile(name)
			}
			if errors.Is(err, fs.ErrNotExist) {
				problems = append(problems, fmt.Sprintf("init file %s not found", name))
			} else if err != nil {
				problems = append(problems, fmt.Sprintf("init file %s: %v", name, err))
			}
			return b, nil
		}
		prefix := fmt.Sprintf("%s %q: ", dc.Kind, dc.Name)
		dev, err := devices.Parse(dc.Kind, dc.Raw, read)
		switch {
		case err != nil:
			problems = append(problems, strings.TrimPrefix(err.Error(), prefix))
		case seen[dc.Name]:
			problems = append(problems, fmt.Sprintf("another device is named %q", dc.Name))
		default:
			if err := runner.Check(nl, dev); err != nil {
				problems = append(problems, strings.TrimPrefix(err.Error(), prefix))
			} else if m, ok := dev.(*devices.Memory); ok && len(problems) == 0 {
				c := m.Config()
				info.Summary = fmt.Sprintf("%s_0..%d, %s_0..%d, %s, %s", c.Addr, m.AddrBits()-1, c.Data, c.Width-1, c.Select, c.Write)
			}
		}
		info.Problem = strings.Join(problems, "; ")
		seen[dc.Name] = true
		infos[i] = info
	}
	e.devInfo, e.devKey = infos, key
	return infos
}

// AddMemory adds a 256-byte RAM on the usual bus names, with a free name.
func (e *Editor) AddMemory() {
	name := "ram"
	for i := 1; slices.ContainsFunc(e.Doc.Devices, func(d doc.DeviceConfig) bool { return d.Name == name }); i++ {
		name = fmt.Sprintf("ram%d", i)
	}
	c := devices.MemoryConfig{Kind: "memory", Name: name, Addr: "addr", Data: "data", Select: "sel", Write: "we", Words: 256, Width: 8}
	raw, _ := json.Marshal(c)
	e.setDevices("add memory "+name, append(cloneDevices(e.Doc.Devices), doc.DeviceConfig{Kind: "memory", Name: name, Raw: raw}))
}

// DeleteDevice removes the i-th device.
func (e *Editor) DeleteDevice(i int) {
	if i < 0 || i >= len(e.Doc.Devices) {
		return
	}
	name := e.Doc.Devices[i].Name
	e.setDevices("delete device "+name, slices.Delete(cloneDevices(e.Doc.Devices), i, i+1))
}

// MemoryFields are the editable fields of a memory, in display order.
var MemoryFields = []string{"name", "addr", "data", "select", "write", "words", "width", "readonly", "init"}

// SetMemoryField changes one field of the i-th device, a memory. Numbers
// accept 0x and 0b prefixes; readonly takes yes or no; an empty init means
// no init file. Invalid values are refused with a status message.
func (e *Editor) SetMemoryField(i int, field, value string) bool {
	if i < 0 || i >= len(e.Doc.Devices) || e.Doc.Devices[i].Kind != "memory" {
		return false
	}
	var c devices.MemoryConfig
	if err := json.Unmarshal(e.Doc.Devices[i].Raw, &c); err != nil {
		e.Status = "device " + e.Doc.Devices[i].Name + ": " + err.Error()
		return false
	}
	value = strings.TrimSpace(value)
	fail := func(format string, args ...any) bool {
		e.Status = fmt.Sprintf("%s: ", field) + fmt.Sprintf(format, args...)
		return false
	}
	num := func() (int, bool) {
		n, err := strconv.ParseInt(value, 0, 64)
		return int(n), err == nil
	}
	switch field {
	case "name", "addr", "data", "select", "write":
		if value == "" || strings.ContainsAny(value, " \t") {
			return fail("needs a name without spaces")
		}
		*map[string]*string{"name": &c.Name, "addr": &c.Addr, "data": &c.Data, "select": &c.Select, "write": &c.Write}[field] = value
		if field == "name" && slices.ContainsFunc(e.Doc.Devices, func(d doc.DeviceConfig) bool { return d.Name == value }) && value != e.Doc.Devices[i].Name {
			return fail("another device is named %q", value)
		}
	case "words":
		n, ok := num()
		if !ok || n < 1 || n > 1<<24 {
			return fail("a number from 1 to %d", 1<<24)
		}
		c.Words = n
	case "width":
		n, ok := num()
		if !ok || n < 1 || n > 64 {
			return fail("a number of bits from 1 to 64")
		}
		c.Width = n
	case "readonly":
		switch strings.ToLower(value) {
		case "yes", "true", "1":
			c.ReadOnly = true
		case "no", "false", "0":
			c.ReadOnly = false
		default:
			return fail("yes or no")
		}
	case "init":
		c.Init = value
	default:
		return fail("not a memory field")
	}
	raw, _ := json.Marshal(c)
	devs := cloneDevices(e.Doc.Devices)
	devs[i] = doc.DeviceConfig{Kind: "memory", Name: c.Name, Raw: raw}
	e.setDevices(fmt.Sprintf("set %s %s", c.Name, field), devs)
	return true
}

// ReloadInit reads a running memory's init file again and settles, for
// when the file was rebuilt (a new program for a ROM, say).
func (e *Editor) ReloadInit(name string) {
	if e.run == nil {
		return
	}
	for _, d := range e.run.Devices() {
		m, ok := d.(*devices.Memory)
		if !ok || m.Name() != name {
			continue
		}
		init := m.Config().Init
		if init == "" || e.ReadFile == nil {
			e.Status = name + " has no init file"
			return
		}
		b, err := e.ReadFile(init)
		if err != nil {
			e.Status = "reload " + name + ": " + err.Error()
			return
		}
		m.Load(b)
		if e.Settle() {
			e.Status = fmt.Sprintf("reloaded %s from %s (%d bytes)", name, init, len(b))
		}
		return
	}
}
