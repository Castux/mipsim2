package main

import (
	"fmt"
	"image"

	"github.com/Castux/mipsim2/bitmap"
)

// Registers in instruction order (register numbers 0 to 5).
var registerNames = []string{"a", "b", "x", "y", "c", "d"}

// stepDecoder turns the step counter's state into the instruction's
// steps, and the write pulse in the middle of each step:
//
//	states 0-2   fetching                  write_instruction in state 1
//	states 3-5   incrementing              write_temporary in state 4
//	states 6-8   executing                 write_result in state 7
//	states 9-11  updating_program_counter  write_program_counter in state 10
func (l *Library) stepDecoder() *Def {
	var in []string
	for k := range 6 {
		in = append(in, fmt.Sprintf("stage_%d", k), fmt.Sprintf("stage_%d_inverted", k))
	}
	s := func(k int, on bool) string {
		if on {
			return fmt.Sprintf("stage_%d", k)
		}
		return fmt.Sprintf("stage_%d_inverted", k)
	}
	return l.logicBlock(logic{
		id:     "step_decoder",
		inputs: in,
		outputs: []string{"fetching", "incrementing", "executing", "updating_program_counter",
			"write_instruction", "write_temporary", "write_result", "write_program_counter"},
		terms: map[string][][]string{
			"fetching":                 {{s(2, false), s(5, false)}},
			"incrementing":             {{s(2, true), s(5, false)}},
			"executing":                {{s(2, true), s(5, true)}},
			"updating_program_counter": {{s(2, false), s(5, true)}},
			"write_instruction":        {{s(0, true), s(1, false)}},
			"write_temporary":          {{s(3, true), s(4, false)}},
			"write_result":             {{s(0, false), s(1, true)}},
			"write_program_counter":    {{s(3, false), s(4, true)}},
		},
	})
}

// opcode returns the literals matching opcode bits (instruction 15..8) to a
// pattern such as "01xxxxx0", most significant first.
func opcode(pattern string) []string {
	var lits []string
	for k, ch := range pattern {
		j := 15 - k
		switch ch {
		case '0':
			lits = append(lits, fmt.Sprintf("instruction_%d_inverted", j))
		case '1':
			lits = append(lits, fmt.Sprintf("instruction_%d", j))
		}
	}
	return lits
}

// opcodeDecoder recognises the instruction groups (proc.md, Opcodes).
func (l *Library) opcodeDecoder() *Def {
	var in []string
	for j := 8; j <= 15; j++ {
		in = append(in, fmt.Sprintf("instruction_%d", j), fmt.Sprintf("instruction_%d_inverted", j))
	}
	return l.logicBlock(logic{
		id:     "opcode_decoder",
		inputs: in,
		outputs: []string{"is_halt", "is_move", "is_load", "is_store", "is_jump", "is_jump_on_carry",
			"is_alu", "is_set_low", "is_set_high",
			"operation_add", "operation_shift", "operation_and", "operation_or", "operation_xor"},
		terms: map[string][][]string{
			"is_halt":          {opcode("00000000")},
			"is_move":          {opcode("00000001")},
			"is_load":          {opcode("00000010")},
			"is_store":         {opcode("00000011")},
			"is_jump":          {opcode("00010")},
			"is_jump_on_carry": {opcode("00011")},
			"is_alu":           {opcode("01")},
			"is_set_low":       {opcode("10")},
			"is_set_high":      {opcode("11")},
			"operation_add":    {opcode("xx000")},
			"operation_shift":  {opcode("xx001")},
			"operation_and":    {opcode("xx010")},
			"operation_or":     {opcode("xx011")},
			"operation_xor":    {opcode("xx100")},
		},
	})
}

// registerFieldDecoder turns a 3-bit register number into one line per
// register. The same component decodes the source, destination and set
// target fields.
func (l *Library) registerFieldDecoder() *Def {
	terms := map[string][][]string{}
	var outs []string
	for code, r := range registerNames {
		var lits []string
		for k := range 3 {
			if code>>k&1 == 1 {
				lits = append(lits, fmt.Sprintf("field_%d", k))
			} else {
				lits = append(lits, fmt.Sprintf("field_%d_inverted", k))
			}
		}
		out := "is_" + r
		outs = append(outs, out)
		terms[out] = [][]string{lits}
	}
	return l.logicBlock(logic{
		id:      "register_field_decoder",
		inputs:  []string{"field_0", "field_0_inverted", "field_1", "field_1_inverted", "field_2", "field_2_inverted"},
		outputs: outs,
		terms:   terms,
	})
}

