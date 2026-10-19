package isatest

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/Castux/mipsim2/coolproc/asm"
)

// Machine is a processor booted with a program, after reset.
type Machine interface {
	// Run runs until the processor halts, at most limit instructions, and
	// returns the number of instructions run (-1 if it did not halt).
	Run(limit int) int
	Reg(name string) uint16 // a b x y c d, or pc
	Carry() bool
	RAM() []uint64
}

// Boot loads a program into a processor and resets it.
type Boot func(t *testing.T, prog []uint16) Machine

// Run checks the processor booted by boot against the emulator.
func Run(t *testing.T, boot Boot) {
	t.Run("set", func(t *testing.T) { check(t, boot, setProgram()) })
	t.Run("move", func(t *testing.T) { check(t, boot, moveProgram()) })
	t.Run("load_store", func(t *testing.T) { check(t, boot, memoryProgram()) })
	for _, op := range []string{"add", "and", "or", "xor"} {
		for m := range 4 {
			for _, d := range []string{"x", "y"} {
				name := fmt.Sprintf("%s_mode%d_%s", op, m, d)
				t.Run(name, func(t *testing.T) { check(t, boot, aluProgram(op, m, d, operands, operands)) })
			}
		}
	}
	for _, dir := range []string{"left", "right"} {
		for _, d := range []string{"x", "y"} {
			t.Run("shift_"+dir+"_"+d, func(t *testing.T) {
				check(t, boot, aluProgram("shift", map[string]int{"left": 0, "right": 1}[dir], d, operands, amounts))
			})
		}
	}
	t.Run("jumps", func(t *testing.T) { check(t, boot, jumpProgram()) })
	t.Run("jumps_on_carry", func(t *testing.T) { check(t, boot, carryJumpProgram()) })
	for seed := int64(1); seed <= 8; seed++ {
		t.Run(fmt.Sprintf("random_%d", seed), func(t *testing.T) { check(t, boot, randomProgram(seed, 300)) })
	}
	for _, p := range Programs {
		t.Run(p.Name, func(t *testing.T) {
			s := check(t, boot, p.Source)
			if p.Check != nil {
				if err := p.Check(s); err != nil {
					t.Error(err)
				}
			}
		})
	}
}

// check runs src on the emulator and the processor, and compares what they
// leave: registers, carry, program counter, memory, instruction count.
func check(t *testing.T, boot Boot, src string) State {
	t.Helper()
	prog, err := asm.Assemble(src)
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	const limit = 200000
	want, ok := Emulate(prog, limit)
	if !ok {
		t.Fatalf("the emulator did not halt")
	}
	m := boot(t, prog)
	if n := m.Run(want.Steps + 2); n != want.Steps {
		t.Fatalf("halted after %d instructions, want %d", n, want.Steps)
	}
	for k, r := range Registers {
		if got := m.Reg(r); got != want.Reg[k] {
			t.Errorf("register %s = %#04x, want %#04x", r, got, want.Reg[k])
		}
	}
	if got := m.Reg("pc"); got != want.PC {
		t.Errorf("pc = %#04x, want %#04x", got, want.PC)
	}
	if got := m.Carry(); got != want.Carry {
		t.Errorf("carry = %v, want %v", got, want.Carry)
	}
	bad := 0
	for addr, v := range m.RAM() {
		if w := want.RAM[uint16(addr)]; uint64(w) != v {
			if bad++; bad <= 10 {
				t.Errorf("ram[%#04x] = %#04x, want %#04x", addr, v, w)
			}
		}
	}
	if bad > 10 {
		t.Errorf("... %d words differ", bad)
	}
	return want
}

// operands are interesting 16-bit values: zero, one, signs, all ones,
// patterns.
var operands = []uint16{0, 1, 2, 0x7fff, 0x8000, 0x8001, 0xffff, 0x1234, 0xabcd, 0x5555, 0x00ff, 0xff00}

// amounts are shift amounts: all of B[3:0], and values above 15 (only the
// low four bits count).
var amounts = []uint16{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 0x8003, 0xffff}

// prog builds an assembly program.
type prog struct {
	b     strings.Builder
	label int
}

func (p *prog) f(format string, a ...any) { fmt.Fprintf(&p.b, format+"\n", a...) }

func (p *prog) newLabel() string {
	p.label++
	return fmt.Sprintf("l%d", p.label)
}

// keep stores register r at address addr (Y is used for the address, so Y
// itself goes through C).
func (p *prog) keep(r string, addr int) {
	if r == "y" {
		p.f("move y, c")
		r = "c"
	}
	p.f("set y, %d", addr)
	p.f("store %s", r)
}

// keepCarry stores the carry (0 or 1) at addr, using D and Y.
func (p *prog) keepCarry(addr int) {
	l := p.newLabel()
	p.f("setl d, 0")
	p.f("set y, %s", l)
	p.f("jumpcz")
	p.f("setl d, 1")
	p.f("%s:", l)
	p.keep("d", addr)
}

// setProgram writes every register with setl and seth, alone and combined.
func setProgram() string {
	var p prog
	for k, r := range Registers {
		p.f("setl %s, %d", r, 0x11*(k+1))
		p.keep(r, 0x100+3*k)
		p.f("seth %s, %d", r, 0xa0+k) // keeps the low byte
		p.keep(r, 0x101+3*k)
		p.f("setl %s, 0xfe", r) // clears the high byte
		p.keep(r, 0x102+3*k)
	}
	p.f("halt")
	return p.b.String()
}

// distinct gives every register its own value.
func (p *prog) distinct() {
	for k, r := range Registers {
		p.f("set %s, %d", r, 0x1111*(k+1)+3)
	}
}

