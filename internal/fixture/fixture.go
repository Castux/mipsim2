// Package fixture parses the ASCII test fixtures in testdata/.
//
// A fixture is text: drawing rows of '#' (on) and '.' (off), and directive
// lines starting with "# " (hash, space). A line made only of '#' and '.'
// is always a drawing row, so "###" is three on pixels, never a directive.
// Blank lines are ignored; draw empty rows with '.'.
//
// Structural directives build the document:
//
//	# def NAME WxH              start a definition block; its rows start at y=0
//	# root                      go back to the root block, continuing its rows
//	# label NAME X,Y            label in the current block (local coordinates)
//	# input NAME X,Y            a label that tests pin
//	# output NAME X,Y           a label that tests check
//	# place DEF NAME X,Y [ORIENT]  place an instance in the current block
//	# line X,Y X,Y              turn on a horizontal or vertical segment (inclusive)
//	# px X,Y                    turn on one pixel
//
// Every other "# " line is kept as a Directive in file order, for the tests
// that understand it (for example "expect" lines in behaviour fixtures). A
// first line naming the file, like "# inverter.fix", is just such a line.
package fixture

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Castux/mipsim2/doc"
)

// Directive is a "# word args..." line that is not structural.
type Directive struct {
	Line  int // 1-based line number
	Word  string
	Args  []string
	Block doc.DefID // the block it appeared in
}

// Fixture is a parsed fixture file.
type Fixture struct {
	Doc        *doc.Document
	Inputs     []string // names declared with "input", in order
	Outputs    []string // names declared with "output", in order
	Directives []Directive
}

// Find returns the directives with the given word, in file order.
func (f *Fixture) Find(word string) []Directive {
	var out []Directive
	for _, d := range f.Directives {
		if d.Word == word {
			out = append(out, d)
		}
	}
	return out
}

// ParseFile reads and parses a fixture file.
func ParseFile(path string) (*Fixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// MustParse is Parse for fixtures written inline in tests; it panics on error.
func MustParse(src string) *Fixture {
	f, err := Parse(src)
	if err != nil {
		panic(err)
	}
	return f
}

func isRow(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] != '#' && s[i] != '.' {
			return false
		}
	}
	return true
}

func parsePoint(s string) (int, int, error) {
	xs, ys, ok := strings.Cut(s, ",")
	if !ok {
		return 0, 0, fmt.Errorf("want X,Y, got %q", s)
	}
	x, err1 := strconv.Atoi(xs)
	y, err2 := strconv.Atoi(ys)
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("want X,Y, got %q", s)
	}
	return x, y, nil
}

// Parse parses fixture text into a validated document plus directives.
func Parse(src string) (*Fixture, error) {
	d := doc.New()
	f := &Fixture{Doc: d}
	cur := d.RootDef()
	nextRow := map[doc.DefID]int{d.Root: 0}
	var order []doc.DefID // definitions in declaration order, for error messages

	for i, raw := range strings.Split(src, "\n") {
		lineNo := i + 1
		line := strings.TrimRight(raw, " \t\r")
		fail := func(format string, args ...any) error {
			return fmt.Errorf("line %d: %s", lineNo, fmt.Sprintf(format, args...))
		}

		switch {
		case line == "":
			continue
		case isRow(line):
			y := nextRow[cur.ID]
			for x := 0; x < len(line); x++ {
				if line[x] == '#' {
					cur.Pixels.Set(x, y, true)
				}
			}
			nextRow[cur.ID] = y + 1
			continue
		case !strings.HasPrefix(line, "# "):
			return nil, fail("not a drawing row or a directive: %q", line)
		}

		fields := strings.Fields(line[2:])
		if len(fields) == 0 {
			continue
		}
		word, args := fields[0], fields[1:]
		switch word {
		case "def":
			if len(args) != 2 {
				return nil, fail("want: def NAME WxH")
			}
			ws, hs, ok := strings.Cut(args[1], "x")
			w, err1 := strconv.Atoi(ws)
			h, err2 := strconv.Atoi(hs)
			if !ok || err1 != nil || err2 != nil {
				return nil, fail("bad size %q, want WxH", args[1])
			}
			id := doc.DefID(args[0])
			if d.Defs[id] != nil {
				return nil, fail("definition %q declared twice", id)
			}
			cur = doc.NewDefinition(id, w, h)
			d.Defs[id] = cur
			nextRow[id] = 0
			order = append(order, id)

		case "root":
			cur = d.RootDef()

		case "label", "input", "output":
			if len(args) != 2 {
				return nil, fail("want: %s NAME X,Y", word)
			}
			x, y, err := parsePoint(args[1])
			if err != nil {
				return nil, fail("%v", err)
			}
			cur.Labels = append(cur.Labels, doc.Label{X: x, Y: y, Name: args[0]})
			switch word {
			case "input":
				f.Inputs = append(f.Inputs, args[0])
			case "output":
				f.Outputs = append(f.Outputs, args[0])
			}

		case "px", "line":
			want := 1
			if word == "line" {
				want = 2
			}
			if len(args) != want {
				return nil, fail("want: px X,Y or line X,Y X,Y")
			}
			x0, y0, err := parsePoint(args[0])
			if err != nil {
				return nil, fail("%v", err)
			}
			x1, y1 := x0, y0
			if word == "line" {
				if x1, y1, err = parsePoint(args[1]); err != nil {
					return nil, fail("%v", err)
				}
				if x0 != x1 && y0 != y1 {
					return nil, fail("line must be horizontal or vertical")
				}
			}
			for y := min(y0, y1); y <= max(y0, y1); y++ {
				for x := min(x0, x1); x <= max(x0, x1); x++ {
					cur.Pixels.Set(x, y, true)
				}
			}

		case "place":
			if len(args) != 3 && len(args) != 4 {
				return nil, fail("want: place DEF NAME X,Y [ORIENT]")
			}
			x, y, err := parsePoint(args[2])
			if err != nil {
				return nil, fail("%v", err)
			}
			o := doc.Identity
			if len(args) == 4 {
				if o, err = doc.ParseOrient(args[3]); err != nil {
					return nil, fail("%v", err)
				}
			}
			cur.Instances = append(cur.Instances, doc.Instance{
				ID:     doc.InstID(fmt.Sprintf("i%d", len(cur.Instances)+1)),
				Def:    doc.DefID(args[0]),
				X:      x,
				Y:      y,
				Orient: o,
				Name:   args[1],
			})

		default:
			f.Directives = append(f.Directives, Directive{Line: lineNo, Word: word, Args: args, Block: cur.ID})
		}
	}

	for _, id := range order {
		def := d.Defs[id]
		if rows := nextRow[id]; rows > def.H {
			return nil, fmt.Errorf("definition %q: %d rows drawn for height %d", id, rows, def.H)
		}
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return f, nil
}
