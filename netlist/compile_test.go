package netlist

import (
	"flag"
	"fmt"
	"github.com/Castux/mipsim2/bitmap"
	"image"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/internal/fixture"
)

var update = flag.Bool("update", false, "rewrite classification goldens in testdata/netlist")

// Fixture directives understood here (besides the structural ones):
//
//	# diag CODE...           distinct diagnostic codes with thin wires ("none" for a clean circuit)
//	# diag-isolated CODE...  the same for the isolated-sources variant
//	# nets N                 number of nets (thin wires)
//	# transistors N          number of transistors (thin wires)
//	# same NAME NAME...      the named nets are one net
//	# differ NAME NAME       the named nets are different nets

func fixtureFiles(t *testing.T) []string {
	files, err := filepath.Glob(filepath.Join("..", "testdata", "netlist", "*.fix"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures found: %v", err)
	}
	return files
}

// extent is the area a fixture's role map covers: everything drawn, labelled
// or placed, plus a one-pixel border.
func extent(d *doc.Document, f *doc.Flat) image.Rectangle {
	r := f.Pixels.Bounds()
	for _, l := range f.Labels {
		r = r.Union(image.Rectangle{Min: l.Pos, Max: l.Pos.Add(image.Pt(1, 1))})
	}
	for _, inst := range f.Instances {
		r = r.Union(inst.Rect)
	}
	return r.Inset(-1)
}

func golden(nl *Netlist, r image.Rectangle) string {
	var b strings.Builder
	for _, row := range nl.RoleMap(r) {
		b.WriteString(row + "\n")
	}
	fmt.Fprintf(&b, "nets %d, transistors %d\n", len(nl.Nets), len(nl.Transistors))
	for _, d := range nl.Diagnostics {
		b.WriteString(d.String() + "\n")
	}
	return b.String()
}

func codes(nl *Netlist) []string {
	var cs []string
	for _, d := range nl.Diagnostics {
		if !slices.Contains(cs, d.Code) {
			cs = append(cs, d.Code)
		}
	}
	slices.Sort(cs)
	if cs == nil {
		cs = []string{"none"}
	}
	return cs
}

func TestFixtures(t *testing.T) {
	for _, path := range fixtureFiles(t) {
		name := strings.TrimSuffix(filepath.Base(path), ".fix")
		t.Run(name, func(t *testing.T) {
			fx, err := fixture.ParseFile(path)
			if err != nil {
				t.Fatal(err)
			}
			flat := fx.Doc.Flatten()
			r := extent(fx.Doc, flat)
			thin := Compile(flat, Options{Variant: ThinWires})
			iso := Compile(flat, Options{Variant: IsolatedSources})

			got := "== thin-wires\n" + golden(thin, r) + "== isolated-sources\n" + golden(iso, r)
			gpath := strings.TrimSuffix(path, ".fix") + ".golden"
			if *update {
				if err := os.WriteFile(gpath, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			} else if want, err := os.ReadFile(gpath); err != nil {
				t.Errorf("no golden (run go test ./netlist -update and review it): %v", err)
			} else if string(want) != got {
				t.Errorf("classification differs from %s:\ngot:\n%s\nwant:\n%s", gpath, got, want)
			}

			for _, dir := range fx.Directives {
				switch dir.Word {
				case "diag", "diag-isolated":
					nl := thin
					if dir.Word == "diag-isolated" {
						nl = iso
					}
					want := slices.Clone(dir.Args)
					slices.Sort(want)
					if got := codes(nl); !slices.Equal(got, want) {
						t.Errorf("line %d: %s codes = %v, want %v\n%s", dir.Line, dir.Word, got, want, golden(nl, r))
					}
				case "nets", "transistors":
					n, _ := strconv.Atoi(dir.Args[0])
					got := len(thin.Nets)
					if dir.Word == "transistors" {
						got = len(thin.Transistors)
					}
					if got != n {
						t.Errorf("line %d: %d %s, want %d", dir.Line, got, dir.Word, n)
					}
				case "same", "differ":
					var ids []NetID
					for _, a := range dir.Args {
						id, ok := thin.Lookup(a)
						if !ok {
							t.Errorf("line %d: no net named %q", dir.Line, a)
						}
						ids = append(ids, id)
					}
					for _, id := range ids[1:] {
						if dir.Word == "same" && id != ids[0] {
							t.Errorf("line %d: %v are not one net", dir.Line, dir.Args)
						}
						if dir.Word == "differ" && id == ids[0] {
							t.Errorf("line %d: %v are the same net", dir.Line, dir.Args)
						}
					}
				}
			}
		})
	}
}

// wrap moves a fixture's root content into a definition and places it once
// with the given orientation, returning the new document and the map from
// old world coordinates to new ones.
func wrap(d *doc.Document, r image.Rectangle, o doc.Orient) (*doc.Document, doc.Affine) {
	root := d.RootDef()
	def := doc.NewDefinition("wrapped", r.Dx(), r.Dy())
	shift := doc.Translate(-r.Min.X, -r.Min.Y)
	root.Pixels.ForEach(func(x, y int) {
		p := shift.Apply(image.Pt(x, y))
		def.Pixels.Set(p.X, p.Y, true)
	})
	for _, l := range root.Labels {
		p := shift.Apply(l.Pos())
		def.Labels = append(def.Labels, doc.Label{X: p.X, Y: p.Y, Name: l.Name})
	}
	for _, inst := range root.Instances {
		inst.X -= r.Min.X
		inst.Y -= r.Min.Y
		def.Instances = append(def.Instances, inst)
	}
	n := d.Clone()
	n.Defs["wrapped"] = def
	n.RootDef().Pixels = doc.New().RootDef().Pixels
	n.RootDef().Labels = nil
	n.RootDef().Instances = []doc.Instance{{ID: "w", Def: "wrapped", Orient: o, Name: "x"}}
	return n, shift.Then(o.Affine(r.Dx(), r.Dy()))
}

func diagCounts(nl *Netlist) map[string]int {
	m := map[string]int{}
	for _, d := range nl.Diagnostics {
		m[d.Code]++
	}
	return m
}

// TestOrientationInvariance compiles every fixture in all 8 orientations,
// wrapped in an oriented instance, and checks that classification moves with
// the pixels and that the netlist and diagnostics are otherwise unchanged.
func TestOrientationInvariance(t *testing.T) {
	for _, path := range fixtureFiles(t) {
		name := strings.TrimSuffix(filepath.Base(path), ".fix")
		fx, err := fixture.ParseFile(path)
		if err != nil {
			t.Fatal(err)
		}
		flat := fx.Doc.Flatten()
		r := extent(fx.Doc, flat)
		for _, variant := range []Variant{ThinWires, IsolatedSources} {
			base := Compile(flat, Options{Variant: variant})
			for _, o := range doc.AllOrients {
				wd, m := wrap(fx.Doc, r, o)
				if err := wd.Validate(); err != nil {
					t.Fatalf("%s %v: wrapped document invalid: %v", name, o, err)
				}
				got := CompileDoc(wd, Options{Variant: variant})
				for y := r.Min.Y; y < r.Max.Y; y++ {
					for x := r.Min.X; x < r.Max.X; x++ {
						p := m.Apply(image.Pt(x, y))
						if a, b := base.RoleAt(x, y), got.RoleAt(p.X, p.Y); a != b {
							t.Fatalf("%s %v %v: pixel %d,%d is %c, but %c after orienting", name, variant, o, x, y, a.Letter(), b.Letter())
						}
					}
				}
				if len(got.Nets) != len(base.Nets) || len(got.Transistors) != len(base.Transistors) {
					t.Errorf("%s %v %v: %d nets %d transistors, want %d and %d", name, variant, o,
						len(got.Nets), len(got.Transistors), len(base.Nets), len(base.Transistors))
				}
				if a, b := diagCounts(base), diagCounts(got); fmt.Sprint(a) != fmt.Sprint(b) {
					t.Errorf("%s %v %v: diagnostics %v, want %v", name, variant, o, b, a)
				}
			}
		}
	}
}

// TestFarApartPixelsAreRefused checks that a huge bounding box is an error,
// not a multi-gigabyte allocation.
func TestFarApartPixelsAreRefused(t *testing.T) {
	px := bitmap.New()
	px.Set(0, 0, true)
	px.Set(1<<20, 1<<20, true)
	nl := Compile(&doc.Flat{Pixels: px}, Options{})
	if !nl.HasErrors() || nl.Diagnostics[len(nl.Diagnostics)-1].Code != "E_TOO_LARGE" {
		t.Errorf("diagnostics %v", nl.Diagnostics)
	}
}
