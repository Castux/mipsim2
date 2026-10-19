package main

import "testing"

func TestControlBlocksBuild(t *testing.T) {
	l := newLibrary()
	for _, d := range []*Def{l.stepDecoder(), l.opcodeDecoder(), l.registerFieldDecoder(), l.controlSignals(),
		l.registerControl(), l.jumpDecision(), l.statusRegisters(), l.clockControl()} {
		bench(t, l, d)
	}
}

func TestStatusRegisters(t *testing.T) {
	l := newLibrary()
	r := bench(t, l, l.statusRegisters())
	drive(t, r, map[string]bool{"not_reset": false, "write_carry": false, "write_result": false})
	drive(t, r, map[string]bool{"not_reset": true, "alu_carry": true, "shift_carry": false, "operation_shift": false})
	drive(t, r, map[string]bool{"write_carry": true})
	drive(t, r, map[string]bool{"write_carry": false})
	if !read(t, r, "carry") {
		t.Error("carry not taken from the alu")
	}
	drive(t, r, map[string]bool{"operation_shift": true, "write_carry": true})
	drive(t, r, map[string]bool{"write_carry": false})
	if read(t, r, "carry") {
		t.Error("carry not taken from the shifter")
	}
	drive(t, r, map[string]bool{"is_halt": true, "jump_condition": true, "write_result": true})
	if !read(t, r, "halted") || !read(t, r, "jump_taken") {
		t.Error("halt or jump not latched")
	}
}
