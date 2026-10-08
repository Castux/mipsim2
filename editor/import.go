package editor

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Castux/mipsim2/doc"
)

// Importing components from another document: the user picks definitions,
// each comes with every definition it uses (at any depth), and all are
// copied in under fresh IDs as one undoable command. A display name already
// used in this document gets a suffix: fa becomes fa_2.

// ImportItem is a component of the source document, for the import dialog.
type ImportItem struct {
	ID   doc.DefID
	Name string
	W, H int
	Uses []doc.DefID // every definition it contains, at any depth, sorted
}

// ImportSelection is the import dialog's state: the components of a source
// document and which ones the user picked. A picked component forces the
// ones it uses, which show as checked but cannot be unchecked on their own.
type ImportSelection struct {
	Src    *doc.Document
	Items  []ImportItem // sorted by name
	picked map[doc.DefID]bool
}

// NewImportSelection lists a document's components (not its root).
func NewImportSelection(src *doc.Document) *ImportSelection {
	s := &ImportSelection{Src: src, picked: map[doc.DefID]bool{}}
	for _, id := range src.DefIDs() {
		if id == src.Root {
			continue
		}
		d := src.Defs[id]
		s.Items = append(s.Items, ImportItem{ID: id, Name: d.Name, W: d.W, H: d.H, Uses: uses(src, id)})
	}
	slices.SortStableFunc(s.Items, func(a, b ImportItem) int { return strings.Compare(a.Name, b.Name) })
	return s
}

// uses returns every definition that id contains, at any depth, sorted.
func uses(d *doc.Document, id doc.DefID) []doc.DefID {
	seen := map[doc.DefID]bool{}
	var out []doc.DefID
	var visit func(doc.DefID)
	visit = func(id doc.DefID) {
		for _, inst := range d.Defs[id].Instances {
			if !seen[inst.Def] {
				seen[inst.Def] = true
				out = append(out, inst.Def)
				visit(inst.Def)
			}
		}
	}
	visit(id)
	slices.Sort(out)
	return out
}

// Toggle picks or unpicks a component. Forced components cannot be
// unpicked while a picked component uses them.
func (s *ImportSelection) Toggle(id doc.DefID) {
	if s.Forced(id) {
		return
	}
	s.picked[id] = !s.picked[id]
}

// Checked reports whether a component will be imported.
func (s *ImportSelection) Checked(id doc.DefID) bool { return s.picked[id] || s.Forced(id) }

// Forced reports whether a component will be imported because a picked
// component uses it.
func (s *ImportSelection) Forced(id doc.DefID) bool {
	for _, it := range s.Items {
		if s.picked[it.ID] && slices.Contains(it.Uses, id) {
			return true
		}
	}
	return false
}

// Chosen returns every component to import, picked or forced, sorted by ID.
func (s *ImportSelection) Chosen() []doc.DefID {
	var out []doc.DefID
	for _, it := range s.Items {
		if s.Checked(it.ID) {
			out = append(out, it.ID)
		}
	}
	slices.Sort(out)
	return out
}

// Import copies the chosen definitions of src (which must include everything
// they use, as Chosen does) into the document, as one undoable command. It
// reports how many were imported and which were renamed.
func (e *Editor) Import(src *doc.Document, chosen []doc.DefID) bool {
	if len(chosen) == 0 {
		e.Status = "nothing to import"
		return false
	}
	// Fresh IDs, and display names made unique against the document and
	// against each other.
	newID := map[doc.DefID]doc.DefID{}
	taken := map[string]bool{}
	for _, id := range e.Doc.DefIDs() {
		taken[e.Doc.Defs[id].Name] = true
	}
	var ids []doc.DefID
	var renamed []string
	reserved := map[doc.DefID]bool{}
	for _, id := range chosen {
		n := e.freeDefIDExcept(reserved)
		reserved[n] = true
		newID[id] = n
		ids = append(ids, n)
	}
	names := map[doc.DefID]string{}
	for _, id := range chosen {
		name := src.Defs[id].Name
		if taken[name] {
			base := name
			for i := 2; taken[name]; i++ {
				name = fmt.Sprintf("%s_%d", base, i)
			}
			renamed = append(renamed, base+" as "+name)
		}
		taken[name] = true
		names[id] = name
	}

	ok := e.change(fmt.Sprintf("import %d components", len(chosen)), ids, func() error {
		for _, id := range chosen {
			s := src.Defs[id]
			if s == nil {
				return fmt.Errorf("component %q is not in the source document", id)
			}
			def := s.Clone()
			def.ID, def.Name = newID[id], names[id]
			for i := range def.Instances {
				n, ok := newID[def.Instances[i].Def]
				if !ok {
					return fmt.Errorf("component %q uses %q, which is not being imported", s.Name, src.Defs[def.Instances[i].Def].Name)
				}
				def.Instances[i].Def = n
			}
			e.Doc.Defs[def.ID] = def
		}
		return nil
	})
	if !ok {
		return false
	}
	e.Status = fmt.Sprintf("imported %d components", len(chosen))
	if len(renamed) > 0 {
		e.Status += " (renamed: " + strings.Join(renamed, ", ") + ")"
	}
	return true
}

// freeDefIDExcept is freeDefID, also skipping IDs reserved for definitions
// not yet added.
func (e *Editor) freeDefIDExcept(reserved map[doc.DefID]bool) doc.DefID {
	for n := 1; ; n++ {
		id := doc.DefID(fmt.Sprintf("part%d", n))
		if e.Doc.Defs[id] == nil && !reserved[id] {
			return id
		}
	}
}
