package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// dumpWatch renders rows as "depth label=name" lines, groups with a slash.
func dumpWatch(rows []watchRow) string {
	var b strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&b, "%s%s", strings.Repeat("  ", r.depth), r.node.label)
		if r.node.name == "" {
			b.WriteString("/")
		} else if r.node.name != r.node.label {
			b.WriteString("=" + r.node.name)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func TestWatchTree(t *testing.T) {
	names := []string{
		"clock", "a_0", "a_1", "a", "a_2_x",
		"alu.result_0", "alu.result_1", "alu.result", "alu.fa10.s", "alu.fa2.s", "alu.cout",
		"x_10", "x_9", "x_1",
	}
	buses := map[string]int{"a": 2, "alu.result": 2}
	roots := buildWatchTree(names, func(n string) int { return buses[n] })

	if got, want := dumpWatch(visibleWatchRows(roots, nil)), "a\na_2_x\nclock\nx_1\nx_9\nx_10\nalu/\n"; got != want {
		t.Errorf("folded:\n%s\nwant:\n%s", got, want)
	}

	expanded := map[string]bool{}
	for _, n := range roots {
		setExpanded(expanded, n, true, true)
	}
	want := `a
  a_0
  a_1
a_2_x
clock
x_1
x_9
x_10
alu/
  cout=alu.cout
  result=alu.result
    result_0=alu.result_0
    result_1=alu.result_1
  fa2/
    s=alu.fa2.s
  fa10/
    s=alu.fa10.s
`
	if got := dumpWatch(visibleWatchRows(roots, expanded)); got != want {
		t.Errorf("expanded:\n%s\nwant:\n%s", got, want)
	}

	// Folding alu hides everything below it but remembers the inner state.
	alu := roots[len(roots)-1]
	setExpanded(expanded, alu, false, false)
	if got := len(visibleWatchRows(roots, expanded)); got != 9 {
		t.Errorf("after folding alu: %d rows, want 9", got)
	}
	if !expanded["alu.fa2."] || !expanded["alu.result"] {
		t.Error("folding alu forgot its children's state")
	}
	setExpanded(expanded, alu, false, true)
	if len(expanded) != 1 || !expanded["a"] {
		t.Errorf("deep fold left %v", expanded)
	}
}

func TestNaturalCompare(t *testing.T) {
	s := []string{"fa10", "fa2", "fa", "b", "fa02", "fa1x", "fa1"}
	slices.SortFunc(s, naturalCompare)
	if want := []string{"b", "fa", "fa1", "fa1x", "fa2", "fa02", "fa10"}; !slices.Equal(s, want) {
		t.Errorf("got %v, want %v", s, want)
	}
}
