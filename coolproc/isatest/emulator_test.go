package isatest

import "testing"

// emulated is a Machine backed by the emulator itself: it checks the suite's
// programs and their checks.
type emulated struct {
	prog []uint16
	s    State
}

func (e *emulated) Run(limit int) int {
	s, ok := Emulate(e.prog, limit)
	if !ok {
		return -1
	}
	e.s = s
	return s.Steps
}

func (e *emulated) Reg(name string) uint16 {
	if name == "pc" {
		return e.s.PC
	}
	for k, r := range Registers {
		if r == name {
			return e.s.Reg[k]
		}
	}
	panic(name)
}

func (e *emulated) Carry() bool { return e.s.Carry }

func (e *emulated) RAM() []uint64 {
	ram := make([]uint64, 65536)
	for a, v := range e.s.RAM {
		ram[a] = uint64(v)
	}
	return ram
}

func TestSuiteOnEmulator(t *testing.T) {
	Run(t, func(t *testing.T, prog []uint16) Machine { return &emulated{prog: prog} })
}

func TestEmulatorBasics(t *testing.T) {
	// sub 3 - 10, shifts and their carry, seth keeping the low byte.
	for _, tc := range []struct {
		prog  []uint16
		reg   int
		want  uint16
		carry bool
	}{
		{[]uint16{0x8003, 0x810a, 0x4600, 0}, 2, 0xfff9, false},   // setl a,3; setl b,10; sub x
		{[]uint16{0x8001, 0xc080, 0x8101, 0x4800, 0}, 2, 2, true}, // a = 0x8001: shl by 1
		{[]uint16{0x8001, 0x8101, 0x4a00, 0}, 2, 0, true},         // shr 1 by 1
		{[]uint16{0x8034, 0xc012, 0}, 0, 0x1234, false},           // setl, seth
		{[]uint16{0x80ff, 0x8101, 0x4200, 0}, 2, 0x101, false},    // add with carry in: 255 + 1 + 1
	} {
		s, ok := Emulate(tc.prog, 100)
		if !ok || s.Reg[tc.reg] != tc.want || s.Carry != tc.carry {
			t.Errorf("%04x: reg %#x carry %v", tc.prog, s.Reg[tc.reg], s.Carry)
		}
	}
}
