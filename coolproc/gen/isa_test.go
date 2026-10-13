package main

import "testing"

func TestSetAndMove(t *testing.T) {
	m := boot(t, `
		setl a, 0x34
		seth a, 0x12
		setl b, 0xff     ; zero extends
		seth c, 0xab     ; keeps the low byte (reset: 0)
		setl c, 0x01     ; clears the high byte
		seth c, 0xcd
		move a, d
		move d, x
		set y, 0xbeef
		halt`)
	m.run(20)
	m.expect(map[string]uint64{"a": 0x1234, "b": 0xff, "c": 0xcd01, "d": 0x1234, "x": 0x1234, "y": 0xbeef, "pc": 10})
}

func TestALU(t *testing.T) {
	type c struct {
		ins  string
		a, b uint16
		want uint16
	}
	for _, tc := range []c{
		{"add", 0x1234, 0x0f0f, 0x2143},
		{"add y", 0xffff, 1, 0},
		{"inc", 5, 7, 13},
		{"sub", 10, 3, 7},
		{"sub", 3, 10, 0xfff9},
		{"subb", 10, 3, 6},
		{"and", 0xff0f, 0x0ff0, 0x0f00},
		{"and x na", 0xff0f, 0x0ff0, 0x00f0},
		{"or", 0xf000, 0x000f, 0xf00f},
		{"nand", 0xff00, 0x0ff0, 0xf0ff},
		{"nor", 0xff00, 0x0ff0, 0x000f},
		{"xor", 0xff00, 0x0ff0, 0xf0f0},
		{"xnor", 0xff00, 0x0ff0, 0x0f0f},
		{"shl", 0x8421, 4, 0x4210},
		{"shr", 0x8421, 4, 0x0842},
		{"shl", 0x1234, 0, 0x1234},
		{"shl", 1, 15, 0x8000},
		{"shr", 0x8000, 15, 1},
		{"shl", 0xffff, 0x13, 0xfff8}, // only B[3:0]
	} {
		ins := tc.ins
		dest := "x"
		if len(ins) > 4 && ins[len(ins)-2:] == " y" {
			dest = "y"
		}
		switch ins {
		case "add y":
			ins = "add y"
		default:
			if !containsSpace(ins) {
				ins += " x"
			}
		}
		m := boot(t, `
			set a, `+itoa(tc.a)+`
			set b, `+itoa(tc.b)+`
			`+ins+`
			halt`)
		m.run(10)
		if got := m.reg(dest); got != uint64(tc.want) {
			t.Errorf("%s %#x %#x = %#x, want %#x", tc.ins, tc.a, tc.b, got, tc.want)
		}
	}
}

func containsSpace(s string) bool {
	for _, r := range s {
		if r == ' ' {
			return true
		}
	}
	return false
}

func itoa(v uint16) string {
	const hex = "0123456789abcdef"
	return "0x" + string([]byte{hex[v>>12], hex[v>>8&15], hex[v>>4&15], hex[v&15]})
}

func TestCarry(t *testing.T) {
	for _, tc := range []struct {
		prog  string
		carry bool
	}{
		{"set a, 0xffff\n setl b, 1\n add x", true},
		{"set a, 0x7fff\n setl b, 1\n add x", false},
		{"setl a, 5\n setl b, 3\n sub x", true}, // no borrow
		{"setl a, 3\n setl b, 5\n sub x", false},
		{"set a, 0x8000\n setl b, 1\n shl x", true},
		{"set a, 0x4000\n setl b, 1\n shl x", false},
		{"setl a, 1\n setl b, 1\n shr x", true},
		{"setl a, 4\n setl b, 3\n shr x", true}, // bit 2 shifted out last
		{"setl a, 4\n setl b, 2\n shr x", false},
		{"set a, 0xffff\n setl b, 0\n shl x", false},
	} {
		m := boot(t, tc.prog+"\n halt")
		m.run(10)
		want := "0"
		if tc.carry {
			want = "1"
		}
		if got := m.r.Format("carry"); got != want {
			t.Errorf("%q: carry %s, want %s", tc.prog, got, want)
		}
	}
	// Other instructions leave the carry alone.
	m := boot(t, "set a, 0xffff\n setl b, 1\n add x\n xor x\n move a, c\n halt")
	m.run(10)
	if m.r.Format("carry") != "1" {
		t.Error("xor or move changed the carry")
	}
}

func TestLoadStore(t *testing.T) {
	m := boot(t, `
		set y, 0x1234
		set a, 0xcafe
		store a
		setl y, 7
		setl b, 0x42
		store b
		set y, 0x1234
		load c
		setl y, 7
		load d
		halt`)
	m.run(20)
	m.expect(map[string]uint64{"c": 0xcafe, "d": 0x42})
	if w := m.ram.Words(); w[0x1234] != 0xcafe || w[7] != 0x42 {
		t.Errorf("ram[0x1234] = %#x, ram[7] = %#x", w[0x1234], w[7])
	}
}

func TestJumps(t *testing.T) {
	// Each test sets c to 1 if the jump is taken, 2 if not.
	for _, tc := range []struct {
		setup, jump string
		taken       bool
	}{
		{"", "jump", true},
		{"setl a, 0", "jumpz a", true},
		{"setl a, 1", "jumpz a", false},
		{"set a, 0x8000", "jumpz a", false},
		{"setl a, 0", "jumpnz a", false},
		{"set a, 0x100", "jumpnz a", true},
		{"set d, 0x8000", "jumpn d", true},
		{"set d, 0x7fff", "jumpn d", false},
		{"setl x, 0", "jumpz x", true},
		{"setl a, 1\n setl b, 1\n add x", "jumpcz", true},
		{"setl a, 1\n setl b, 1\n add x", "jumpcnz", false},
		{"set a, 0xffff\n setl b, 1\n add x", "jumpcnz", true},
		{"set a, 0xffff\n setl b, 1\n add x", "jumpcz", false},
	} {
		m := boot(t, tc.setup+`
			set y, yes
			`+tc.jump+`
			setl c, 2
			halt
		yes:
			setl c, 1
			halt`)
		m.run(20)
		want := uint64(2)
		if tc.taken {
			want = 1
		}
		if got := m.reg("c"); got != want {
			t.Errorf("%s; %s: c = %d, want %d", tc.setup, tc.jump, got, want)
		}
	}
}

func TestLoop(t *testing.T) {
	// Sum 1..10 into x.
	m := boot(t, `
		setl c, 10      ; counter
		setl d, 0       ; total
	loop:
		move d, a
		move c, b
		add x
		move x, d
		move c, a
		setl b, 1
		sub x
		move x, c
		set y, loop
		jumpnz c
		move d, x
		halt`)
	m.run(200)
	m.expect(map[string]uint64{"x": 55})
}