// moveProgram copies every register into every register.
func moveProgram() string {
	var p prog
	addr := 0x200
	for _, s := range Registers {
		for _, d := range Registers {
			p.distinct()
			p.f("move %s, %s", s, d)
			p.keep(d, addr)
			addr++
		}
	}
	p.f("halt")
	return p.b.String()
}

// memoryProgram stores every register and loads into every register but Y
// (load y is forbidden: the address would change while loading).
func memoryProgram() string {
	var p prog
	for k, s := range Registers {
		p.distinct()
		p.f("set y, %d", 0x300+k)
		p.f("store %s", s) // store y stores the address itself
	}
	for k, d := range Registers {
		if d == "y" {
			continue
		}
		p.distinct()
		p.f("set y, %d", 0x300+(k+1)%6)
		p.f("load %s", d)
		p.keep(d, 0x310+k)
	}
	p.f("halt")
	return p.b.String()
}

// aluProgram runs one ALU instruction (op, mode m = m1 m0, destination d)
// on every pair of operands, keeping the result and the carry.
func aluProgram(op string, m int, d string, as, bs []uint16) string {
	var p prog
	addr := 0x400
	// Every instruction starts from a known carry: 1, so that a carry left
	// unchanged by and, or, xor shows.
	ins := fmt.Sprintf("word %d", (0x40|map[string]int{"add": 0, "shift": 1, "and": 2, "or": 3, "xor": 4}[op]<<3|m<<1|map[string]int{"x": 0, "y": 1}[d])<<8)
	for _, a := range as {
		for _, b := range bs {
			p.f("set a, 0xffff")
			p.f("setl b, 1")
			p.f("add x") // carry 1
			p.f("set a, %d", a)
			p.f("set b, %d", b)
			p.f("%s ; %s mode %d to %s", ins, op, m, d)
			p.keep(d, addr)
			p.keepCarry(addr + 1)
			addr += 2
		}
	}
	p.f("halt")
	return p.b.String()
}

// jumpProgram tries every jump condition on every register, with values
// that make it true and false, and keeps whether it was taken.
func jumpProgram() string {
	var p prog
	addr := 0x600
	for mm, j := range []string{"jump", "jumpz", "jumpnz", "jumpn"} {
		for _, r := range Registers {
			for _, v := range []uint16{0, 1, 0x7fff, 0x8000, 0xffff} {
				mark := "c"
				if r == "c" {
					mark = "d"
				}
				taken, end := p.newLabel(), p.newLabel()
				p.f("set %s, %d", r, v)
				p.f("set y, %s", taken) // Y is the target: as the source, its value is the target
				if mm == 0 {
					p.f("jump")
				} else {
					p.f("%s %s", j, r)
				}
				p.f("setl %s, 1", mark)
				p.f("set y, %s", end)
				p.f("jump")
				p.f("%s:", taken)
				p.f("setl %s, 2", mark)
				p.f("%s:", end)
				p.keep(mark, addr)
				addr++
			}
		}
	}
	p.f("halt")
	return p.b.String()
}

// carryJumpProgram tries jumpc, jumpcz and jumpcnz with the carry set and
// clear.
func carryJumpProgram() string {
	var p prog
	addr := 0x700
	for _, j := range []string{"jumpc", "jumpcz", "jumpcnz"} {
		for _, carry := range []bool{false, true} {
			if carry {
				p.f("set a, 0xffff")
			} else {
				p.f("setl a, 0")
			}
			p.f("setl b, 1")
			p.f("add x")
			taken, end := p.newLabel(), p.newLabel()
			p.f("set y, %s", taken)
			p.f("%s", j)
			p.f("setl c, 1")
			p.f("set y, %s", end)
			p.f("jump")
			p.f("%s:", taken)
			p.f("setl c, 2")
			p.f("%s:", end)
			p.keep("c", addr)
			addr++
		}
	}
	p.f("halt")
	return p.b.String()
}

// randomProgram is a random program without loops: sets, moves, loads and
// stores, every ALU instruction, and forward jumps of every kind.
func randomProgram(seed int64, n int) string {
	rng := rand.New(rand.NewSource(seed))
	var p prog
	reg := func() string { return Registers[rng.Intn(6)] }
	notY := func() string { return []string{"a", "b", "x", "c", "d"}[rng.Intn(5)] }
	pending := map[int][]string{} // instruction index -> labels to place there
	for i := range n {
		for _, l := range pending[i] {
			p.f("%s:", l)
		}
		switch k := rng.Intn(20); {
		case k < 4:
			p.f("setl %s, %d", reg(), rng.Intn(256))
		case k < 6:
			p.f("seth %s, %d", reg(), rng.Intn(256))
		case k < 8:
			p.f("move %s, %s", reg(), reg())
		case k < 9:
			p.f("store %s", reg())
		case k < 10:
			p.f("load %s", notY())
		case k < 17:
			op := []string{"add", "shift", "and", "or", "xor"}[rng.Intn(5)]
			m, d := rng.Intn(4), rng.Intn(2)
			p.f("word %d", (0x40|map[string]int{"add": 0, "shift": 1, "and": 2, "or": 3, "xor": 4}[op]<<3|m<<1|d)<<8)
		default:
			l := p.newLabel()
			at := i + 1 + rng.Intn(4)
			pending[at] = append(pending[at], l)
			p.f("set y, %s", l)
			switch rng.Intn(6) {
			case 0:
				p.f("jump")
			case 1:
				p.f("jumpz %s", reg())
			case 2:
				p.f("jumpnz %s", reg())
			case 3:
				p.f("jumpn %s", reg())
			case 4:
				p.f("jumpcz")
			default:
				p.f("jumpcnz")
			}
		}
	}
	for i := n; i < n+5; i++ {
		for _, l := range pending[i] {
			p.f("%s:", l)
		}
	}
	p.f("halt")
	return p.b.String()
}