// controlSignals is the table from instruction and step to the routing
// and write lines shared by the datapath (proc.md, Implementation).
// Instruction bits: 8 is the ALU destination (0: X, 1: Y), 9 is mode m0
// (not A, carry in, shift right), 10 is mode m1 (not B).
func (l *Library) controlSignals() *Def {
	return l.logicBlock(logic{
		id: "control_signals",
		inputs: []string{"executing", "updating_program_counter", "write_result",
			"is_move", "is_load", "is_store", "is_jump", "is_alu", "is_set_low", "is_set_high",
			"operation_add", "operation_shift", "instruction_8", "instruction_9", "jump_taken"},
		outputs: []string{"reads_source_now", "write_destination", "write_set_low", "write_set_high",
			"write_alu_to_x", "write_alu_to_y", "alu_to_x", "alu_to_y",
			"immediate_low_to_bus", "immediate_high_to_bus", "memory_read", "memory_write",
			"temporary_to_bus", "jump_target_to_bus", "write_carry", "invert_a"},
		terms: map[string][][]string{
			"reads_source_now":      {{"executing", "is_move"}, {"executing", "is_store"}, {"executing", "is_jump"}},
			"write_destination":     {{"write_result", "is_move"}, {"write_result", "is_load"}},
			"write_set_low":         {{"write_result", "is_set_low"}},
			"write_set_high":        {{"write_result", "is_set_high"}},
			"write_alu_to_x":        {{"write_result", "is_alu", "!instruction_8"}},
			"write_alu_to_y":        {{"write_result", "is_alu", "instruction_8"}},
			"alu_to_x":              {{"is_alu", "!instruction_8"}},
			"alu_to_y":              {{"is_alu", "instruction_8"}},
			"immediate_low_to_bus":  {{"executing", "is_set_low"}},
			"immediate_high_to_bus": {{"executing", "is_set_high"}},
			"memory_read":           {{"executing", "is_load"}},
			"memory_write":          {{"write_result", "is_store"}},
			"temporary_to_bus":      {{"updating_program_counter", "!jump_taken"}},
			"jump_target_to_bus":    {{"updating_program_counter", "jump_taken"}},
			"write_carry":           {{"write_result", "is_alu", "operation_add"}, {"write_result", "is_alu", "operation_shift"}},
			"invert_a":              {{"instruction_9", "!operation_add"}},
		},
	})
}

// registerControl drives one register: onto the bus when it is the source
// being read (or Y, the jump target), and written when it is the
// destination, the target of a set, or (X, Y) the ALU's result.
func (l *Library) registerControl() *Def {
	both := [][]string{
		{"write_destination", "destination_is_this"},
		{"write_set_low", "set_target_is_this"},
		{"write_alu_result"},
	}
	return l.logicBlock(logic{
		id: "register_control",
		inputs: []string{"reads_source_now", "source_is_this", "extra_read",
			"write_destination", "destination_is_this", "write_set_low", "write_set_high",
			"set_target_is_this", "write_alu_result"},
		outputs: []string{"to_bus", "write_low", "write_high"},
		terms: map[string][][]string{
			"to_bus":     {{"reads_source_now", "source_is_this"}, {"extra_read"}},
			"write_low":  both,
			"write_high": append(append([][]string{}, both...), []string{"write_set_high", "set_target_is_this"}),
		},
	})
}

