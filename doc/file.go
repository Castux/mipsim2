package doc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"slices"
	"strings"
)

// FormatName and FormatVersion identify .mip files.
const (
	FormatName    = "mipsim"
	FormatVersion = 1
)

type fileDoc struct {
	Format  string             `json:"format"`
	Version int                `json:"version"`
	Root    string             `json:"root"`
	Defs    map[string]fileDef `json:"defs"`
	Devices []json.RawMessage  `json:"devices,omitempty"`
}

type fileDef struct {
	Name      string      `json:"name,omitempty"`
	W         int         `json:"w,omitempty"`
	H         int         `json:"h,omitempty"`
	Origin    *[2]int     `json:"origin,omitempty"`
	Rows      []string    `json:"rows"`
	Labels    []fileLabel `json:"labels"`
	Instances []fileInst  `json:"instances"`
}

type fileLabel struct {
	X    int    `json:"x"`
	Y    int    `json:"y"`
	Name string `json:"name"`
}

type fileInst struct {
	ID     string `json:"id"`
	Def    string `json:"def"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Orient string `json:"orient,omitempty"`
	Name   string `json:"name"`
}

// Save encodes the document as a .mip file. The output is deterministic and
// laid out for readable diffs: one pixel row, label or instance per line, the
// root definition first and the others sorted by ID.
func (d *Document) Save() ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	str := func(s string) string { j, _ := json.Marshal(s); return string(j) }
	compact := func(v any) string { j, _ := json.Marshal(v); return string(j) }

	fmt.Fprintf(&b, "{\n  \"format\": %s,\n  \"version\": %d,\n  \"root\": %s,\n  \"defs\": {\n",
		str(FormatName), FormatVersion, str(string(d.Root)))

	ids := d.DefIDs()
	ids = slices.DeleteFunc(ids, func(id DefID) bool { return id == d.Root })
	ids = append([]DefID{d.Root}, ids...)
	for i, id := range ids {
		def := d.Defs[id]
		fmt.Fprintf(&b, "    %s: {\n", str(string(id)))
		if def.Name != "" && def.Name != string(id) {
			fmt.Fprintf(&b, "      \"name\": %s,\n", str(def.Name))
		}
		var rows []string
		if id == d.Root {
			r := def.Pixels.Bounds()
			fmt.Fprintf(&b, "      \"origin\": [%d, %d],\n", r.Min.X, r.Min.Y)
			rows = def.Pixels.EncodeRows(r)
		} else {
			fmt.Fprintf(&b, "      \"w\": %d, \"h\": %d,\n", def.W, def.H)
			rows = def.Pixels.EncodeRows(def.Rect())
		}

		writeList(&b, "rows", len(rows), func(i int) string { return str(rows[i]) })
		b.WriteString(",\n")
		writeList(&b, "labels", len(def.Labels), func(i int) string {
			l := def.Labels[i]
			return compact(fileLabel(l))
		})
		b.WriteString(",\n")
		writeList(&b, "instances", len(def.Instances), func(i int) string {
			inst := def.Instances[i]
			fi := fileInst{ID: string(inst.ID), Def: string(inst.Def), X: inst.X, Y: inst.Y, Name: inst.Name}
			if inst.Orient != Identity {
				fi.Orient = inst.Orient.String()
			}
			return compact(fi)
		})
		b.WriteString("\n    }")
		if i < len(ids)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("  }")

	if len(d.Devices) > 0 {
		b.WriteString(",\n")
		writeList(&b, "devices", len(d.Devices), func(i int) string {
			var c bytes.Buffer
			if err := json.Compact(&c, d.Devices[i].Raw); err != nil {
				return "{}"
			}
			return c.String()
		})
	}
	b.WriteString("\n}\n")
	return b.Bytes(), nil
}

// writeList writes `"name": [` and one item per line, indented to match Save.
func writeList(b *bytes.Buffer, name string, n int, item func(int) string) {
	indent := "      "
	if name == "devices" {
		indent = "  "
	}
	if n == 0 {
		fmt.Fprintf(b, "%s%q: []", indent, name)
		return
	}
	fmt.Fprintf(b, "%s%q: [\n", indent, name)
	for i := range n {
		b.WriteString(indent + "  " + item(i))
		if i < n-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString(indent + "]")
}

// Load decodes a .mip file and validates it. Errors name the definition and
// item at fault; the loader never repairs a file.
func Load(data []byte) (*Document, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var f fileDoc
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("not a valid .mip file: %w", err)
	}
	if f.Format != FormatName {
		return nil, fmt.Errorf("format is %q, want %q", f.Format, FormatName)
	}
	switch {
	case f.Version > FormatVersion:
		return nil, fmt.Errorf("file version %d is newer than this program supports (%d)", f.Version, FormatVersion)
	case f.Version < 1:
		return nil, fmt.Errorf("invalid file version %d", f.Version)
	}
	// Future versions: migrate f from f.Version up to FormatVersion here, one
	// tested function per step.

	d := &Document{Root: DefID(f.Root), Defs: map[DefID]*Definition{}}
	var ps Problems
	add := func(where, format string, args ...any) {
		ps = append(ps, Problem{Where: where, Msg: fmt.Sprintf(format, args...)})
	}

	keys := make([]string, 0, len(f.Defs))
	for k := range f.Defs {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		fd := f.Defs[k]
		where := fmt.Sprintf("def %q", k)
		def := NewDefinition(DefID(k), fd.W, fd.H)
		if fd.Name != "" {
			def.Name = fd.Name
		}
		var origin image.Point
		if DefID(k) == d.Root {
			if fd.W != 0 || fd.H != 0 {
				add(where, "the root definition has no size; remove w and h")
			}
			if fd.Origin != nil {
				origin = image.Pt(fd.Origin[0], fd.Origin[1])
			}
		} else {
			if fd.Origin != nil {
				add(where, "only the root definition has an origin")
			}
			if len(fd.Rows) > fd.H {
				add(where, "%d rows for height %d", len(fd.Rows), fd.H)
			}
			for y, row := range fd.Rows {
				if len(row) > fd.W {
					add(fmt.Sprintf("%s, row %d", where, y), "%d characters for width %d", len(row), fd.W)
				}
			}
		}
		if err := def.Pixels.DecodeRows(fd.Rows, origin); err != nil {
			add(where, "%v", err)
		}
		for _, l := range fd.Labels {
			def.Labels = append(def.Labels, Label(l))
		}
		for _, fi := range fd.Instances {
			o, err := ParseOrient(fi.Orient)
			if err != nil {
				add(fmt.Sprintf("%s, instance %q", where, fi.Name), "%v", err)
			}
			def.Instances = append(def.Instances, Instance{
				ID: InstID(fi.ID), Def: DefID(fi.Def), X: fi.X, Y: fi.Y, Orient: o, Name: fi.Name,
			})
		}
		d.Defs[def.ID] = def
	}

	for i, raw := range f.Devices {
		var head struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &head); err != nil || head.Kind == "" {
			add(fmt.Sprintf("device %d", i), "needs a \"kind\"")
			continue
		}
		d.Devices = append(d.Devices, DeviceConfig{Kind: head.Kind, Name: head.Name, Raw: raw})
	}

	if len(ps) > 0 {
		return nil, ps
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return d, nil
}

// LoadString is Load for literals in tests.
func LoadString(s string) (*Document, error) { return Load([]byte(strings.TrimSpace(s))) }
