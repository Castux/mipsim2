package doc

import "fmt"

// Instance names are optional. An empty name means "named after its
// component": a lone unnamed instance of a component is called after it
// ("fa"), and several unnamed instances of one component among siblings are
// numbered from 0 in placement order ("fa0" ... "fa7"), with an underscore
// before the number when the component's name ends in a digit ("part1_0").
// Explicit names always win, and automatic numbers skip names taken by an
// explicit sibling. Hierarchical net names use these effective names.

// BaseName is the name derived from a definition for its unnamed instances:
// its display name (or ID) with every character that is not a letter, digit
// or underscore replaced by an underscore.
func (def *Definition) BaseName() string {
	s := def.Name
	if s == "" {
		s = string(def.ID)
	}
	b := []byte(s)
	for i, c := range b {
		if !(c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			b[i] = '_'
		}
	}
	if len(b) == 0 {
		return "part"
	}
	return string(b)
}

// InstanceNames returns the effective names of def's instances, in order.
func (d *Document) InstanceNames(def *Definition) []string {
	names := make([]string, len(def.Instances))
	taken := map[string]bool{}  // lookups only
	unnamed := map[string]int{} // base -> number of unnamed siblings using it
	bases := make([]string, len(def.Instances))
	for i, inst := range def.Instances {
		if inst.Name != "" {
			names[i] = inst.Name
			taken[inst.Name] = true
			continue
		}
		base := "part"
		if child := d.Defs[inst.Def]; child != nil {
			base = child.BaseName()
		}
		bases[i] = base
		unnamed[base]++
	}
	next := map[string]int{}
	for i, inst := range def.Instances {
		if inst.Name != "" {
			continue
		}
		base := bases[i]
		if unnamed[base] == 1 && !taken[base] {
			names[i] = base
			taken[base] = true
			continue
		}
		sep := ""
		if c := base[len(base)-1]; c >= '0' && c <= '9' {
			sep = "_"
		}
		for {
			n := fmt.Sprintf("%s%s%d", base, sep, next[base])
			next[base]++
			if !taken[n] {
				names[i] = n
				taken[n] = true
				break
			}
		}
	}
	return names
}

// InstanceName returns the effective name of def's i-th instance.
func (d *Document) InstanceName(def *Definition, i int) string {
	return d.InstanceNames(def)[i]
}
