package doc

import (
	"slices"
	"strings"
	"testing"
)

func namingDoc(names ...string) *Document {
	d := New()
	fa := NewDefinition("fa", 2, 2)
	fa.Labels = []Label{{X: 0, Y: 0, Name: "s"}}
	fa.Pixels.Set(0, 0, true)
	d.Defs["fa"] = fa
	p1 := NewDefinition("part1", 2, 2)
	d.Defs["p1"] = p1
	p1.ID = "p1"
	p1.Name = "part1"
	for i, n := range names {
		def := DefID("fa")
		if strings.HasPrefix(n, "p:") {
			def, n = "p1", strings.TrimPrefix(n, "p:")
		}
		d.RootDef().Instances = append(d.RootDef().Instances, Instance{ID: InstID("i" + string(rune('a'+i))), Def: def, X: 3 * i, Name: n})
	}
	return d
}

func TestInstanceNames(t *testing.T) {
	cases := []struct {
		names []string
		want  string
	}{
		{[]string{""}, "fa"},
		{[]string{"", ""}, "fa0 fa1"},
		{[]string{"", "x", ""}, "fa0 x fa1"},
		{[]string{"fa0", "", ""}, "fa0 fa1 fa2"},
		{[]string{"fa", ""}, "fa fa0"},
		{[]string{"p:", "p:"}, "part1_0 part1_1"},
		{[]string{"p:", ""}, "part1 fa"},
	}
	for _, c := range cases {
		d := namingDoc(c.names...)
		got := strings.Join(d.InstanceNames(d.RootDef()), " ")
		if got != c.want {
			t.Errorf("%q: names %q, want %q", c.names, got, c.want)
		}
		if err := d.Validate(); err != nil {
			t.Errorf("%q: %v", c.names, err)
		}
	}
}

func TestUnnamedInstancesInFlattenAndFiles(t *testing.T) {
	d := namingDoc("", "", "out")
	var labels []string
	for _, l := range d.Flatten().Labels {
		labels = append(labels, l.Name)
	}
	if want := []string{"fa0.s", "fa1.s", "out.s"}; !slices.Equal(labels, want) {
		t.Errorf("labels %v, want %v", labels, want)
	}
	data, err := d.Save()
	if err != nil {
		t.Fatal(err)
	}
	named := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, `"id":`) && strings.Contains(line, `"name":`) {
			named++
		}
	}
	if named != 1 {
		t.Errorf("empty names should be omitted from the file:\n%s", data)
	}
	back, err := Load(data)
	if err != nil || !back.Equal(d) {
		t.Errorf("round trip: %v", err)
	}
	// Explicit duplicates are still an error; a display name with spaces
	// becomes a valid base.
	d.RootDef().Instances[1].Name = "out" // two explicit "out"
	if err := d.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate name") {
		t.Errorf("duplicate explicit name: %v", err)
	}
	d.Defs["fa"].Name = "full adder"
	d.RootDef().Instances[1].Name = ""
	d.RootDef().Instances[2].Name = ""
	if got := strings.Join(d.InstanceNames(d.RootDef()), " "); got != "full_adder0 full_adder1 full_adder2" {
		t.Errorf("names from a display name with a space: %q", got)
	}
}
