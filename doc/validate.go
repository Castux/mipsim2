package doc

import (
	"bytes"
	"fmt"
	"image"
	"strings"
)

// Problem is one broken invariant, located by definition and item.
type Problem struct {
	Where string // e.g. `def "nand2", instance "g1"`
	Msg   string
}

func (p Problem) String() string { return p.Where + ": " + p.Msg }

// Problems is a list of broken invariants. It is an error when non-empty.
type Problems []Problem

func (ps Problems) Error() string {
	var b strings.Builder
	for i, p := range ps {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(p.String())
	}
	return b.String()
}

// ValidName reports whether s can be used as a label or instance name:
// non-empty ASCII letters, digits and underscores. Dots are reserved for
// hierarchical names.
func ValidName(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !(c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}

// Limits keep any document, however it was made, cheap enough to flatten,
// compile and save: a few kilobytes of JSON must not describe billions of
// pixels or instances.
const (
	MaxCoord             = 1 << 24 // |x| and |y| of any pixel, label or instance
	MaxSize              = 1 << 16 // a definition's width and height
	MaxExpandedInstances = 1 << 20 // instances in the flattened circuit
	MaxExpandedPixels    = 1 << 28 // on pixels in the flattened circuit
)

func inRange(p image.Point) bool {
	return p.X >= -MaxCoord && p.X <= MaxCoord && p.Y >= -MaxCoord && p.Y <= MaxCoord
}

// Validate checks every document invariant from the spec and returns nil or
// a Problems error listing all of them.
func (d *Document) Validate() error {
	var ps Problems
	add := func(where, format string, args ...any) {
		ps = append(ps, Problem{Where: where, Msg: fmt.Sprintf(format, args...)})
	}

	root := d.Defs[d.Root]
	if root == nil {
		add("document", "root definition %q does not exist", d.Root)
		return ps
	}

	ids := d.DefIDs()
	for _, id := range ids {
		def := d.Defs[id]
		where := fmt.Sprintf("def %q", id)
		if def.ID != id {
			add(where, "definition is stored under %q but has ID %q", id, def.ID)
		}
		if def.Pixels == nil {
			add(where, "no pixel bitmap")
			continue
		}
		isRoot := id == d.Root

		if b := def.Pixels.Bounds(); !b.Empty() && !(inRange(b.Min) && inRange(b.Max)) {
			add(where, "pixels beyond the coordinate limit of %d (bounds %v)", MaxCoord, b)
		}
		if !isRoot {
			if def.W <= 0 || def.H <= 0 {
				add(where, "size %dx%d is not positive", def.W, def.H)
			} else if def.W > MaxSize || def.H > MaxSize {
				add(where, "size %dx%d is above the limit of %d", def.W, def.H, MaxSize)
				continue // later checks build rectangles of this size
			}
			if b := def.Pixels.Bounds(); !b.Empty() && !b.In(def.Rect()) {
				add(where, "pixels extend outside the %dx%d rectangle (bounds %v)", def.W, def.H, b)
			}
		}

		labelAt := map[image.Point]string{}
		for _, l := range def.Labels {
			lw := fmt.Sprintf("%s, label %q at %d,%d", where, l.Name, l.X, l.Y)
			if !ValidName(l.Name) {
				add(lw, "invalid name (use letters, digits and underscores)")
			}
			if !inRange(l.Pos()) {
				add(lw, "beyond the coordinate limit of %d", MaxCoord)
			}
			if !isRoot && !l.Pos().In(def.Rect()) {
				add(lw, "outside the %dx%d rectangle", def.W, def.H)
			}
			if other, dup := labelAt[l.Pos()]; dup {
				add(lw, "same position as label %q", other)
			}
			labelAt[l.Pos()] = l.Name
		}

		seenName := map[string]bool{}
		seenID := map[InstID]bool{}
		var placed []image.Rectangle
		effective := d.InstanceNames(def)
		for i, inst := range def.Instances {
			iw := fmt.Sprintf("%s, instance %q", where, effective[i])
			if inst.ID == "" {
				add(iw, "empty ID")
			} else if seenID[inst.ID] {
				add(iw, "duplicate ID %q", inst.ID)
			}
			seenID[inst.ID] = true
			// Names are optional (empty: named after the component), but an
			// explicit one must be valid, and effective names unique.
			if inst.Name != "" && !ValidName(inst.Name) {
				add(iw, "invalid name (use letters, digits and underscores)")
			} else if seenName[effective[i]] {
				add(iw, "duplicate name among siblings")
			}
			seenName[effective[i]] = true
			if inst.Orient.Rot > 3 {
				add(iw, "rotation %d out of range", inst.Orient.Rot)
			}
			if !inRange(image.Pt(inst.X, inst.Y)) {
				add(iw, "position %d,%d beyond the coordinate limit of %d", inst.X, inst.Y, MaxCoord)
				placed = append(placed, image.Rectangle{})
				continue
			}

			child := d.Defs[inst.Def]
			if child == nil {
				add(iw, "definition %q does not exist", inst.Def)
				placed = append(placed, image.Rectangle{})
				continue
			}
			if inst.Def == d.Root {
				add(iw, "the root definition cannot be instanced")
			}
			r := d.PlacedRect(inst)
			placed = append(placed, r)
			if !isRoot && !r.In(def.Rect()) {
				add(iw, "rectangle %v is not inside the parent's %dx%d rectangle", r, def.W, def.H)
			}
			for j := range i {
				if placed[j].Overlaps(r) {
					add(iw, "overlaps sibling instance %q", effective[j])
				}
			}
			if def.Pixels.AnyIn(r) {
				add(iw, "parent has pixels inside the instance rectangle %v", r)
			}
			for _, l := range def.Labels {
				if l.Pos().In(r) {
					add(iw, "parent label %q at %d,%d is inside the instance rectangle", l.Name, l.X, l.Y)
				}
			}
		}
	}

	if cycle := d.findCycle(ids); cycle != nil {
		parts := make([]string, len(cycle))
		for i, id := range cycle {
			parts[i] = string(id)
		}
		add("document", "definitions contain themselves: %s", strings.Join(parts, " -> "))
	} else if len(ps) == 0 {
		insts, pixels := d.expandedSize()
		if insts > MaxExpandedInstances {
			add("document", "the flattened circuit has over %d instances", MaxExpandedInstances)
		}
		if pixels > MaxExpandedPixels {
			add("document", "the flattened circuit has over %d pixels", MaxExpandedPixels)
		}
	}

	seenDev := map[string]bool{}
	for i, dc := range d.Devices {
		where := fmt.Sprintf("device %d (%q)", i, dc.Name)
		check, err := NewDeviceConfig(dc.Raw)
		switch {
		case err != nil:
			add(where, "%v", err)
		case check.Kind != dc.Kind || check.Name != dc.Name:
			add(where, "kind and name %q %q do not match the configuration (%q %q)", dc.Kind, dc.Name, check.Kind, check.Name)
		case !ValidName(dc.Name):
			add(where, "invalid name (use letters, digits and underscores)")
		case seenDev[dc.Name]:
			add(where, "another device has the same name")
		case !bytes.Equal(check.Raw, dc.Raw):
			add(where, "configuration is not compact JSON (build it with NewDeviceConfig)")
		}
		seenDev[dc.Name] = true
	}

	if len(ps) == 0 {
		return nil
	}
	return ps
}

// expandedSize counts the instances and on pixels of the flattened root,
// saturating just above the limits so huge fan-outs cannot overflow. The
// instance graph must be acyclic.
func (d *Document) expandedSize() (insts, pixels int) {
	type size struct{ insts, pixels int }
	memo := map[DefID]size{}
	var visit func(id DefID) size
	visit = func(id DefID) size {
		if s, ok := memo[id]; ok {
			return s
		}
		def := d.Defs[id]
		s := size{pixels: def.Pixels.Count()}
		for _, inst := range def.Instances {
			c := visit(inst.Def)
			s.insts = min(s.insts+1+c.insts, MaxExpandedInstances+1)
			s.pixels = min(s.pixels+c.pixels, MaxExpandedPixels+1)
		}
		memo[id] = s
		return s
	}
	s := visit(d.Root)
	return s.insts, s.pixels
}

// findCycle returns a cycle of definition IDs if the instance graph has one.
func (d *Document) findCycle(ids []DefID) []DefID {
	const (
		unseen = iota
		active
		done
	)
	state := map[DefID]int{}
	var stack []DefID
	var visit func(id DefID) []DefID
	visit = func(id DefID) []DefID {
		switch state[id] {
		case active:
			i := len(stack) - 1
			for stack[i] != id {
				i--
			}
			return append(append([]DefID{}, stack[i:]...), id)
		case done:
			return nil
		}
		def := d.Defs[id]
		if def == nil {
			return nil
		}
		state[id] = active
		stack = append(stack, id)
		for _, inst := range def.Instances {
			if c := visit(inst.Def); c != nil {
				return c
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = done
		return nil
	}
	for _, id := range ids {
		if c := visit(id); c != nil {
			return c
		}
	}
	return nil
}
