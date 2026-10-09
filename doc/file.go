package doc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"slices"
	"strings"
)

// FormatName and FormatVersion identify .mip files. Version 2 added the
// optional note, tests and checks. Version 3 made cells typed: rows use
// '#' wire, 'H' power, 'L' ground, 'T' transistor and 'B' bridge. Versions
// 1 and 2 were one-bit drawings in an older pattern language and are no
// longer read.
const (
	FormatName    = "mipsim"
	FormatVersion = 3
)

type fileDoc struct {
	Format  string             `json:"format"`
	Version int                `json:"version"`
	Note    string             `json:"note,omitempty"`
	Root    string             `json:"root"`
	Defs    map[string]fileDef `json:"defs"`
	Devices []json.RawMessage  `json:"devices,omitempty"`
	Tests   []Step             `json:"tests,omitempty"`
	Checks  *Checks            `json:"checks,omitempty"`
}

type fileDef struct {
	Name      string      `json:"name,omitempty"`
	W         int         `json:"w,omitempty"`
	H         int         `json:"h,omitempty"`
	Origin    *[2]int     `json:"origin,omitempty"`
	Rows      []string    `json:"rows,omitempty"`
	Labels    []fileLabel `json:"labels,omitempty"`
	Instances []fileInst  `json:"instances,omitempty"`
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
	Name   string `json:"name,omitempty"`
}

// Save encodes the document as a .mip file: indented JSON, deterministic,
// with one pixel row per line.
func (d *Document) Save() ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	f := fileDoc{
		Format: FormatName, Version: FormatVersion, Note: d.Note, Root: string(d.Root),
		Defs: map[string]fileDef{}, Tests: d.Tests, Checks: d.Checks,
	}
	for id, def := range d.Defs {
		fd := fileDef{}
		if def.Name != "" && def.Name != string(id) {
			fd.Name = def.Name
		}
		if id == d.Root {
			// The origin's x snaps to a multiple of 16, so drawing a little left
			// of the circuit does not rewrite every row in a diff. (A new row
			// above only adds a line, so y is not snapped.)
			r := def.Pixels.Bounds()
			r.Min.X = floorTo(r.Min.X, 16)
			fd.Origin = &[2]int{r.Min.X, r.Min.Y}
			fd.Rows = def.Pixels.EncodeRows(r)
		} else {
			fd.W, fd.H = def.W, def.H
			fd.Rows = def.Pixels.EncodeRows(def.Rect())
		}
		for _, l := range def.Labels {
			fd.Labels = append(fd.Labels, fileLabel(l))
		}
		for _, inst := range def.Instances {
			fi := fileInst{ID: string(inst.ID), Def: string(inst.Def), X: inst.X, Y: inst.Y, Name: inst.Name}
			if inst.Orient != Identity {
				fi.Orient = inst.Orient.String()
			}
			fd.Instances = append(fd.Instances, fi)
		}
		f.Defs[string(id)] = fd
	}
	for _, dc := range d.Devices {
		f.Devices = append(f.Devices, dc.Raw)
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(f); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Load decodes a .mip file and validates it. Errors name the definition and
// item at fault; the loader never repairs a file.
func Load(data []byte) (*Document, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")) // a BOM, as some Windows editors write
	if err := checkJSON(data); err != nil {
		return nil, fmt.Errorf("not a valid .mip file: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var f fileDoc
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("not a valid .mip file: %w", locate(data, err))
	}
	if f.Format != FormatName {
		return nil, fmt.Errorf("format is %q, want %q", f.Format, FormatName)
	}
	switch {
	case f.Version > FormatVersion:
		return nil, fmt.Errorf("file version %d is newer than this program supports (%d)", f.Version, FormatVersion)
	case f.Version < 1:
		return nil, fmt.Errorf("invalid file version %d", f.Version)
	case f.Version < 3:
		return nil, fmt.Errorf("file version %d uses the old one-bit pattern language, which is no longer read; convert it with the upgrade tool from commit 35584bd (go run ./internal/tools/upgrade FILE)", f.Version)
	}

	d := &Document{Root: DefID(f.Root), Defs: map[DefID]*Definition{}, Note: f.Note, Tests: f.Tests, Checks: f.Checks}
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
				add(fmt.Sprintf("%s, instance %q", where, fi.ID), "%v", err)
			}
			def.Instances = append(def.Instances, Instance{
				ID: InstID(fi.ID), Def: DefID(fi.Def), X: fi.X, Y: fi.Y, Orient: o, Name: fi.Name,
			})
		}
		d.Defs[def.ID] = def
	}

	for i, raw := range f.Devices {
		dc, err := NewDeviceConfig(raw)
		if err != nil {
			add(fmt.Sprintf("device %d", i), "%v", err)
			continue
		}
		d.Devices = append(d.Devices, dc)
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

func floorTo(v, n int) int {
	if v >= 0 {
		return v / n * n
	}
	return -((-v + n - 1) / n * n)
}

// checkJSON rejects what encoding/json would quietly accept in a .mip file:
// anything after the document, and duplicate keys in an object (the last
// would silently win).
func checkJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	type frame struct {
		object bool
		keys   map[string]bool
		key    bool // the next token in this object is a key
	}
	var stack []*frame
	for depth := 0; ; {
		tok, err := dec.Token()
		if err != nil {
			return locate(data, err)
		}
		if n := len(stack); n > 0 && stack[n-1].object {
			f := stack[n-1]
			if f.key {
				if k, ok := tok.(string); ok {
					if f.keys[k] {
						return fmt.Errorf("%s: duplicate key %q", position(data, dec.InputOffset()), k)
					}
					f.keys[k] = true
					f.key = false
					continue
				}
			} else {
				f.key = true // this token is the value
			}
		}
		switch tok {
		case json.Delim('{'):
			stack = append(stack, &frame{object: true, keys: map[string]bool{}, key: true})
			depth++
		case json.Delim('['):
			stack = append(stack, &frame{})
			depth++
		case json.Delim('}'), json.Delim(']'):
			stack = stack[:len(stack)-1]
			depth--
		}
		if depth == 0 {
			if _, err := dec.Token(); err != io.EOF {
				return fmt.Errorf("%s: unexpected data after the document", position(data, dec.InputOffset()))
			}
			return nil
		}
	}
}

// locate adds a line and column to JSON syntax and type errors.
func locate(data []byte, err error) error {
	var se *json.SyntaxError
	var te *json.UnmarshalTypeError
	switch {
	case errors.As(err, &se):
		return fmt.Errorf("%s: %w", position(data, se.Offset), err)
	case errors.As(err, &te):
		return fmt.Errorf("%s: %w", position(data, te.Offset), err)
	case err == io.EOF || err == io.ErrUnexpectedEOF:
		return errors.New("the file ends too early")
	}
	return err
}

func position(data []byte, offset int64) string {
	offset = min(max(offset, 0), int64(len(data)))
	before := data[:offset]
	line := bytes.Count(before, []byte("\n")) + 1
	col := int(offset) - bytes.LastIndexByte(before, '\n')
	return fmt.Sprintf("line %d, column %d", line, col)
}
