package isatest

import (
	"fmt"
	"math/bits"
	"sort"
	"strings"
)

// Program is a real program, with a check of its result on top of the
// comparison with the emulator.
type Program struct {
	Name   string
	Source string
	Check  func(State) error
}

// Programs use loops, memory, the carry and every kind of jump.
var Programs = []Program{
	multiply(1234, 56),
	multiply(0xffff, 0xffff),
	multiply(0, 777),
	divide(50000, 7),
	divide(65535, 65535),
	divide(12345, 1),
	sortProgram([]uint16{9, 0x8000, 3, 0xffff, 0, 77, 3, 1234}),
	popcount(0xb6d9),
	copyProgram(10),
	subroutines(),
}

func want(s State, addr uint16, v uint16) error {
	if s.RAM[addr] != v {
		return fmt.Errorf("ram[%#x] = %d, want %d", addr, s.RAM[addr], v)
	}
	return nil
}

// multiply is shift-and-add: c is the multiplicand, d the multiplier, the
// product (low 16 bits) builds up at 0x13.
func multiply(m, n uint16) Program {
	return Program{
		Name: fmt.Sprintf("multiply_%d_by_%d", m, n),
		Source: fmt.Sprintf(`
		set c, %d
		set d, %d
	loop:
		set y, done
		jumpz d          ; no multiplier bits left
		move d, a
		setl b, 1
		and x            ; lowest bit of the multiplier
		set y, skip
		jumpz x
		set y, 0x13      ; product += multiplicand
		load a
		move c, b
		add x
		store x
	skip:
		move c, a        ; multiplicand <<= 1
		setl b, 1
		shl x
		move x, c
		move d, a        ; multiplier >>= 1
		setl b, 1
		shr x
		move x, d
		set y, loop
		jump
	done:
		halt`, m, n),
		Check: func(s State) error { return want(s, 0x13, m*n) },
	}
}

// divide is restoring long division, the compare done by a subtraction and
// its carry (set when there is no borrow): quotient at 0x20, remainder at
// 0x22.
func divide(n, d uint16) Program {
	return Program{
		Name: fmt.Sprintf("divide_%d_by_%d", n, d),
		Source: fmt.Sprintf(`
		setl c, 0        ; remainder
		set d, %d        ; dividend, becoming the quotient
		set y, 0x23
		setl a, 16
		store a          ; bits left
	loop:
		move c, a        ; remainder = remainder << 1 | dividend >> 15
		setl b, 1
		shl x
		move x, c
		move d, a
		setl b, 15
		shr x
		move x, a
		move c, b
		or x
		move x, c
		move d, a        ; dividend <<= 1
		setl b, 1
		shl x
		move x, d
		move c, a        ; remainder - divisor: carry if it fits
		set b, %d
		sub x
		set y, no_fit
		jumpcz
		move x, c
		move d, a        ; quotient bit
		setl b, 1
		or x
		move x, d
	no_fit:
		set y, 0x23
		load a
		setl b, 1
		sub x
		store x
		set y, loop
		jumpnz x
		set y, 0x20
		store d
		set y, 0x22
		store c
		halt`, n, d),
		Check: func(s State) error {
			if err := want(s, 0x20, n/d); err != nil {
				return err
			}
			return want(s, 0x22, n%d)
		},
	}
}

// sortProgram bubble-sorts words at 0x40 (unsigned), comparing by
// subtraction and swapping through registers.
func sortProgram(vs []uint16) Program {
	var init strings.Builder
	for k, v := range vs {
		fmt.Fprintf(&init, "\tset y, %d\n\tset a, %d\n\tstore a\n", 0x40+k, v)
	}
	n := len(vs)
	sorted := append([]uint16(nil), vs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return Program{
		Name: "sort",
		Source: init.String() + fmt.Sprintf(`
		set y, 0x30
		setl a, %d
		store a          ; passes left
	pass:
		set c, 0x40      ; address of the pair
		set y, 0x31
		setl a, %d
		store a          ; pairs left in this pass
	pair:
		move c, y
		load d           ; first
		move c, a
		setl b, 1
		add y
		load b           ; second
		move b, a
		move d, b
		sub x            ; second - first: carry if in order
		set y, in_order
		jumpcnz
		move c, y        ; swap
		store a
		move c, a
		setl b, 1
		add y
		store d
	in_order:
		move c, a
		setl b, 1
		add x
		move x, c
		set y, 0x31
		load a
		setl b, 1
		sub x
		store x
		set y, pair
		jumpnz x
		set y, 0x30
		load a
		setl b, 1
		sub x
		store x
		set y, pass
		jumpnz x
		halt`, n, n-1),
		Check: func(s State) error {
			for k, v := range sorted {
				if err := want(s, uint16(0x40+k), v); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// popcount counts set bits by shifting them out through the carry.
func popcount(v uint16) Program {
	return Program{
		Name: fmt.Sprintf("popcount_%#x", v),
		Source: fmt.Sprintf(`
		set c, %d        ; value
		setl d, 0        ; count
		set y, 0x50
		setl a, 16
		store a
	loop:
		move c, a
		setl b, 1
		shr x            ; the low bit goes into the carry
		move x, c
		set y, zero
		jumpcz
		move d, a
		setl b, 1
		add x
		move x, d
	zero:
		set y, 0x50
		load a
		setl b, 1
		sub x
		store x
		set y, loop
		jumpnz x
		set y, 0x51
		store d
		halt`, v),
		Check: func(s State) error { return want(s, 0x51, uint16(bits.OnesCount16(v))) },
	}
}

// copyProgram fills n words with a pattern, then copies them elsewhere.
func copyProgram(n int) Program {
	return Program{
		Name: "copy",
		Source: fmt.Sprintf(`
		set c, 0x80      ; fill 0x80.. with 3, 6, 9...
		setl d, 0
	fill:
		move d, a
		setl b, 3
		add x
		move x, d
		move c, y
		store d
		move c, a
		setl b, 1
		add x
		move x, c
		set b, %d
		move c, a
		sub x
		set y, fill
		jumpnz x
		set c, 0x80      ; copy to 0x90..
	copy:
		move c, y
		load d
		move c, a
		setl b, 0x10
		add y
		store d
		move c, a
		setl b, 1
		add x
		move x, c
		set b, %d
		move c, a
		sub x
		set y, copy
		jumpnz x
		halt`, 0x80+n, 0x80+n),
		Check: func(s State) error {
			for k := range n {
				if err := want(s, uint16(0x90+k), uint16(3*(k+1))); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// subroutines calls a subroutine from two places, the return address in D.
func subroutines() Program {
	return Program{
		Name: "subroutines",
		Source: `
		set c, 21
		set d, back_1
		set y, double
		jump
	back_1:
		set y, 0x60
		store c          ; 42
		set d, back_2
		set y, double
		jump
	back_2:
		set y, 0x61
		store c          ; 84
		halt
	double:              ; c = c + c, then return to d
		move c, a
		move c, b
		add x
		move x, c
		move d, y
		jump`,
		Check: func(s State) error {
			if err := want(s, 0x60, 42); err != nil {
				return err
			}
			return want(s, 0x61, 84)
		},
	}
}
