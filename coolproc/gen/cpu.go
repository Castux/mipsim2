package main

// The coolproc processor (proc.md) as NOR gates, for the gate matrix.
//
// The control block (instruction register, step counter, decoding) sits on
// top, then 16 bit slices of the datapath, bit 15 first. Every register
// bit is a gated D latch of four NOR gates, and data only ever drives
// transistor gates. The bus is one NOR gate per bit, pulled up, with one
// pull-down per driver, so it reads all ones when nothing drives it.
//
// Signals named ..._inverted are the complement of the signal without the
// suffix. Many control lines exist only inverted, because that is what the
// gates they drive need: a latch takes its write line inverted, a bus
// driver its read line.
//
// The step counter is a Johnson counter of six master-slave flip-flops:
// twelve states per instruction, in four steps of three states (route,
// route and write, route):
//
//	states 0-2   fetching                  write_instruction in state 1
//	states 3-5   incrementing              write_temporary in state 4
//	states 6-8   executing                 write_result in state 7
//	states 9-11  updating_program_counter  write_program_counter in state 10

import (
	"fmt"

	"github.com/Castux/mipsim2/coolproc/isa"
)

// processor is the design, with what building it needs to remember.
type processor struct {
	*Design
	complements  map[string]string // a signal's complement, both ways
	controlBlock int
	bitBlocks    [16]int // the block of each datapath bit
}

// bitSignal names bit i of a group of signals: name_i, which the editor
// reads as one number.
func bitSignal(name string, i int) string { return fmt.Sprintf("%s_%d", name, i) }

// instructionBit is the instruction register's line for word bit n, true or
// inverted.
func instructionBit(n int, set bool) string {
	if set {
		return bitSignal("instruction", n)
	}
	return bitSignal("instruction_inverted", n)
}

// Opcode bits the control reads directly, both ways.
var (
	modeLow          = instructionBit(8+isa.ModeLowBit, true) // not A, carry in, or shift right
	modeLowInverted  = instructionBit(8+isa.ModeLowBit, false)
	modeHigh         = instructionBit(8+isa.ModeHighBit, true) // not B
	modeHighInverted = instructionBit(8+isa.ModeHighBit, false)
)

// registerNames are the registers' names, in instruction order.
var registerNames = func() []string {
	var names []string
	for _, r := range isa.Registers {
		names = append(names, r.String())
	}
	return names
}()

// recordComplements notes that a and b are each other's complement.
func (p *processor) recordComplements(a, b string) {
	p.complements[a], p.complements[b] = b, a
}

// complement returns a signal's complement, adding an inverter if there is
// none yet.
func (p *processor) complement(block int, unit, signal string) string {
	if c, ok := p.complements[signal]; ok {
		return c
	}
	inverted := signal + "_inverted"
	if s := p.Signal(signal); s.driver != nil {
		unit = s.Unit // next to the signal it inverts
	}
	p.Not(block, unit, inverted, signal)
	p.recordComplements(signal, inverted)
	return inverted
}

// andGate makes output = AND(literals): a NOR of their complements.
func (p *processor) andGate(block int, unit, output string, literals ...string) string {
	var inputs []string
	for _, l := range literals {
		inputs = append(inputs, p.complement(block, unit, l))
	}
	p.NOR(block, unit, output, inputs...)
	return output
}

// latchSignals are the names of a latch's signals.
type latchSignals struct {
	value, inverted     string
	storeOne, storeZero string // data and write, not data and write
}

// latchNames names a latch's signals after base, with bit i (or none when
// i is negative): base_i, base_inverted_i, base_store_one_i...
func latchNames(base string, i int) latchSignals {
	name := func(suffix string) string {
		if i < 0 {
			return base + suffix
		}
		return bitSignal(base+suffix, i)
	}
	return latchSignals{value: name(""), inverted: name("_inverted"), storeOne: name("_store_one"), storeZero: name("_store_zero")}
}