// jumpDecision evaluates the jump condition (mm = instruction bits 9, 8)
// while the source is on the bus.
func (l *Library) jumpDecision() *Def {
	return l.logicBlock(logic{
		id: "jump_decision",
		inputs: []string{"is_jump", "is_jump_on_carry", "instruction_8", "instruction_8_inverted",
			"instruction_9", "instruction_9_inverted", "bus_is_zero", "bus_is_negative", "carry", "carry_inverted"},
		outputs: []string{"jump_condition"},
		terms: map[string][][]string{
			"jump_condition": {
				{"is_jump", "instruction_9_inverted", "instruction_8_inverted"},
				{"is_jump", "instruction_9_inverted", "instruction_8", "bus_is_zero"},
				{"is_jump", "instruction_9", "instruction_8_inverted", "!bus_is_zero"},
				{"is_jump", "instruction_9", "instruction_8", "bus_is_negative"},
				{"is_jump_on_carry", "instruction_9_inverted", "instruction_8_inverted"},
				{"is_jump_on_carry", "instruction_9_inverted", "instruction_8", "carry_inverted"},
				{"is_jump_on_carry", "instruction_9", "instruction_8_inverted", "carry"},
			},
		},
	})
}

// statusRegisters holds the carry, the jump decision and the halt flag.
func (l *Library) statusRegisters() *Def {
	if d, ok := l.get("status_registers"); ok {
		return d
	}
	la := l.latch()
	b := l.build("status_registers", 200, 200)
	carrySel := b.Place(l.selector(), "carry_source", 20, 10)
	carry := b.Place(la, "carry_register", 100, 10)
	jump := b.Place(la, "jump_taken_register", 100, 70)
	halt := b.Place(la, "halted_register", 100, 140)
	ins := []string{"alu_carry", "shift_carry", "operation_shift", "write_carry", "jump_condition", "write_result", "is_halt", "not_reset"}
	for k, n := range ins {
		b.Export(n, n, image.Pt(0, 50+8*k+20*(k/4)))
	}
	outs := []string{"carry", "carry_inverted", "jump_taken", "jump_taken_inverted", "halted"}
	for k, n := range outs {
		b.Export(n, n, image.Pt(199, 20+30*k))
	}
	b.Connect("alu_carry", carrySel, "first")
	b.Connect("shift_carry", carrySel, "second")
	b.Connect("operation_shift", carrySel, "choose")
	b.Connect("carry_result", carrySel, "output")
	b.Connect("carry_result", carry, "data")
	b.Connect("write_carry", carry, "write")
	b.Connect("carry", carry, "value")
	b.Connect("carry_inverted", carry, "value_inverted")
	b.Connect("jump_condition", jump, "data")
	b.Connect("write_result", jump, "write")
	b.Connect("jump_taken", jump, "value")
	b.Connect("jump_taken_inverted", jump, "value_inverted")
	// Halt: written in every instruction's write state; when it is set,
	// the sequencer stops there, so it stays set until reset.
	b.Connect("is_halt", halt, "data")
	b.Connect("write_result", halt, "write")
	b.Connect("halted", halt, "value")
	for _, la := range []*Inst{carry, jump, halt} {
		b.Connect("not_reset", la, "not_reset")
	}
	return b.Route()
}

// clockControl makes the reset line the latches use (not_reset) and stops
// the clock once the processor has halted.
func (l *Library) clockControl() *Def {
	if d, ok := l.get("clock_control"); ok {
		return d
	}
	b := l.build("clock_control", 80, 50)
	resetInv := b.Place(l.nand(1), "reset_inverter", 20, 6)
	haltInv := b.Place(l.nand(1), "halted_inverter", 20, 22)
	gate := b.Place(l.andGate(2), "clock_gate", 40, 30)
	b.Export("reset", "reset", image.Pt(0, 8))
	b.Export("halted", "halted", image.Pt(0, 24))
	b.Export("clock", "clock", image.Pt(0, 40))
	b.Export("not_reset", "not_reset", image.Pt(79, 7))
	b.Export("running_clock", "running_clock", image.Pt(79, 31))
	b.Connect("reset", resetInv, "input")
	b.Connect("not_reset", resetInv, "output")
	b.Connect("halted", haltInv, "input")
	b.Connect("running", haltInv, "output")
	b.Connect("clock", gate, "input_1")
	b.Connect("running", gate, "input_2")
	b.Connect("running_clock", gate, "output")
	return b.Route()
}

var _ = bitmap.Wire
