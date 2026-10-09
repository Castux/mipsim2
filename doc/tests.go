package doc

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Step is one step of a document's test script. It pins the nets and buses
// in Set, lets the circuit settle, then checks the values in Expect, and
// that at least one of the AnyUnstable nets is unstable (which net of an
// oscillating loop trips first depends on evaluation order). Pins persist
// from step to step, so sequential circuits can be tested.
type Step struct {
	Set         map[string]Value `json:"set,omitempty"`
	Expect      map[string]Value `json:"expect,omitempty"`
	AnyUnstable []string         `json:"any_unstable,omitempty"`
}

// Value is a value in a test step: "high", "low", "float" or "unstable" for
// a net, or an unsigned number for a bus (name_0, name_1, ...). In files a
// number may be written as a JSON number or as a string such as "0xab".
type Value string

// UnmarshalJSON accepts a string or an unsigned integer.
func (v *Value) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*v = Value(s)
		return nil
	}
	if _, err := strconv.ParseUint(string(data), 10, 64); err != nil {
		return fmt.Errorf("test value %s is not a string or an unsigned integer", data)
	}
	*v = Value(data)
	return nil
}

// MarshalJSON writes decimal numbers as JSON numbers and the rest as strings.
func (v Value) MarshalJSON() ([]byte, error) {
	if _, err := strconv.ParseUint(string(v), 10, 64); err == nil {
		return []byte(v), nil
	}
	return json.Marshal(string(v))
}

// Checks are structural assertions on the compiled netlist, used by the
// compiler tests. Nil fields are not checked.
type Checks struct {
	Diag        *[]string  `json:"diag,omitempty"`        // distinct diagnostic codes; empty for a clean circuit
	Nets        *int       `json:"nets,omitempty"`        // number of nets
	Transistors *int       `json:"transistors,omitempty"` // number of transistors
	Same        [][]string `json:"same,omitempty"`        // each group of named nets is one net
	Differ      [][]string `json:"differ,omitempty"`      // the first named net differs from each of the others
}
