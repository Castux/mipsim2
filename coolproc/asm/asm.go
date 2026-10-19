// Package asm assembles coolproc programs (proc.md) into ROM words.
package asm

// A small assembler for coolproc (proc.md), to make ROM images for tests
// and demos. One instruction per line; ";" starts a comment; "name:" defines
// a label (an instruction address). Registers: a b x y c d.
//
//	halt
//	move SRC, DEST        copy SRC into DEST
//	load DEST             DEST = ram[Y]
//	store SRC             ram[Y] = SRC
//	setl R, N             R = N (0..255)
//	seth R, N             high byte of R = N (0..255)
//	set R, N              setl and seth (two instructions; N may be a label)
//	jump                  PC = Y
//	jumpz R, jumpnz R, jumpn R      PC = Y if R is 0, not 0, negative
//	jumpcz, jumpcnz       PC = Y if carry is 0, 1
//	goto L                set y, L then jump (three instructions)
//
// ALU instructions take the destination (x or y), then optional modes:
//
//	add D [na] [nb] [c]   A + B; nb inverts B, c adds 1
//	sub D                 A - B   (add nb c)
//	shl D, shr D          A shifted by B[3:0]
//	and D, or D, xor D [na] [nb]
//	nand D, nor D, xnor D (or na nb, and na nb, xor na)

import (
	"fmt"
	"strconv"
	"strings"
)

var regCode = map[string]int{"a": 0, "b": 1, "x": 2, "y": 3, "c": 4, "d": 5}

var aluOps = map[string]int{"add": 0, "shift": 1, "and": 2, "or": 3, "xor": 4}

// aliases: name -> operation and modes (m1 m0 as "nb"/"na"/"c").
var aluAliases = map[string]struct {
	op    string
	modes []string
}{
	"sub":  {"add", []string{"nb", "c"}},
	"subb": {"add", []string{"nb"}}, // A - B - 1
	"inc":  {"add", []string{"c"}},  // A + B + 1
	"shl":  {"shift", nil},
	"shr":  {"shift", []string{"right"}},
	"nand": {"or", []string{"na", "nb"}},
	"nor":  {"and", []string{"na", "nb"}},
	"xnor": {"xor", []string{"na"}},
}

// Assemble returns the program as 16-bit words: opcode in the high byte.
func Assemble(src string) ([]uint16, error) {
	type line struct {
		n      int
		fields []string
	}
	var lines []line
	labels := map[string]int{}
	pc := 0
	for n, raw := range strings.Split(src, "\n") {
		if i := strings.IndexByte(raw, ';'); i >= 0 {
			raw = raw[:i]
		}
		raw = strings.TrimSpace(raw)
		for {
			i := strings.IndexByte(raw, ':')
			if i < 0 {
				break
			}
			name := strings.TrimSpace(raw[:i])
			if _, dup := labels[name]; dup {
				return nil, fmt.Errorf("line %d: label %s defined twice", n+1, name)
			}
			labels[name] = pc
			raw = strings.TrimSpace(raw[i+1:])
		}
		if raw == "" {
			continue
		}
		f := strings.Fields(strings.ReplaceAll(raw, ",", " "))
		f[0] = strings.ToLower(f[0])
		lines = append(lines, line{n + 1, f})
		pc += size(f)
	}
	var out []uint16
	for _, l := range lines {
		words, err := encode(l.fields, labels)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", l.n, err)
		}
		out = append(out, words...)
	}
	return out, nil
}

func size(f []string) int {
	switch f[0] {
	case "set":
		return 2
	case "goto":
		return 3
	}
	return 1
}

func encode(f []string, labels map[string]int) ([]uint16, error) {
	reg := func(i int) (int, error) {
		if i >= len(f) {
			return 0, fmt.Errorf("%s: missing register", f[0])
		}
		r, ok := regCode[strings.ToLower(f[i])]
		if !ok {
			return 0, fmt.Errorf("%s: unknown register %q", f[0], f[i])
		}
		return r, nil
	}
	num := func(i int, limit int) (int, error) {
		if i >= len(f) {
			return 0, fmt.Errorf("%s: missing value", f[0])
		}
		if v, ok := labels[f[i]]; ok {
			return v, nil
		}
		v, err := strconv.ParseInt(f[i], 0, 32)
		if err != nil || v < -int64(limit/2) || v >= int64(limit) {
			return 0, fmt.Errorf("%s: bad value %q", f[0], f[i])
		}
		return int(v) & (limit - 1), nil
	}
	w := func(op, arg int) uint16 { return uint16(op<<8 | arg&0xff) }
	switch f[0] {
	case "halt":
		return []uint16{0}, nil
	case "move":
		s, err := reg(1)
		if err != nil {
			return nil, err
		}
		d, err := reg(2)
		if err != nil {
			return nil, err
		}
		return []uint16{w(0x01, s<<4|d)}, nil
	case "load":
		d, err := reg(1)
		if err != nil {
			return nil, err
		}
		return []uint16{w(0x02, d)}, nil
	case "store":
		s, err := reg(1)
		if err != nil {
			return nil, err
		}
		return []uint16{w(0x03, s<<4)}, nil
	case "setl", "seth", "set":
		r, err := reg(1)
		if err != nil {
			return nil, err
		}
		if f[0] == "set" {
			v, err := num(2, 1<<16)
			if err != nil {
				return nil, err
			}
			return []uint16{w(0x80|r, v), w(0xc0|r, v>>8)}, nil
		}
		v, err := num(2, 256)
		if err != nil {
			return nil, err
		}
		op := 0x80 | r
		if f[0] == "seth" {
			op |= 0x40
		}
		return []uint16{w(op, v)}, nil
	case "jump", "jumpz", "jumpnz", "jumpn":
		mm := map[string]int{"jump": 0, "jumpz": 1, "jumpnz": 2, "jumpn": 3}[f[0]]
		s := 0
		if mm > 0 {
			var err error
			if s, err = reg(1); err != nil {
				return nil, err
			}
		}
		return []uint16{w(0x10|mm, s<<4)}, nil
	case "jumpcz", "jumpcnz":
		mm := map[string]int{"jumpcz": 1, "jumpcnz": 2}[f[0]]
		return []uint16{w(0x18|mm, 0)}, nil
	case "goto":
		v, err := num(1, 1<<16)
		if err != nil {
			return nil, err
		}
		return []uint16{w(0x80|3, v), w(0xc0|3, v>>8), w(0x10, 0)}, nil
	}
	// ALU.
	name, modes := f[0], []string(nil)
	if a, ok := aluAliases[name]; ok {
		name, modes = a.op, a.modes
	}
	o, ok := aluOps[name]
	if !ok {
		return nil, fmt.Errorf("unknown instruction %q", f[0])
	}
	if len(f) < 2 {
		return nil, fmt.Errorf("%s: missing destination (x or y)", f[0])
	}
	d := 0
	switch strings.ToLower(f[1]) {
	case "x":
	case "y":
		d = 1
	default:
		return nil, fmt.Errorf("%s: destination must be x or y", f[0])
	}
	m := 0
	for _, mode := range append(modes, f[2:]...) {
		switch mode {
		case "na", "c", "right":
			m |= 1 // m0: not A, carry in, or right
		case "nb":
			m |= 2 // m1: not B
		default:
			return nil, fmt.Errorf("%s: unknown mode %q", f[0], mode)
		}
	}
	return []uint16{uint16((0x40 | o<<3 | m<<1 | d) << 8)}, nil
}