// latch is a gated D latch: its value follows data while the write line is
// high (given inverted), and holds when it is low. reset_buffered clears
// it.
func (p *processor) latch(block int, unit string, l latchSignals, data, dataInverted, writeInverted string) {
	p.NOR(block, unit, l.storeOne, dataInverted, writeInverted)
	p.NOR(block, unit, l.storeZero, data, writeInverted)
	p.NOR(block, unit, l.value, l.storeZero, l.inverted, "reset_buffered")
	p.NOR(block, unit, l.inverted, l.storeOne, l.value)
	p.recordComplements(l.value, l.inverted)
}

// sumOfProducts builds outputInverted = NOR(products), and output = its
// inverse when output is not empty. A product is a list of literals (an
// AND); a single literal is used as it is.
func (p *processor) sumOfProducts(block int, unit, output, outputInverted string, products ...[]string) {
	base := output
	if base == "" {
		base = outputInverted
	}
	var terms []string
	for k, product := range products {
		if len(product) == 1 {
			terms = append(terms, product[0])
			continue
		}
		terms = append(terms, p.andGate(block, unit, fmt.Sprintf("%s_term_%d", base, k+1), product...))
	}
	p.NOR(block, unit, outputInverted, terms...)
	if output != "" {
		p.Not(block, unit, output, outputInverted)
		p.recordComplements(output, outputInverted)
	}
}

// opcodeLiterals are the literals that match an opcode pattern (word bits
// 15 to 8).
func opcodeLiterals(pattern isa.Pattern) []string {
	var literals []string
	for k, bit := range pattern {
		switch bit {
		case '0':
			literals = append(literals, instructionBit(15-k, false))
		case '1':
			literals = append(literals, instructionBit(15-k, true))
		}
	}
	return literals
}

// registerFieldLiterals are the literals that match register number r in
// the three-bit field starting at word bit first.
func registerFieldLiterals(first, r int) []string {
	var literals []string
	for k := range 3 {
		literals = append(literals, instructionBit(first+k, r>>k&1 == 1))
	}
	return literals
}

func concatenate(parts ...[]string) []string {
	var all []string
	for _, part := range parts {
		all = append(all, part...)
	}
	return all
}

// BuildProcessor returns the processor design.
func BuildProcessor() *processor {
	p := &processor{Design: NewDesign(), complements: map[string]string{}}
	// Units, left to right: their lines stay together.
	for _, u := range []string{"instruction_register", "immediate", "register_c", "register_d", "register_x", "register_y",
		"bus", "zero_detector", "register_a", "register_b", "operand_selection", "logic", "adder", "result_selection",
		"shifter", "program_counter", "incrementer", "temporary_register", "control"} {
		p.Unit(u)
	}
	p.controlBlock = p.Block("control")
	p.Fold(p.controlBlock)
	p.NoTap("control")
	p.NoTap("instruction_register")
	for i := 15; i >= 0; i-- {
		p.bitBlocks[i] = p.Block(fmt.Sprintf("bit_%d", i))
	}
	p.External("clock", "control")
	p.External("reset", "control")
	// Reset, buffered so that its row can reach every unit with a tap.
	p.Not(p.controlBlock, "control", "reset_inverted", "reset")
	p.Not(p.controlBlock, "program_counter", "reset_buffered", "reset_inverted")
	p.datapath()
	p.control()
	return p
}

