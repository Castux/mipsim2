package doc

import (
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

		if !isRoot {
			if def.W <= 0 || def.H <= 0 {
				add(where, "size %dx%d is not positive", def.W, def.H)
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
		for i, inst := range def.Instances {
			iw := fmt.Sprintf("%s, instance %q", where, inst.Name)
			if inst.ID == "" {
				add(iw, "empty ID")
			} else if seenID[inst.ID] {
				add(iw, "duplicate ID %q", inst.ID)
			}
			seenID[inst.ID] = true
			if !ValidName(inst.Name) {
				add(iw, "invalid name (use letters, digits and underscores)")
			} else if seenName[inst.Name] {
				add(iw, "duplicate name among siblings")
			}
			seenName[inst.Name] = true
			if inst.Orient.Rot > 3 {
				add(iw, "rotation %d out of range", inst.Orient.Rot)
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
					add(iw, "overlaps sibling instance %q", def.Instances[j].Name)
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
	}

	if len(ps) == 0 {
		return nil
	}
	return ps
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
