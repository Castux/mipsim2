// Package isatest checks a coolproc processor (proc.md) against a reference
// emulator of the instruction set: every instruction in every form, random
// programs, and real programs (multiplication, division, sorting...). Both
// generated processors run it from their tests.
package isatest

// Registers in instruction order.
var Registers = []string{"a", "b", "x", "y", "c", "d"}

// State is what a program leaves behind.
type State struct {
	Reg   [6]uint16 // a b x y c d
	PC    uint16
	Carry bool
	RAM   map[uint16]uint16 // words written (non-zero), by address
	Steps int               // instructions run, the halt included
}

// Emulate runs a program from reset until it halts, or for at most limit
// instructions (ok is false then). The program memory holds the program
// and zeros after it (zero is halt).
func Emulate(prog []uint16, limit int) (s State, ok bool) {
	ram := map[uint16]uint16{}
	var r [6]uint16
	var pc uint16
	carry := false
	for s.Steps = 1; s.Steps <= limit; s.Steps++ {
		var w uint16
		if int(pc) < len(prog) {
			w = prog[pc]
		}
		op, arg := w>>8, w&0xff
		src, dst := arg>>4&7, arg&7
		next := pc + 1
		switch {
		case op == 0x00: // halt: the program counter stays on it
			s.Reg, s.PC, s.Carry = r, pc, carry
			s.RAM = map[uint16]uint16{}
			for k, v := range ram {
				if v != 0 {
					s.RAM[k] = v
				}
			}
			return s, true
		case op == 0x01:
			r[dst] = r[src]
		case op == 0x02:
			r[dst] = ram[r[3]]
		case op == 0x03:
			ram[r[3]] = r[src]
		case op>>4 == 0x1: // jump, jumpc: 0001 c x mm
			mm, taken := op&3, false
			if op>>3&1 == 0 {
				v := r[src]
				taken = []bool{true, v == 0, v != 0, v&0x8000 != 0}[mm]
			} else {
				taken = []bool{true, !carry, carry, false}[mm]
			}
			if taken {
				next = r[3]
			}
		case op>>6 == 0x1: // ALU: 01 ooo mm d
			ooo, m0, m1, d := op>>3&7, op>>1&1 == 1, op>>2&1 == 1, 2+op&1
			a, b := r[0], r[1]
			var res uint16
			switch ooo {
			case 0: // add
				bb := uint32(b)
				if m1 {
					bb = uint32(^b)
				}
				sum := uint32(a) + bb
				if m0 {
					sum++
				}
				res, carry = uint16(sum), sum>>16 == 1
			case 1: // shift by B[3:0], left or right
				n := b & 15
				if m0 {
					res = a >> n
					carry = n > 0 && a>>(n-1)&1 == 1
				} else {
					res = a << n
					carry = n > 0 && a>>(16-n)&1 == 1
				}
			default:
				if m0 {
					a = ^a
				}
				if m1 {
					b = ^b
				}
				switch ooo {
				case 2:
					res = a & b
				case 3:
					res = a | b
				case 4:
					res = a ^ b
				}
			}
			r[d] = res
		case op>>7 == 1: // setl, seth: 1 h 000 ddd
			v, reg := arg, op&7
			if op>>6&1 == 0 {
				r[reg] = v
			} else {
				r[reg] = r[reg]&0xff | v<<8
			}
		}
		pc = next
	}
	return s, false
}