func (p *processor) datapath() {
	for i := range 16 {
		block := p.bitBlocks[i]
		bus, busInverted := bitSignal("bus", i), bitSignal("bus_inverted", i)
		p.Not(block, "bus", busInverted, bus)
		p.recordComplements(bus, busInverted)

		// Registers on the bus. The program counter is written from the bus
		// but never drives it; its value addresses the program memory.
		var busDrivers []string
		for _, r := range append(append([]string{}, registerNames...), "program_counter") {
			unit := "register_" + r
			half := "low"
			if i >= 8 {
				half = "high"
			}
			write := "write_" + r + "_" + half + "_inverted"
			if r == "program_counter" {
				unit, write = r, "write_program_counter_inverted"
			}
			p.latch(block, unit, latchNames(unit, i), bus, busInverted, write)
			if r != "program_counter" {
				pullsLow := bitSignal(unit+"_pulls_bus_low", i)
				p.NOR(block, unit, pullsLow, r+"_to_bus_inverted", bitSignal(unit, i)) // read, and the bit is 0
				busDrivers = append(busDrivers, pullsLow)
			}
		}

		// The instruction register sits in the control block, which
		// decodes it: the program memory's data pins have no position.
		fromMemory := bitSignal("instruction_from_memory", i)
		p.External(fromMemory, "instruction_register")
		fromMemoryInverted := bitSignal("instruction_from_memory_inverted", i)
		p.Not(p.controlBlock, "instruction_register", fromMemoryInverted, fromMemory)
		p.recordComplements(fromMemory, fromMemoryInverted)
		p.latch(p.controlBlock, "instruction_register", latchNames("instruction", i), fromMemory, fromMemoryInverted, "write_instruction_inverted")

		// Immediates: setl puts the argument byte on bus bits 0-7 and
		// zeros on bits 8-15; seth puts it on bits 8-15.
		pullsLow := bitSignal("immediate_pulls_bus_low", i)
		if i < 8 {
			p.NOR(block, "immediate", pullsLow, "immediate_low_to_bus_inverted", instructionBit(i, true))
			busDrivers = append(busDrivers, pullsLow)
		} else {
			busDrivers = append(busDrivers, "immediate_low_to_bus") // zeros up here
			p.NOR(block, "immediate", pullsLow, "immediate_high_to_bus_inverted", instructionBit(i-8, true))
			busDrivers = append(busDrivers, pullsLow)
		}

		// The program counter plus one, into the temporary register.
		plusOne, plusOneInverted := p.incrementerBit(i)
		temporary := latchNames("temporary_register", i)
		p.latch(block, "temporary_register", temporary, plusOne, plusOneInverted, "write_temporary_inverted")
		temporaryPullsLow := bitSignal("temporary_register_pulls_bus_low", i)
		p.NOR(block, "temporary_register", temporaryPullsLow, "temporary_to_bus_inverted", temporary.value)
		busDrivers = append(busDrivers, temporaryPullsLow, p.aluBit(i))

		// The bus.
		p.NOR(block, "bus", bus, busDrivers...)

		// Zero detection, chained from bit 0 up: bus_nonzero_up_to_i says
		// whether bus bits 0 to i are not all zero.
		if i > 0 {
			below := bitSignal("bus_nonzero_up_to", i-1)
			if i == 1 {
				below = bitSignal("bus", 0)
			}
			nonzero, zero := bitSignal("bus_nonzero_up_to", i), bitSignal("bus_zero_up_to", i)
			p.NOR(block, "zero_detector", zero, bitSignal("bus", i), below)
			p.Not(block, "zero_detector", nonzero, zero)
			p.recordComplements(nonzero, zero)
		}
	}
	p.shifter()
}

// incrementerBit adds the carry from the bit below to bit i of the program
// counter (the carry into bit 0 is 1). It returns the sum, both ways.
func (p *processor) incrementerBit(i int) (string, string) {
	block := p.bitBlocks[i]
	counter, counterInverted := bitSignal("program_counter", i), bitSignal("program_counter_inverted", i)
	if i == 0 {
		return counterInverted, counter
	}
	carry, carryInverted := bitSignal("program_counter", 0), bitSignal("program_counter_inverted", 0)
	if i > 1 {
		carry, carryInverted = bitSignal("incrementer_carry", i), bitSignal("incrementer_carry_inverted", i)
	}
	bothZero := bitSignal("incrementer_both_zero", i)
	p.NOR(block, "incrementer", bothZero, counter, carry)
	carryOut := bitSignal("incrementer_carry", i+1)
	p.NOR(block, "incrementer", carryOut, counterInverted, carryInverted) // both one
	if i < 15 {
		p.Not(block, "incrementer", bitSignal("incrementer_carry_inverted", i+1), carryOut)
	}
	sum, sumInverted := bitSignal("program_counter_plus_one", i), bitSignal("program_counter_plus_one_inverted", i)
	p.NOR(block, "incrementer", sum, carryOut, bothZero) // neither both one nor both zero
	p.Not(block, "incrementer", sumInverted, sum)
	return sum, sumInverted
}

