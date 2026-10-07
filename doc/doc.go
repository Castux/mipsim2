// Package doc is the document model: definitions, instances and their
// orientations, labels, invariants, flattening to a world bitmap, and .mip
// load and save.
package doc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"slices"

	"github.com/Castux/mipsim2/bitmap"
)

// DefID identifies a definition within a document (the key in the file).
type DefID string

// InstID identifies an instance among its siblings.
type InstID string

// Label names the net under pixel (X, Y), in the definition's local coordinates.
type Label struct {
	X, Y int
	Name string
}

// Pos returns the label's position.
func (l Label) Pos() image.Point { return image.Pt(l.X, l.Y) }

// Instance is a placement of a definition inside a parent definition.
type Instance struct {
	ID     InstID
	Def    DefID
	X, Y   int    // top-left of the placed (oriented) rectangle, in the parent's coordinates
	Orient Orient // applied to the definition's rectangle before placing
	Name   string // optional; empty means named after its component (see InstanceNames)
}

// Definition is a reusable rectangle of pixels, labels and child instances.
// The root definition is unbounded: its W and H are ignored and its pixels
// are in world coordinates.
type Definition struct {
	ID        DefID
	Name      string // display name; defaults to the ID
	W, H      int    // pixels live in [0,W)x[0,H)
	Pixels    *bitmap.Bitmap
	Labels    []Label
	Instances []Instance
}

// DeviceConfig is one entry of the document's device list. The document only
// carries it; the runner interprets Raw by Kind. Kind and Name are copies of
// Raw's "kind" and "name" fields: build configs with NewDeviceConfig, and
// Validate checks they agree.
type DeviceConfig struct {
	Kind string
	Name string
	Raw  json.RawMessage // the whole JSON object, compact
}

// NewDeviceConfig reads a device's JSON object. It needs a "kind" and a
// "name" string; the other fields are the device's business.
func NewDeviceConfig(raw json.RawMessage) (DeviceConfig, error) {
	var head struct {
		Kind *string `json:"kind"`
		Name *string `json:"name"`
	}
	var c bytes.Buffer
	if err := json.Compact(&c, raw); err != nil {
		return DeviceConfig{}, fmt.Errorf("not a JSON object: %w", err)
	}
	if err := json.Unmarshal(c.Bytes(), &head); err != nil {
		return DeviceConfig{}, fmt.Errorf("not a device object: %w", err)
	}
	switch {
	case head.Kind == nil || *head.Kind == "":
		return DeviceConfig{}, fmt.Errorf("needs a \"kind\"")
	case head.Name == nil:
		return DeviceConfig{}, fmt.Errorf("needs a \"name\"")
	}
	return DeviceConfig{Kind: *head.Kind, Name: *head.Name, Raw: c.Bytes()}, nil
}

// Document is a tree of definitions rooted at Root.
type Document struct {
	Root    DefID
	Defs    map[DefID]*Definition
	Devices []DeviceConfig
}

// New returns a document with an empty root definition called "top".
func New() *Document {
	d := &Document{Root: "top", Defs: map[DefID]*Definition{}}
	d.Defs["top"] = NewDefinition("top", 0, 0)
	return d
}

// NewDefinition returns an empty definition.
func NewDefinition(id DefID, w, h int) *Definition {
	return &Definition{ID: id, Name: string(id), W: w, H: h, Pixels: bitmap.New()}
}

// RootDef returns the root definition.
func (d *Document) RootDef() *Definition { return d.Defs[d.Root] }

// DefIDs returns every definition ID in sorted order, for deterministic iteration.
func (d *Document) DefIDs() []DefID {
	ids := make([]DefID, 0, len(d.Defs))
	for id := range d.Defs {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// Rect returns the definition's rectangle in its local coordinates.
func (def *Definition) Rect() image.Rectangle { return image.Rect(0, 0, def.W, def.H) }

// Placement returns the map from the instance's local coordinates to its
// parent's coordinates.
func (d *Document) Placement(inst Instance) Affine {
	def := d.Defs[inst.Def]
	return inst.Orient.Affine(def.W, def.H).Then(Translate(inst.X, inst.Y))
}

// PlacedRect returns the instance's rectangle in its parent's coordinates.
func (d *Document) PlacedRect(inst Instance) image.Rectangle {
	def := d.Defs[inst.Def]
	w, h := inst.Orient.Size(def.W, def.H)
	return image.Rect(inst.X, inst.Y, inst.X+w, inst.Y+h)
}

// Clone returns a deep copy.
func (d *Document) Clone() *Document {
	n := &Document{Root: d.Root, Defs: make(map[DefID]*Definition, len(d.Defs))}
	for _, id := range d.DefIDs() {
		n.Defs[id] = d.Defs[id].Clone()
	}
	for _, dev := range d.Devices {
		dev.Raw = slices.Clone(dev.Raw)
		n.Devices = append(n.Devices, dev)
	}
	return n
}

// Clone returns a deep copy.
func (def *Definition) Clone() *Definition {
	n := *def
	n.Pixels = def.Pixels.Clone()
	n.Labels = slices.Clone(def.Labels)
	n.Instances = slices.Clone(def.Instances)
	return &n
}

// Equal reports whether two documents have the same content.
func (d *Document) Equal(o *Document) bool {
	if d.Root != o.Root || len(d.Defs) != len(o.Defs) || len(d.Devices) != len(o.Devices) {
		return false
	}
	for id, a := range d.Defs {
		b, ok := o.Defs[id]
		if !ok || !a.Equal(b) {
			return false
		}
	}
	for i := range d.Devices {
		a, b := d.Devices[i], o.Devices[i]
		if a.Kind != b.Kind || a.Name != b.Name || !jsonEqual(a.Raw, b.Raw) {
			return false
		}
	}
	return true
}

// Equal reports whether two definitions have the same content.
func (def *Definition) Equal(o *Definition) bool {
	return def.ID == o.ID && def.Name == o.Name && def.W == o.W && def.H == o.H &&
		def.Pixels.Equal(o.Pixels) &&
		slices.Equal(def.Labels, o.Labels) &&
		slices.Equal(def.Instances, o.Instances)
}

func jsonEqual(a, b json.RawMessage) bool {
	var x, y any
	decode := func(raw json.RawMessage, v *any) error {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber() // exact integers, not float64
		return dec.Decode(v)
	}
	if decode(a, &x) != nil || decode(b, &y) != nil {
		return string(a) == string(b)
	}
	xa, _ := json.Marshal(x)
	ya, _ := json.Marshal(y)
	return string(xa) == string(ya)
}
