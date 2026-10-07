package doc

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// mip wraps definitions (and optional extra top-level fields) in a file.
func mip(defs string, extra string) string {
	return `{"format": "mipsim", "version": 1, "root": "top", "defs": {` + defs + `}` + extra + `}`
}

const emptyTop = `"top": {"rows": [], "labels": [], "instances": []}`

// TestHostileFiles checks that small malformed or hostile files are refused
// quickly with a located message, instead of hanging, exhausting memory or
// loading something else than written.
func TestHostileFiles(t *testing.T) {
	// d0 holds two d1, d1 two d2, ...: 2^30 instances in a few KB. Sizes
	// double in width and height alternately, so every level is valid.
	const levels = 30
	w, h := make([]int, levels), make([]int, levels)
	w[levels-1], h[levels-1] = 1, 1
	for i := levels - 2; i >= 0; i-- {
		w[i], h[i] = w[i+1], h[i+1]
		if i%2 == 0 {
			w[i] *= 2
		} else {
			h[i] *= 2
		}
	}
	fan := []string{`"top": {"rows": [], "labels": [], "instances": [{"id": "i1", "def": "d0", "x": 0, "y": 0}]}`}
	for i := range levels {
		insts := `[]`
		if i < levels-1 {
			dx, dy := w[i+1], 0
			if i%2 == 1 {
				dx, dy = 0, h[i+1]
			}
			insts = fmt.Sprintf(`[{"id": "a", "def": "d%d", "x": 0, "y": 0}, {"id": "b", "def": "d%d", "x": %d, "y": %d}]`, i+1, i+1, dx, dy)
		}
		fan = append(fan, fmt.Sprintf(`"d%d": {"w": %d, "h": %d, "rows": [], "labels": [], "instances": %s}`, i, w[i], h[i], insts))
	}

	cases := []struct {
		name, file, want string
	}{
		{"exponential fan-out", mip(strings.Join(fan, ","), ""), "over 1048576 instances"},
		{"huge definition", mip(emptyTop+`, "a": {"w": 1, "h": 1099511627776, "rows": [], "labels": [], "instances": []}`, ""), "above the limit"},
		{"far origin", mip(`"top": {"origin": [9223372036854775000, 0], "rows": ["##"], "labels": [], "instances": []}`, ""), "coordinate limit"},
		{"far instance", mip(`"top": {"rows": [], "labels": [], "instances": [{"id": "i1", "def": "a", "x": -9000000000000000000, "y": 0}]}, "a": {"w": 1, "h": 1, "rows": [], "labels": [], "instances": []}`, ""), "coordinate limit"},
		{"far label", mip(`"top": {"rows": [], "labels": [{"x": 0, "y": 99999999, "name": "a"}], "instances": []}`, ""), "coordinate limit"},
		{"trailing data", mip(emptyTop, "") + ` garbage {{{`, "after the document"},
		{"two documents", mip(emptyTop, "") + mip(emptyTop, ""), "after the document"},
		{"duplicate definition", mip(emptyTop+`, "top": {"rows": ["#"], "labels": [], "instances": []}`, ""), `line 1, column `},
		{"duplicate field", `{"format": "mipsim", "format": "mipsim", "version": 1, "root": "top", "defs": {` + emptyTop + `}}`, `duplicate key "format"`},
		{"syntax error position", "{\n  \"format\": \"mipsim\",\n  \"version\": 1,\n  oops\n}", "line 4, column 3"},
		{"truncated", `{"format": "mipsim", "version"`, "ends too early"},
		{"device without name", mip(emptyTop, `, "devices": [{"kind": "memory"}]`), `needs a "name"`},
		{"device bad name", mip(emptyTop, `, "devices": [{"kind": "memory", "name": "my ram"}]`), "invalid name"},
		{"duplicate devices", mip(emptyTop, `, "devices": [{"kind": "memory", "name": "a"}, {"kind": "memory", "name": "a"}]`), "same name"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start := time.Now()
			_, err := Load([]byte(c.file))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %v, want it to mention %q", err, c.want)
			}
			if d := time.Since(start); d > time.Second {
				t.Errorf("took %v", d)
			}
		})
	}
}

func TestLoadAcceptsBOM(t *testing.T) {
	if _, err := Load([]byte("\xef\xbb\xbf" + mip(emptyTop, ""))); err != nil {
		t.Error(err)
	}
}

// TestDeviceConfigIsOneSource checks that Kind and Name cannot drift from the
// raw configuration that Save writes.
func TestDeviceConfigIsOneSource(t *testing.T) {
	d, err := Load([]byte(mip(emptyTop, `, "devices": [{"kind": "memory", "name": "ram", "words": 9007199254740993}]`)))
	if err != nil {
		t.Fatal(err)
	}
	d.Devices[0].Name = "renamed"
	if _, err := d.Save(); err == nil || !strings.Contains(err.Error(), "do not match") {
		t.Errorf("save with a drifted name: %v", err)
	}
	d.Devices[0].Name = "ram"
	other := d.Clone()
	other.Devices[0], _ = NewDeviceConfig([]byte(`{"kind": "memory", "name": "ram", "words": 9007199254740992}`))
	if d.Equal(other) {
		t.Error("2^53+1 and 2^53 compare equal")
	}
}

func TestFloorTo(t *testing.T) {
	for _, c := range [][3]int{{0, 16, 0}, {15, 16, 0}, {16, 16, 16}, {-1, 16, -16}, {-16, 16, -16}, {-17, 16, -32}} {
		if got := floorTo(c[0], c[1]); got != c[2] {
			t.Errorf("floorTo(%d, %d) = %d, want %d", c[0], c[1], got, c[2])
		}
	}
}