// aluBit builds bit i of the ALU and returns its bus driver.
func (p *processor) aluBit(i int) string {
	block := p.bitBlocks[i]
	bit := func(name string) string { return bitSignal(name, i) }
	// The operands, inverted by their mode: operand = register xor invert.
	for _, register := range []string{"a", "b"} {
		invert, invertInverted := "invert_a", "invert_a_inverted"
		if register == "b" {
			invert, invertInverted = modeHigh, modeHighInverted // not B, for every operation
		}
		operand := "operand_" + register
		value, valueInverted := bit("register_"+register), bit("register_"+register+"_inverted")
		kept, flipped := bit(operand+"_kept"), bit(operand+"_flipped")
		p.NOR(block, "operand_selection", kept, valueInverted, invert)    // 1 and not inverting
		p.NOR(block, "operand_selection", flipped, value, invertInverted) // 0 and inverting
		p.NOR(block, "operand_selection", bit(operand+"_inverted"), kept, flipped)
		p.Not(block, "operand_selection", bit(operand), bit(operand+"_inverted"))
	}
	and, nor := bit("and"), bit("nor")
	p.NOR(block, "logic", and, bit("operand_a_inverted"), bit("operand_b_inverted"))
	p.NOR(block, "logic", nor, bit("operand_a"), bit("operand_b"))
	p.NOR(block, "logic", bit("xor"), and, nor)
	p.Not(block, "logic", bit("xor_inverted"), bit("xor"))
	p.Not(block, "logic", bit("and_inverted"), and)

	// Full adder, the carry from bit i-1 (for bit 0, the carry in mode).
	carryIn, carryInInverted := modeLow, modeLowInverted
	if i > 0 {
		carryIn, carryInInverted = bitSignal("carry_out", i-1), bitSignal("carry_out_inverted", i-1)
	}
	xorAndCarry, neither := bit("sum_xor_and_carry"), bit("sum_neither")
	p.NOR(block, "adder", xorAndCarry, bit("xor_inverted"), carryInInverted)
	p.NOR(block, "adder", neither, bit("xor"), carryIn)
	p.NOR(block, "adder", bit("sum"), xorAndCarry, neither)
	p.Not(block, "adder", bit("sum_inverted"), bit("sum"))
	p.NOR(block, "adder", bit("carry_out_inverted"), and, xorAndCarry)
	p.Not(block, "adder", bit("carry_out"), bit("carry_out_inverted"))

	// The result: the selected function's value. A term is the AND of a
	// select line and a value, as a NOR of their complements.
	terms := []struct{ name, selectInverted, valueInverted string }{
		{"and", "select_and_inverted", bit("and_inverted")},
		{"or", "select_or_inverted", nor}, // nor is or, inverted
		{"xor", "select_xor_inverted", bit("xor_inverted")},
		{"add", "select_add_inverted", bit("sum_inverted")},
		{"shift", "select_shift_inverted", bitSignal("shifted_by_8_inverted", i)},
	}
	var picked []string
	for _, t := range terms {
		name := bit("result_" + t.name)
		p.NOR(block, "result_selection", name, t.selectInverted, t.valueInverted)
		picked = append(picked, name)
	}
	p.NOR(block, "result_selection", bit("result_inverted"), picked...)
	p.Not(block, "result_selection", bit("result"), bit("result_inverted"))
	pullsLow := bit("alu_pulls_bus_low")
	p.NOR(block, "result_selection", pullsLow, "alu_to_bus_inverted", bit("result"))
	return pullsLow
}

