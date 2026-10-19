package main

import (
	"encoding/json"
	"fmt"
	"image"

	"github.com/Castux/mipsim2/coolproc/isa"

	"github.com/Castux/mipsim2/bitmap"
	"github.com/Castux/mipsim2/doc"
)

// processor builds the whole processor at the document's root: the control
// blocks on top, the datapath units side by side below, with the bus and
// the other lanes running straight through them.
func (l *Library) processor() *Def {
	reg := func(name string, s slice) *Def { return l.registerUnit(name, s) }
	units := []struct {
		name string
		def  *Def
	}{
		{"immediate", l.immediate()},
		{"register_c", reg("general_register", slice{name: "general_register_bit", driver: true})},
		{"register_d", reg("general_register", slice{name: "general_register_bit", driver: true})},
		{"register_a", reg("register_a", slice{name: "register_a_bit", driver: true, output: lane1})},
		{"register_b", reg("register_b", slice{name: "register_b_bit", driver: true, output: lane2, pass: []int{lane1}})},
		{"shifter", l.shifter()},
		{"alu", l.alu()},
		{"register_x", reg("register_x", slice{name: "register_x_bit", driver: true, selectAlt: lane3, pass: []int{lane3}})},
		{"register_y", reg("register_y", slice{name: "register_y_bit", driver: true, selectAlt: lane3})},
		{"program_counter", reg("program_counter", slice{name: "program_counter_bit", output: lane1, whole: true})},
		{"incrementer", l.incrementer()},
		{"temporary_register", reg("temporary_register", slice{name: "temporary_register_bit", fromLane: lane2, driver: true, whole: true})},
		{"bus_end", l.busEnd()},
	}
	const gap = 16
	width := 40
	for _, u := range units {
		width += u.def.W + gap
	}
	controlH := 1100
	b := l.build("top", width+40, controlH+unitHeight+60)
	b.root = true
	dpY := controlH
	at := map[string]*Inst{}
	x := 40
	for _, u := range units {
		at[u.name] = b.Place(u.def, u.name, x, dpY)
		x += u.def.W + gap
	}

	// Control blocks, in two rows.
	place := func(d *Def, name string, x, y int) *Inst {
		in := b.Place(d, name, x, y)
		at[name] = in
		return in
	}
	y1 := 40
	place(l.clockControl(), "clock_control", 40, y1)
	place(l.stepCounter(), "step_counter", 180, y1)
	place(l.stepDecoder(), "step_decoder", 1260, y1)
	place(l.instructionRegister(), "instruction_register", 1750, y1)
	place(l.statusRegisters(), "status_registers", 3000, y1)
	y2 := 420
	place(l.opcodeDecoder(), "opcode_decoder", 40, y2)
	fx := 40 + l.opcodeDecoder().W + 60
	for _, f := range []string{"source", "destination", "set_target"} {
		place(l.registerFieldDecoder(), f+"_decoder", fx, y2)
		fx += l.registerFieldDecoder().W + 40
	}
	place(l.controlSignals(), "control_signals", fx, y2)
	fx += l.controlSignals().W + 60
	for _, r := range registerNames {
		place(l.registerControl(), "register_"+r+"_control", fx, y2)
		fx += l.registerControl().W + 30
	}
	place(l.jumpDecision(), "jump_decision", fx, y2)

	// Lanes between neighbouring units. A unit joins its own left and
	// right ports of a lane inside.
	lanes := []struct {
		net   string
		ports [][2]string // unit, port prefix (port is prefix_i_left/right)
	}{
		{"bus", nil}, // every unit
		{"operand_a", [][2]string{{"register_a", "value"}, {"register_b", "operand_a"}, {"shifter", "operand_a"}, {"alu", "operand_a"}}},
		{"operand_b", [][2]string{{"register_b", "value"}, {"shifter", "operand_b"}, {"alu", "operand_b"}}},
		{"shifted", [][2]string{{"shifter", "shifted"}, {"alu", "shifted"}}},
		{"alu_result", [][2]string{{"alu", "result"}, {"register_x", "alu_result"}, {"register_y", "alternative"}}},
		{"program_counter_value", [][2]string{{"program_counter", "value"}, {"incrementer", "value"}}},
		{"program_counter_plus_one", [][2]string{{"incrementer", "plus_one"}, {"temporary_register", "data"}}},
	}
	for _, ln := range lanes {
		ports := ln.ports
		if ln.net == "bus" {
			for _, u := range units {
				ports = append(ports, [2]string{u.name, "bus"})
			}
		}
		for i := range 16 {
			net := fmt.Sprintf("%s_%d", ln.net, i)
			for _, p := range ports {
				in := at[p[0]]
				left, right := fmt.Sprintf("%s_%d_left", p[1], i), fmt.Sprintf("%s_%d_right", p[1], i)
				switch {
				case hasPort(in.Def, left) && hasPort(in.Def, right):
					b.Through(net, in, left, right)
				case hasPort(in.Def, left):
					b.Connect(net, in, left)
				case hasPort(in.Def, right):
					b.Connect(net, in, right)
				default:
					panic(fmt.Sprintf("%s has no %s or %s", p[0], left, right))
				}
			}
		}
	}

	// Control lines: connect(net, block, port).
	c := func(net, block, port string) { b.Connect(net, at[block], port) }
	// Clock, reset and the steps.
	c("clock", "clock_control", "clock")
	c("reset", "clock_control", "reset")
	c("halted", "clock_control", "halted")
	c("halted", "status_registers", "halted")
	c("running_clock", "clock_control", "running_clock")
	c("running_clock", "step_counter", "clock")
	for k := range 6 {
		for _, s := range []string{"", "_inverted"} {
			n := fmt.Sprintf("stage_%d%s", k, s)
			c(n, "step_counter", n)
			c(n, "step_decoder", n)
		}
	}
	for _, n := range []string{"fetching", "incrementing"} {
		c(n, "step_decoder", n) // shown, not needed elsewhere
	}
	c("executing", "step_decoder", "executing")
	c("executing", "control_signals", "executing")
	c("updating_program_counter", "step_decoder", "updating_program_counter")
	c("updating_program_counter", "control_signals", "updating_program_counter")
	c("write_result", "step_decoder", "write_result")
	c("write_result", "control_signals", "write_result")
	c("write_result", "status_registers", "write_result")
	c("write_instruction", "step_decoder", "write_instruction")
	c("write_instruction", "instruction_register", "write")
	c("write_temporary", "step_decoder", "write_temporary")
	c("write_temporary", "temporary_register", "write")
	c("write_program_counter", "step_decoder", "write_program_counter")
	c("write_program_counter", "program_counter", "write")
	// Reset everywhere.
	c("not_reset", "clock_control", "not_reset")
	for _, blk := range []string{"step_counter", "instruction_register", "status_registers", "register_a", "register_b", "register_c",
		"register_d", "register_x", "register_y", "program_counter", "temporary_register"} {
		c("not_reset", blk, "not_reset")
	}
	// The instruction.
	for i := range 16 {
		for _, s := range []string{"", "_inverted"} {
			n := fmt.Sprintf("instruction_%d%s", i, s)
			c(n, "instruction_register", n)
		}
	}
	for i := 8; i <= 15; i++ {
		for _, s := range []string{"", "_inverted"} {
			n := fmt.Sprintf("instruction_%d%s", i, s)
			c(n, "opcode_decoder", n)
		}
	}
	// The three register fields, each into its own decoder.
	for _, field := range []struct {
		decoder string
		first   int // the field's first bit in the instruction
	}{
		{"source_decoder", isa.SourceFieldBit},
		{"destination_decoder", isa.DestinationFieldBit},
		{"set_target_decoder", isa.SetRegisterFieldBit},
	} {
		for k := range 3 {
			for _, s := range []string{"", "_inverted"} {
				c(instructionBit(field.first+k)+s, field.decoder, fmt.Sprintf("field_%d%s", k, s))
			}
		}
	}
	for k := range 8 {
		c(fmt.Sprintf("instruction_%d_inverted", k), "immediate", fmt.Sprintf("argument_inverted_%d", k))
	}
	// Decoded instruction.
	for _, n := range []string{"is_move", "is_load", "is_store", "is_jump", "is_alu", "is_set_low", "is_set_high", "operation_add", "operation_shift"} {
		c(n, "opcode_decoder", n)
		c(n, "control_signals", n)
	}
	for _, n := range []string{"is_halt", "is_jump_on_carry", "operation_and", "operation_or", "operation_xor"} {
		c(n, "opcode_decoder", n)
	}
	c("is_halt", "status_registers", "is_halt")
	c("is_jump", "jump_decision", "is_jump")
	c("is_jump_on_carry", "jump_decision", "is_jump_on_carry")
	c(resultToY, "control_signals", resultToY)
	c(modeLow, "control_signals", modeLow)
	for _, n := range []string{conditionLow, conditionLow + "_inverted", conditionHigh, conditionHigh + "_inverted"} {
		c(n, "jump_decision", n)
	}
	// Registers.
	for _, r := range registerNames {
		ctl := "register_" + r + "_control"
		c("reads_source_now", "control_signals", "reads_source_now")
		c("reads_source_now", ctl, "reads_source_now")
		c("source_is_"+r, "source_decoder", "is_"+r)
		c("source_is_"+r, ctl, "source_is_this")
		c("destination_is_"+r, "destination_decoder", "is_"+r)
		c("destination_is_"+r, ctl, "destination_is_this")
		c("set_target_is_"+r, "set_target_decoder", "is_"+r)
		c("set_target_is_"+r, ctl, "set_target_is_this")
		for _, n := range []string{"write_destination", "write_set_low", "write_set_high"} {
			c(n, "control_signals", n)
			c(n, ctl, n)
		}
		switch r {
		case "x", "y":
			c("write_alu_to_"+r, "control_signals", "write_alu_to_"+r)
			c("write_alu_to_"+r, ctl, "write_alu_result")
			c("alu_to_"+r, "control_signals", "alu_to_"+r)
			c("alu_to_"+r, "register_"+r, "select")
		default:
			zero := "no_alu_result_for_" + r
			c(zero, ctl, "write_alu_result")
			b.Tie(zero, bitmap.Ground)
		}
		if r == "y" {
			c("jump_target_to_bus", "control_signals", "jump_target_to_bus")
			c("jump_target_to_bus", ctl, "extra_read")
		} else {
			zero := "no_extra_read_for_" + r
			c(zero, ctl, "extra_read")
			b.Tie(zero, bitmap.Ground)
		}
		c(r+"_to_bus", ctl, "to_bus")
		c(r+"_to_bus", "register_"+r, "read")
		c("write_"+r+"_low", ctl, "write_low")
		c("write_"+r+"_low", "register_"+r, "write_low")
		c("write_"+r+"_high", ctl, "write_high")
		c("write_"+r+"_high", "register_"+r, "write_high")
	}
	// Bus routing from the control table.
	for _, n := range []string{"immediate_low_to_bus", "immediate_high_to_bus"} {
		c(n, "control_signals", n)
		c(n, "immediate", n)
	}
	c("temporary_to_bus", "control_signals", "temporary_to_bus")
	c("temporary_to_bus", "temporary_register", "read")
	c("memory_read", "control_signals", "memory_read")
	c("memory_write", "control_signals", "memory_write")
	c("jump_taken", "status_registers", "jump_taken")
	c("jump_taken", "control_signals", "jump_taken")
	c("jump_taken_inverted", "status_registers", "jump_taken_inverted")
	// The ALU.
	c("invert_a", "control_signals", "invert_a")
	c("invert_a", "alu", "invert_a")
	c(modeHigh, "alu", "invert_b")
	c(modeLow, "alu", "carry_in_0")
	c(modeLow, "shifter", "shift_right")
	for _, op := range []string{"add", "shift", "and", "or", "xor"} {
		c("operation_"+op, "alu", "select_"+op)
	}
	c("operation_shift", "status_registers", "operation_shift")
	c("alu_carry", "alu", "carry_out_15")
	c("alu_carry", "status_registers", "alu_carry")
	c("shift_carry", "shifter", "carry")
	c("shift_carry", "status_registers", "shift_carry")
	c("write_carry", "control_signals", "write_carry")
	c("write_carry", "status_registers", "write_carry")
	c("carry", "status_registers", "carry")
	c("carry", "jump_decision", "carry")
	c("carry_inverted", "status_registers", "carry_inverted")
	c("carry_inverted", "jump_decision", "carry_inverted")
	// Jumps.
	c("bus_is_zero", "bus_end", "bus_is_zero")
	c("bus_is_zero", "jump_decision", "bus_is_zero")
	// Negative is bus bit 15, which the bus end also brings out on top.
	b.Through("bus_15", at["bus_end"], "bus_15_left", "bus_is_negative")
	c("bus_15", "jump_decision", "bus_is_negative")
	c("jump_condition", "jump_decision", "jump_condition")
	c("jump_condition", "status_registers", "jump_condition")
	// The program memory's enables: always reading.
	b.Constant("rom_read", bitmap.Power, image.Pt(1800, 10))
	b.Constant("rom_write", bitmap.Ground, image.Pt(1810, 10))
	return b.Route()
}

// memories are the processor's memories.
func memories(prog string) ([]doc.DeviceConfig, error) {
	var out []doc.DeviceConfig
	for _, m := range []map[string]any{
		{"kind": "memory", "name": "program", "addr": "program_counter.value", "data": "instruction_register.data", "read": "rom_read", "write": "rom_write", "words": 65536, "width": 16, "init": prog, "readonly": true},
		{"kind": "memory", "name": "data", "addr": "register_y.value", "data": "bus", "read": "memory_read", "write": "memory_write", "words": 65536, "width": 16},
	} {
		raw, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		dc, err := doc.NewDeviceConfig(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, dc)
	}
	return out, nil
}
