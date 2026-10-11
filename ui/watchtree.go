package ui

import (
	"slices"
	"strconv"
	"strings"
)

// The Watch tab shows names as a tree: dotted hierarchical names fold under
// their instance path (alu.add3.sum under alu, then add3), and a bus's bits
// fold under the bus's own row. Only the rows of expanded nodes are listed,
// so a large circuit costs what is on screen.

// watchNode is one row of the Watch tree.
type watchNode struct {
	label    string // what the row shows: the last path segment
	name     string // the net or bus it watches; empty for an instance group
	key      string // expansion key: the name, or the path with a trailing dot for a group
	children []*watchNode
}

// watchRow is a visible row: a node and its depth for indentation.
type watchRow struct {
	node  *watchNode
	depth int
}

// buildWatchTree arranges names (every net and bus name) as a tree.
// busWidth returns the number of bits of a bus row, or 0 for anything else;
// the bits present among names (name_0, name_1, ...) become the bus's
// children instead of rows of their own.
func buildWatchTree(names []string, busWidth func(string) int) []*watchNode {
	present := make(map[string]bool, len(names))
	for _, n := range names {
		present[n] = true
	}
	bitOf := map[string]bool{}
	for _, n := range names {
		for i := range busWidth(n) {
			if b := n + "_" + strconv.Itoa(i); present[b] {
				bitOf[b] = true
			}
		}
	}

	root := &watchNode{}
	groups := map[string]*watchNode{"": root}
	var group func(path string) *watchNode
	group = func(path string) *watchNode {
		if g, ok := groups[path]; ok {
			return g
		}
		parent, label := splitPath(path)
		g := &watchNode{label: label, key: path + "."}
		p := group(parent)
		p.children = append(p.children, g)
		groups[path] = g
		return g
	}
	leaves := map[string]*watchNode{}
	for _, n := range names {
		if bitOf[n] {
			continue
		}
		parent, label := splitPath(n)
		leaf := &watchNode{label: label, name: n, key: n}
		leaves[n] = leaf
		g := group(parent)
		g.children = append(g.children, leaf)
	}
	for _, n := range names {
		bus, ok := leaves[n]
		if !ok {
			continue
		}
		for i := range busWidth(n) {
			if b := n + "_" + strconv.Itoa(i); bitOf[b] {
				_, label := splitPath(b)
				bus.children = append(bus.children, &watchNode{label: label, name: b, key: b})
			}
		}
	}
	sortWatch(root)
	return root.children
}

// splitPath splits a hierarchical name into its instance path and last
// segment: "alu.add3.sum" is "alu.add3" and "sum".
func splitPath(name string) (path, label string) {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[:i], name[i+1:]
	}
	return "", name
}

// sortWatch orders siblings: watched names before instance groups, each in
// natural order (fa2 before fa10). A bus's bits keep their bit order.
func sortWatch(n *watchNode) {
	if n.name == "" {
		slices.SortStableFunc(n.children, func(a, b *watchNode) int {
			if ag, bg := a.name == "", b.name == ""; ag != bg {
				if bg {
					return -1
				}
				return 1
			}
			return naturalCompare(a.label, b.label)
		})
	}
	for _, c := range n.children {
		sortWatch(c)
	}
}

// naturalCompare compares strings with runs of digits compared as numbers.
func naturalCompare(a, b string) int {
	for a != "" && b != "" {
		da, db := digitRun(a), digitRun(b)
		if da > 0 && db > 0 {
			na, nb := strings.TrimLeft(a[:da], "0"), strings.TrimLeft(b[:db], "0")
			if c := len(na) - len(nb); c != 0 {
				return c
			}
			if c := strings.Compare(na, nb); c != 0 {
				return c
			}
			if c := da - db; c != 0 {
				return c
			}
			a, b = a[da:], b[db:]
			continue
		}
		if a[0] != b[0] {
			return int(a[0]) - int(b[0])
		}
		a, b = a[1:], b[1:]
	}
	return len(a) - len(b)
}

func digitRun(s string) int {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return i
}

// visibleWatchRows lists the rows shown: every top-level node, and the
// children of expanded nodes.
func visibleWatchRows(roots []*watchNode, expanded map[string]bool) []watchRow {
	var rows []watchRow
	var walk func(ns []*watchNode, depth int)
	walk = func(ns []*watchNode, depth int) {
		for _, n := range ns {
			rows = append(rows, watchRow{n, depth})
			if expanded[n.key] {
				walk(n.children, depth+1)
			}
		}
	}
	walk(roots, 0)
	return rows
}

// setExpanded expands or collapses n and, if deep, everything below it.
func setExpanded(expanded map[string]bool, n *watchNode, open, deep bool) {
	if len(n.children) == 0 {
		return
	}
	if open {
		expanded[n.key] = true
	} else {
		delete(expanded, n.key)
	}
	if deep {
		for _, c := range n.children {
			setExpanded(expanded, c, open, true)
		}
	}
}