// shifter is a barrel shifter of A by B[3:0], left or right (the low mode
// bit), over positions -1 to 16, so that the last bit shifted out lands in
// position 16 (left) or -1 (right): the shift's carry. Each stage computes
// the complement of its output.
func (p *processor) shifter() {
	position := func(k int) string {
		switch k {
		case -1:
			return "below_0"
		case 16:
			return "above_15"
		}
		return fmt.Sprint(k)
	}
	stageOutput := func(stage, k int) string { // inverted
		return fmt.Sprintf("shifted_by_%d_inverted_%s", 1<<stage, position(k))
	}
	blockOf := func(k int) int { return p.bitBlocks[max(0, min(15, k))] }
	for stage := range 4 {
		block := p.bitBlocks[stage] // where bit stage of B is
		amount := fmt.Sprint(1 << stage)
		amountInverted := bitSignal("register_b_inverted", stage)
		p.NOR(block, "shifter", "shift_left_by_"+amount, amountInverted, modeLow)
		p.NOR(block, "shifter", "shift_right_by_"+amount, amountInverted, modeLowInverted)
		p.Not(block, "shifter", "shift_left_by_"+amount+"_inverted", "shift_left_by_"+amount)
		p.Not(block, "shifter", "shift_right_by_"+amount+"_inverted", "shift_right_by_"+amount)
	}
	// input is the complement of a stage's input at position k, or "" for
	// a constant zero.
	input := func(stage, k int) string {
		if stage == 0 {
			if k < 0 || k > 15 {
				return ""
			}
			return bitSignal("register_a_inverted", k)
		}
		if k < -1 || k > 16 {
			return ""
		}
		return stageOutput(stage-1, k)
	}
	for stage := range 4 {
		amount := 1 << stage
		for k := -1; k <= 16; k++ {
			block := blockOf(k)
			var terms []string
			term := func(name, selectInverted, valueInverted string) {
				if valueInverted == "" {
					return
				}
				name = fmt.Sprintf("shift_by_%d_%s_%s", amount, name, position(k))
				p.NOR(block, "shifter", name, selectInverted, valueInverted)
				terms = append(terms, name)
			}
			term("from_below", fmt.Sprintf("shift_left_by_%d_inverted", amount), input(stage, k-amount))
			term("from_above", fmt.Sprintf("shift_right_by_%d_inverted", amount), input(stage, k+amount))
			term("keep", bitSignal("register_b", stage), input(stage, k)) // when bit stage of B is 0
			p.NOR(block, "shifter", stageOutput(stage, k), terms...)
		}
	}
}

