package doc

import (
	"strings"
	"testing"
	"time"
)

const specExample = `
{
  "format": "mipsim",
  "version": 3,
  "note": "the example from the spec",
  "root": "top",
  "defs": {
    "top": {
      "origin": [-12, -8],
      "rows": ["...#.....", "..###....", "...#...."],
      "labels": [{"x": -9, "y": -8, "name": "clock"}],
      "instances": [{"id": "i1", "def": "nand2", "x": 20, "y": 4, "orient": "r90", "name": "g1"}]
    },
    "nand2": {
      "w": 11, "h": 9,
      "rows": ["..."],
      "labels": [{"x": 0, "y": 4, "name": "a"}],
      "instances": []
    }
  },
  "devices": [{"kind": "memory", "name": "data_ram", "addr": "port_addr"}],
  "tests": [
    {"set": {"clock": "high", "a": 0}, "expect": {"q": "low", "sum": "0xff"}},
    {"set": {"clock": "low"}, "any_unstable": ["x", "y"]}
  ],
  "checks": {"diag": [], "nets": 3, "same": [["a", "b"]]}
}`

func TestLoadSpecExample(t *testing.T) {
	d, err := LoadString(specExample)
	if err != nil {
		t.Fatal(err)
	}
	root := d.RootDef()
	if !root.Pixels.Get(-9, -8) || !root.Pixels.Get(-10, -7) || root.Pixels.Count() != 5 {
		t.Errorf("root pixels wrong:\n%v", root.Pixels)
	}
	if inst := root.Instances[0]; inst.Orient != (Orient{Rot: 1}) || inst.Def != "nand2" {
		t.Errorf("instance = %+v", inst)
	}
	if len(d.Devices) != 1 || d.Devices[0].Kind != "memory" || d.Devices[0].Name != "data_ram" {
		t.Errorf("devices = %+v", d.Devices)
	}
}

func TestSaveFormatIsStable(t *testing.T) {
	d, err := LoadString(specExample)
	if err != nil {
		t.Fatal(err)
	}
	data, err := d.Save()
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "format": "mipsim",
  "version": 3,
  "note": "the example from the spec",
  "root": "top",
  "defs": {
    "nand2": {
      "w": 11,
      "h": 9,
      "labels": [
        {
          "x": 0,
          "y": 4,
          "name": "a"
        }
      ]
    },
    "top": {
      "origin": [
        -16,
        -8
      ],
      "rows": [
        ".......#",
        "......###",
        ".......#"
      ],
      "labels": [
        {
          "x": -9,
          "y": -8,
          "name": "clock"
        }
      ],
      "instances": [
        {
          "id": "i1",
          "def": "nand2",
          "x": 20,
          "y": 4,
          "orient": "r90",
          "name": "g1"
        }
      ]
    }
  },
  "devices": [
    {
      "kind": "memory",
      "name": "data_ram",
      "addr": "port_addr"
    }
  ],
  "tests": [
    {
      "set": {
        "a": 0,
        "clock": "high"
      },
      "expect": {
        "q": "low",
        "sum": "0xff"
      }
    },
    {
      "set": {
        "clock": "low"
      },
      "any_unstable": [
        "x",
        "y"
      ]
    }
  ],
  "checks": {
    "diag": [],
    "nets": 3,
    "same": [
      [
        "a",
        "b"
      ]
    ]
  }
}
`
	if string(data) != want {
		t.Errorf("Save output:\n%s\nwant:\n%s", data, want)
	}
	again, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	data2, _ := again.Save()
	if string(data2) != string(data) {
		t.Error("save of a loaded file is not byte-identical")
	}
}

func TestLoadRejects(t *testing.T) {
	cases := []struct{ name, edit, want string }{
		{"bad format", `"format": "mipsim"|"format": "other"`, "format is"},
		{"newer version", `"version": 3|"version": 4`, "newer"},
		{"old pattern language", `"version": 3|"version": 2`, "no longer read"},
		{"unknown field", `"root": "top",|"root": "top", "colour": 3,`, "unknown field"},
		{"bad row char", `"rows": ["..."]|"rows": [".x."]`, "unexpected character"},
		{"row too wide", `"rows": ["..."]|"rows": ["............"]`, "characters for width"},
		{"bad orient", `"orient": "r90"|"orient": "r45"`, "unknown orientation"},
		{"root with size", `"origin": [-12, -8],|"origin": [-12, -8], "w": 3,`, "root definition has no size"},
		{"invariant", `"x": 20, "y": 4|"x": -12, "y": -8`, "parent has pixels inside"},
		{"device without kind", `{"kind": "memory",|{`, "needs a"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			from, to, _ := strings.Cut(c.edit, "|")
			if !strings.Contains(specExample, from) {
				t.Fatalf("test edit %q does not apply", from)
			}
			_, err := LoadString(strings.Replace(specExample, from, to, 1))
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not mention %q", err, c.want)
			}
		})
	}
}

func TestEmptyDocumentRoundTrip(t *testing.T) {
	d := New()
	data, err := d.Save()
	if err != nil {
		t.Fatal(err)
	}
	back, err := Load(data)
	if err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	if !d.Equal(back) {
		t.Fatal("empty document changed")
	}
}

func TestMillionPixelRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("large document")
	}
	d := New()
	px := d.RootDef().Pixels
	// About 1M on pixels: a 2000x1000 area with a fixed pseudo-random pattern.
	for y := range 1000 {
		for x := range 2000 {
			if (x*7+y*13+x*y)%3 != 0 {
				px.Set(x-1000, y-500, true)
			}
		}
	}
	start := time.Now()
	data, err := d.Save()
	if err != nil {
		t.Fatal(err)
	}
	saved := time.Since(start)
	start = time.Now()
	back, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	loaded := time.Since(start)
	if !d.Equal(back) {
		t.Fatal("round trip changed the document")
	}
	t.Logf("%d pixels, %d bytes; save %v, load %v", px.Count(), len(data), saved, loaded)
}