func (p *processor) control() {
	block, unit := p.controlBlock, "control"
	// The clock, stopped once the processor has halted.
	p.Not(block, unit, "clock_inverted", "clock")
	p.NOR(block, unit, "running_clock", "clock_inverted", "halted")
	p.Not(block, unit, "running_clock_inverted", "running_clock")
	// The step counter: stage 0 takes stage 5 inverted, the others shift.
	// Master latches are open while the clock is low, slaves while it is
	// high.
	for k := range 6 {
		data, dataInverted := bitSignal("step", k-1), bitSignal("step_inverted", k-1)
		if k == 0 {
			data, dataInverted = "step_inverted_5", "step_5"
		}
		master := latchNames("step_master", k)
		p.latch(block, unit, master, data, dataInverted, "running_clock")
		p.latch(block, unit, latchNames("step", k), master.value, master.inverted, "running_clock_inverted")
	}
	stage := func(k int, set bool) string {
		if set {
			return bitSignal("step", k)
		}
		return bitSignal("step_inverted", k)
	}
	// Steps, and the write state in the middle of each.
	p.andGate(block, unit, "fetching", stage(2, false), stage(5, false))
	p.andGate(block, unit, "incrementing", stage(2, true), stage(5, false))
	p.andGate(block, unit, "executing", stage(2, true), stage(5, true))
	p.andGate(block, unit, "updating_program_counter", stage(2, false), stage(5, true))
	p.andGate(block, unit, "write_instruction", stage(0, true), stage(1, false))
	p.andGate(block, unit, "write_temporary", stage(3, true), stage(4, false))
	p.andGate(block, unit, "write_result", stage(0, false), stage(1, true))
	p.andGate(block, unit, "write_program_counter", stage(3, false), stage(4, true))
	p.Not(block, "instruction_register", "write_instruction_inverted", "write_instruction")
	p.Not(block, "temporary_register", "write_temporary_inverted", "write_temporary")
	p.Not(block, "program_counter", "write_program_counter_inverted", "write_program_counter")
	p.recordComplements("write_instruction", "write_instruction_inverted")
	p.recordComplements("write_temporary", "write_temporary_inverted")
	p.recordComplements("write_program_counter", "write_program_counter_inverted")

	// Instruction kinds.
	for _, k := range []struct {
		name    string
		pattern isa.Pattern
	}{
		{"is_arithmetic", isa.ArithmeticPattern}, {"is_jump", isa.JumpPattern}, {"is_jump_on_carry", isa.JumpOnCarryPattern},
		{"is_halt", isa.HaltPattern}, {"is_move", isa.MovePattern}, {"is_load", isa.LoadPattern}, {"is_store", isa.StorePattern},
		{"is_set_low", isa.SetLowPattern}, {"is_set_high", isa.SetHighPattern},
	} {
		p.andGate(block, unit, k.name, opcodeLiterals(k.pattern)...)
	}
	p.sumOfProducts(block, unit, "reads_source", "reads_source_inverted", []string{"is_move"}, []string{"is_store"}, []string{"is_jump"})
	p.sumOfProducts(block, unit, "writes_destination", "writes_destination_inverted", []string{"is_move"}, []string{"is_load"})

	p.carry()
	p.jumpAndHalt()
	p.registerControl()

	// Bus and memory routing.
	p.sumOfProducts(block, "temporary_register", "", "temporary_to_bus_inverted", []string{"updating_program_counter", "jump_taken_inverted"})
	p.sumOfProducts(block, "immediate", "immediate_low_to_bus", "immediate_low_to_bus_inverted", []string{"executing", "is_set_low"})
	p.sumOfProducts(block, "immediate", "", "immediate_high_to_bus_inverted", []string{"executing", "is_set_high"})
	p.andGate(block, "bus", "memory_read", "executing", "is_load")
	p.andGate(block, "bus", "memory_write", "write_result", "is_store")
	p.sumOfProducts(block, "result_selection", "", "alu_to_bus_inverted", []string{"executing", "is_arithmetic"})

	// The ALU's controls, from the opcode alone. Invert A is the low mode
	// bit, except for add (operation 000), where that bit is the carry in.
	p.sumOfProducts(block, "operand_selection", "invert_a", "invert_a_inverted",
		opcodeLiterals("xx1xxx1x"), opcodeLiterals("xxx1xx1x"), opcodeLiterals("xxxx1x1x"))
	for _, s := range []struct {
		name      string
		operation isa.Operation
	}{{"add", isa.Add}, {"shift", isa.Shift}, {"and", isa.And}, {"or", isa.Or}, {"xor", isa.Xor}} {
		p.sumOfProducts(block, "result_selection", "", "select_"+s.name+"_inverted", opcodeLiterals(isa.OperationPattern(s.operation)))
	}

	// The program memory is always reading: constants for its enables.
	p.NOR(block, "program_counter", "program_memory_read")
	p.Not(block, "program_counter", "program_memory_write", "program_memory_read")
}

// carry builds the carry register: written in the write state of add and
// shift, from the adder's carry out or the last bit shifted out.
func (p *processor) carry() {
	block := p.controlBlock
	p.NOR(block, "shifter", "shift_carry_left", modeLow, "shifted_by_8_inverted_above_15")         // left: position 16
	p.NOR(block, "shifter", "shift_carry_right", modeLowInverted, "shifted_by_8_inverted_below_0") // right: position -1
	p.NOR(block, "shifter", "shift_carry_inverted", "shift_carry_left", "shift_carry_right")
	p.NOR(block, "adder", "carry_from_add", "select_add_inverted", "carry_out_inverted_15")
	p.NOR(block, "adder", "carry_from_shift", "select_shift_inverted", "shift_carry_inverted")
	p.NOR(block, "adder", "carry_input_inverted", "carry_from_add", "carry_from_shift")
	p.Not(block, "adder", "carry_input", "carry_input_inverted")
	addOrShift := opcodeLiterals("xx00xxxx") // operation 000 or 001
	p.sumOfProducts(block, "adder", "", "write_carry_inverted", concatenate([]string{"write_result", "is_arithmetic"}, addOrShift))
	p.latch(block, "adder", latchNames("carry", -1), "carry_input", "carry_input_inverted", "write_carry_inverted")
}

// jumpAndHalt builds the jump decision, latched in the write state of every
// instruction (while the source register is on the bus), and the halt.
func (p *processor) jumpAndHalt() {
	block, unit := p.controlBlock, "control"
	condition := func(c isa.Condition) []string { return opcodeLiterals(isa.ConditionPattern(c)) }
	p.recordComplements("bus_zero_up_to_15", "bus_nonzero_up_to_15")
	p.sumOfProducts(block, unit, "jump_condition", "jump_condition_inverted",
		concatenate([]string{"is_jump"}, condition(isa.Always)),
		concatenate([]string{"is_jump", "bus_zero_up_to_15"}, condition(isa.IfZero)),
		concatenate([]string{"is_jump", "bus_nonzero_up_to_15"}, condition(isa.IfNotZero)),
		concatenate([]string{"is_jump", "bus_15"}, condition(isa.IfNegative)),
		concatenate([]string{"is_jump_on_carry"}, condition(isa.Always)),
		concatenate([]string{"is_jump_on_carry", "carry_inverted"}, condition(isa.IfCarryClear)),
		concatenate([]string{"is_jump_on_carry", "carry"}, condition(isa.IfCarrySet)),
	)
	p.complement(block, unit, "write_result")
	p.latch(block, unit, latchNames("jump_taken", -1), "jump_condition", "jump_condition_inverted", "write_result_inverted")

	// Halt: set in the write state of a halt, cleared only by reset (the
	// step counter stops there, so the write state lasts).
	p.complement(block, unit, "is_halt")
	p.NOR(block, unit, "halt_now", "is_halt_inverted", "write_result_inverted")
	p.NOR(block, unit, "halted", "halted_inverted", "reset_buffered")
	p.NOR(block, unit, "halted_inverted", "halt_now", "halted")
	p.recordComplements("halted", "halted_inverted")
}

// registerControl builds each register's read and write lines. The enables
// and the decoded register fields are computed once; each register's unit
// reads them through taps.
func (p *processor) registerControl() {
	block, unit := p.controlBlock, "control"
	p.andGate(block, unit, "read_source_now", "executing", "reads_source")
	p.andGate(block, unit, "write_destination_now", "write_result", "writes_destination")
	p.andGate(block, unit, "write_set_low_now", "write_result", "is_set_low")
	p.andGate(block, unit, "write_set_high_now", "write_result", "is_set_high")
	p.andGate(block, unit, "write_alu_to_x_now", concatenate([]string{"write_result", "is_arithmetic"}, opcodeLiterals(isa.ResultToXPattern))...)
	p.andGate(block, unit, "write_alu_to_y_now", concatenate([]string{"write_result", "is_arithmetic"}, opcodeLiterals(isa.ResultToYPattern))...)
	p.andGate(block, unit, "jump_target_to_bus_now", "updating_program_counter", "jump_taken")
	for number, r := range registerNames {
		registerUnit := "register_" + r
		p.andGate(block, unit, "source_is_"+r, registerFieldLiterals(isa.SourceFieldBit, number)...)
		p.andGate(block, unit, "destination_is_"+r, registerFieldLiterals(isa.DestinationFieldBit, number)...)
		p.andGate(block, unit, "set_target_is_"+r, registerFieldLiterals(isa.SetRegisterFieldBit, number)...)
		reads := []string{p.andGate(block, registerUnit, r+"_read_as_source", "read_source_now", "source_is_"+r)}
		if r == "y" {
			reads = append(reads, "jump_target_to_bus_now")
		}
		p.NOR(block, registerUnit, r+"_to_bus_inverted", reads...)
		both := []string{
			p.andGate(block, registerUnit, r+"_written_as_destination", "write_destination_now", "destination_is_"+r),
			p.andGate(block, registerUnit, r+"_written_by_set_low", "write_set_low_now", "set_target_is_"+r),
		}
		switch r {
		case "x":
			both = append(both, "write_alu_to_x_now")
		case "y":
			both = append(both, "write_alu_to_y_now")
		}
		high := append(append([]string{}, both...), p.andGate(block, registerUnit, r+"_written_by_set_high", "write_set_high_now", "set_target_is_"+r))
		p.NOR(block, registerUnit, "write_"+r+"_low_inverted", both...)
		p.NOR(block, registerUnit, "write_"+r+"_high_inverted", high...)
	}
}

// transistors counts the design's transistors: one per gate input.
func (p *processor) transistors() int {
	n := 0
	for _, g := range p.gates {
		n += len(g.Inputs)
	}
	return n
}
