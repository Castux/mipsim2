package main

// The coolproc processor (proc.md) as NOR gates.
//
// Datapath: 16 bit slices, bit 15 at the top, under the control blocks.
// Every register bit is a gated D latch of four NOR gates; data only ever
// drives transistor gates. The bus is one NOR gate per bit, pulled up, with
// one pull-down per possible driver, so it reads as all ones when nothing
// drives it.
//
// Sequencer: a Johnson counter of 6 master-slave flip-flops gives 12 states
// per instruction, in 4 steps of 3 states: route, route + write, route.
//
//	step F  states 0-2   fetch: the ROM word into IR (write in state 1)
//	step I  states 3-5   PC+1 into TMP (write in state 4)
//	step E  states 6-8   execute (writes in state 7)
//	step P  states 9-11  TMP, or Y for a taken jump, into PC (write in 10)

import (
	"fmt"
	"strings"
)

// Register codes in instructions.
var regs = []string{"a", "b", "x", "y", "c", "d"}

type cpu struct {
	*Design
	comp map[string]string // complements: name -> its inverse
	ctl  int               // the control block
	bits [16]int           // block of each bit
}

func bitName(name string, i int) string { return fmt.Sprintf("%s_%d", name, i) }

// pair records that a and b are complements.
func (c *cpu) pair(a, b string) {
	c.comp[a], c.comp[b] = b, a
}

// inv returns the complement of a signal, making an inverter if needed.
func (c *cpu) inv(block int, unit, s string) string {
	if n, ok := c.comp[s]; ok {
		return n
	}
	n := s + "_n"
	if o := c.S(s); o.drv != nil {
		unit = o.Unit // next to the signal it inverts
	}
	c.Not(block, unit, n, s)
	c.pair(s, n)
	return n
}

// and is out = AND(lits): a NOR of their complements.
func (c *cpu) and(block int, unit, out string, lits ...string) string {
	var ins []string
	for _, l := range lits {
		ins = append(ins, c.inv(block, unit, l))
	}
	c.NOR(block, unit, out, ins...)
	return out
}

// latch is a gated D latch with reset: q follows d while w is high (given
// as d, dn and wn) and holds when w is low; rst clears it.
func (c *cpu) latch(block int, unit, q, qn, d, dn, wn string) {
	c.NOR(block, unit, q+"_s", dn, wn) // d and w
	c.NOR(block, unit, q+"_r", d, wn)  // not d and w
	c.NOR(block, unit, q, q+"_r", qn, "rst")
	c.NOR(block, unit, qn, q+"_s", q)
	c.pair(q, qn)
}

// sop builds a function as a sum of products: outn = NOR(products), and
// out = NOT outn when out is not empty. Each product is a list of literals.
func (c *cpu) sop(block int, unit, out, outn string, products ...[]string) {
	var terms []string
	for i, p := range products {
		name := fmt.Sprintf("%s_p%d", strings.TrimSuffix(outn, "_n"), i)
		if len(p) == 1 {
			terms = append(terms, p[0])
			continue
		}
		terms = append(terms, c.and(block, unit, name, p...))
	}
	c.NOR(block, unit, outn, terms...)
	if out != "" {
		c.Not(block, unit, out, outn)
		c.pair(out, outn)
	}
}

// ir returns the literal for instruction bit j (0..15), negated with "!".
func ir(j int, v bool) string {
	if v {
		return bitName("ir", j)
	}
	return bitName("ir_n", j)
}

// op returns literals matching opcode bits (o7..o0 = ir_15..ir_8) to a
// pattern such as "01xxxxx0", most significant first.
func op(pattern string) []string {
	var lits []string
	for k, ch := range pattern {
		j := 15 - k
		switch ch {
		case '0':
			lits = append(lits, ir(j, false))
		case '1':
			lits = append(lits, ir(j, true))
		}
	}
	return lits
}

// code returns the literals of a 3-bit register code in ir bits lo..lo+2.
func code(lo, r int) []string {
	var lits []string
	for k := range 3 {
		lits = append(lits, ir(lo+k, r>>k&1 == 1))
	}
	return lits
}

func cat(parts ...[]string) []string {
	var all []string
	for _, p := range parts {
		all = append(all, p...)
	}
	return all
}

// BuildCPU returns the processor design.
func BuildCPU() *cpu {
	c := &cpu{Design: NewDesign(), comp: map[string]string{}}
	for _, u := range []string{"ir", "imm", "c", "d", "x", "y", "bus", "zero", "a", "b", "alu_in", "logic", "adder", "result", "shift", "pc", "inc", "tmp", "ctl"} {
		c.Unit(u)
	}
	c.ctl = c.Block("control")
	c.Fold(c.ctl)
	c.NoTap("ctl")
	c.NoTap("ir")
	for i := 15; i >= 0; i-- {
		c.bits[i] = c.Block(fmt.Sprintf("bit%d", i))
	}
	c.Ext("clock", "ctl")
	c.Ext("reset", "ctl")
	// Reset, buffered so that its row can reach every unit with a tap.
	c.Not(c.ctl, "ctl", "reset_n", "reset")
	c.Not(c.ctl, "pc", "rst", "reset_n")
	c.datapath()
	c.control()
	return c
}

func (c *cpu) datapath() {
	for i := range 16 {
		b := c.bits[i]
		bus, busn := bitName("bus", i), bitName("bus_n", i)
		c.Not(b, "bus", busn, bus)
		c.pair(bus, busn)

		// Registers on the bus. PC is written from the bus but never
		// drives it; its outputs address the ROM.
		var drivers []string
		for _, r := range append(append([]string{}, regs...), "pc") {
			half := "lo"
			if i >= 8 {
				half = "hi"
			}
			wn := r + "_wn_" + half
			if r == "pc" {
				wn = "pc_wn"
			}
			q, qn := bitName(r, i), bitName(r+"_n", i)
			c.latch(b, r, q, qn, bus, busn, wn)
			if r != "pc" {
				t := bitName(r+"_t", i)
				c.NOR(b, r, t, r+"_rdn", q) // read and q low
				drivers = append(drivers, t)
			}
		}

		// Instruction register, from the ROM.
		rom := bitName("rom", i)
		c.Ext(rom, "ir")
		// It sits in the control block, which decodes it: the ROM's data
		// pins have no position.
		romn := c.inv(c.ctl, "ir", rom)
		c.latch(c.ctl, "ir", bitName("ir", i), bitName("ir_n", i), rom, romn, "ir_wn")

		// Immediates: setl puts ir[7:0] on bus[7:0] and zeros on
		// bus[15:8]; seth puts ir[7:0] on bus[15:8].
		if i < 8 {
			t := bitName("imm_t", i)
			c.NOR(b, "imm", t, "imm_lo_n", bitName("ir", i))
			drivers = append(drivers, t)
		} else {
			drivers = append(drivers, "imm_lo")
			t := bitName("imm_t", i)
			c.NOR(b, "imm", t, "imm_hi_n", bitName("ir", i-8))
			drivers = append(drivers, t)
		}

		// PC + 1 into TMP.
		pc, pcn := bitName("pc", i), bitName("pc_n", i)
		var inc, incn string
		switch i {
		case 0:
			inc, incn = pcn, pc
		default:
			cin, cinn := bitName("pc", 0), bitName("pc_n", 0)
			if i > 1 {
				cin, cinn = bitName("inc_c", i), bitName("inc_c_n", i)
			}
			x := bitName("inc_x", i)
			c.NOR(b, "inc", x, pc, cin) // both low
			co := bitName("inc_c", i+1)
			c.NOR(b, "inc", co, pcn, cinn) // both high: carry out
			if i < 15 {
				c.Not(b, "inc", bitName("inc_c_n", i+1), co)
			}
			inc, incn = bitName("inc", i), bitName("inc_n", i)
			c.NOR(b, "inc", inc, co, x)
			c.Not(b, "inc", incn, inc)
		}
		c.latch(b, "tmp", bitName("tmp", i), bitName("tmp_n", i), inc, incn, "tmp_wn")
		t := bitName("tmp_t", i)
		c.NOR(b, "tmp", t, "tmp_rdn", bitName("tmp", i))
		drivers = append(drivers, t)

		// ALU.
		drivers = append(drivers, c.aluBit(i))

		// The bus.
		c.NOR(b, "bus", bus, drivers...)

		// Zero detection, chained from bit 0 up: nz_i = bus_0..i not all zero.
		if i > 0 {
			prev := bitName("nz", i-1)
			if i == 1 {
				prev = bitName("bus", 0)
			}
			c.NOR(b, "zero", bitName("nz_n", i), bitName("bus", i), prev)
			c.Not(b, "zero", bitName("nz", i), bitName("nz_n", i))
			c.pair(bitName("nz", i), bitName("nz_n", i))
		}
	}
	c.shifter()
}

// aluBit builds bit i of the ALU and returns its bus driver.
func (c *cpu) aluBit(i int) string {
	b := c.bits[i]
	n := func(s string) string { return bitName(s, i) }
	// Operands, inverted on demand: a' = a xor ia, b' = b xor ib.
	for _, o := range []string{"a", "b"} {
		inv, invn := "ia", "ia_n"
		if o == "b" {
			inv, invn = "ir_10", "ir_n_10" // m1: not B, for every operation
		}
		g1, g2 := n(o+"op_g1"), n(o+"op_g2")
		c.NOR(b, "alu_in", g1, n(o+"_n"), inv)  // o and not inv
		c.NOR(b, "alu_in", g2, n(o), invn)      // not o and inv
		c.NOR(b, "alu_in", n(o+"op_n"), g1, g2) // not o'
		c.Not(b, "alu_in", n(o+"op"), n(o+"op_n"))
	}
	ap, apn, bp, bpn := n("aop"), n("aop_n"), n("bop"), n("bop_n")
	and, nor := n("and"), n("nor")
	c.NOR(b, "logic", and, apn, bpn)
	c.NOR(b, "logic", nor, ap, bp)
	c.NOR(b, "logic", n("xor"), and, nor)
	c.Not(b, "logic", n("xor_n"), n("xor"))
	c.Not(b, "logic", n("and_n"), and)

	// Full adder, carry in from bit i-1 (from the control for bit 0).
	cin, cinn := "ir_9", "ir_n_9"
	if i > 0 {
		cin, cinn = bitName("co", i-1), bitName("co_n", i-1)
	}
	k1, k2 := n("add_k1"), n("add_k2")
	c.NOR(b, "adder", k1, n("xor_n"), cinn) // xor and carry
	c.NOR(b, "adder", k2, n("xor"), cin)    // neither
	c.NOR(b, "adder", n("sum"), k1, k2)
	c.Not(b, "adder", n("sum_n"), n("sum"))
	c.NOR(b, "adder", n("co_n"), and, k1)
	c.Not(b, "adder", n("co"), n("co_n"))

	// Result: the selected function, then onto the bus.
	terms := []string{
		c.and0(b, n("res_and"), "sel_and_n", n("and_n")),
		c.and0(b, n("res_or"), "sel_or_n", nor),
		c.and0(b, n("res_xor"), "sel_xor_n", n("xor_n")),
		c.and0(b, n("res_add"), "sel_add_n", n("sum_n")),
		c.and0(b, n("res_sh"), "sel_sh_n", bitName("sh3_n", i)),
	}
	c.NOR(b, "result", n("res_n"), terms...)
	c.Not(b, "result", n("res"), n("res_n"))
	t := n("alu_t")
	c.NOR(b, "result", t, "alu_rdn", n("res"))
	return t
}

// and0 is a NOR of two complements in the result unit: an AND of their
// true forms.
func (c *cpu) and0(b int, out, xn, yn string) string {
	c.NOR(b, "result", out, xn, yn)
	return out
}

// shifter is a barrel shifter of A by B[3:0], left (ir_9 low) or right,
// over positions -1..16 so that the last bit shifted out lands in position
// 16 (left) or -1 (right). Each stage computes the complement of its
// output.
func (c *cpu) shifter() {
	pos := func(p int) string {
		switch p {
		case -1:
			return "lo"
		case 16:
			return "hi"
		}
		return fmt.Sprint(p)
	}
	block := func(p int) int { return c.bits[max(0, min(15, p))] }
	for k := range 4 {
		bk := c.bits[k]
		bkn := bitName("b_n", k)
		c.NOR(bk, "shift", bitName("shl", k), bkn, "ir_9")   // shift left by 2^k
		c.NOR(bk, "shift", bitName("shr", k), bkn, "ir_n_9") // shift right by 2^k
		c.Not(bk, "shift", bitName("shl_n", k), bitName("shl", k))
		c.Not(bk, "shift", bitName("shr_n", k), bitName("shr", k))
	}
	// in returns the complement of stage k's input at position q, or "" if
	// it is a constant zero.
	in := func(k, q int) string {
		if k == 0 {
			if q < 0 || q > 15 {
				return ""
			}
			return bitName("a_n", q)
		}
		if q < -1 || q > 16 {
			return ""
		}
		return fmt.Sprintf("sh%d_n_%s", k-1, pos(q))
	}
	for k := range 4 {
		s := 1 << k
		for p := -1; p <= 16; p++ {
			b := block(p)
			var terms []string
			add := func(name, sel, x string) {
				if x == "" {
					return
				}
				name = fmt.Sprintf("sh%d_%s_%s", k, name, pos(p))
				c.NOR(b, "shift", name, sel, x)
				terms = append(terms, name)
			}
			add("l", bitName("shl_n", k), in(k, p-s))
			add("r", bitName("shr_n", k), in(k, p+s))
			add("k", bitName("b", k), in(k, p)) // keep, when b[k] is low
			c.NOR(b, "shift", fmt.Sprintf("sh%d_n_%s", k, pos(p)), terms...)
		}
	}
}

func (c *cpu) control() {
	b := c.ctl
	// Clock gated by halt, and its complement.
	c.Not(b, "ctl", "clock_n", "clock")
	c.NOR(b, "ctl", "clk", "clock_n", "halted") // clock and not halted
	c.Not(b, "ctl", "clk_n", "clk")
	// Johnson counter: q0 takes not q5, q1..q5 shift. Master latches are
	// open while the clock is low, slaves while it is high.
	for i := range 6 {
		d, dn := bitName("q", i-1), bitName("q_n", i-1)
		if i == 0 {
			d, dn = "q_n_5", "q_5"
		}
		c.latch(b, "ctl", bitName("m", i), bitName("m_n", i), d, dn, "clk")
		c.latch(b, "ctl", bitName("q", i), bitName("q_n", i), bitName("m", i), bitName("m_n", i), "clk_n")
	}
	q := func(i int, v bool) string {
		if v {
			return bitName("q", i)
		}
		return bitName("q_n", i)
	}
	// Steps and write states.
	c.and(b, "ctl", "step_f", q(2, false), q(5, false))
	c.and(b, "ctl", "step_i", q(2, true), q(5, false))
	c.and(b, "ctl", "step_e", q(2, true), q(5, true))
	c.and(b, "ctl", "step_p", q(2, false), q(5, true))
	c.and(b, "ctl", "w_f", q(0, true), q(1, false))
	c.and(b, "ctl", "w_i", q(3, true), q(4, false))
	c.and(b, "ctl", "w_e", q(0, false), q(1, true))
	c.and(b, "ctl", "w_p", q(3, false), q(4, true))
	c.Not(b, "ctl", "ir_wn", "w_f")
	c.Not(b, "tmp", "tmp_wn", "w_i")
	c.Not(b, "pc", "pc_wn", "w_p")
	c.pair("w_f", "ir_wn")
	c.pair("w_i", "tmp_wn")
	c.pair("w_p", "pc_wn")

	// Instruction groups (opcode = ir[15:8]).
	c.and(b, "ctl", "is_alu", op("01")...)
	c.and(b, "ctl", "is_jump", op("00010")...)
	c.and(b, "ctl", "is_jumpc", op("00011")...)
	c.and(b, "ctl", "is_halt", op("00000000")...)
	c.and(b, "ctl", "is_move", op("00000001")...)
	c.and(b, "ctl", "is_load", op("00000010")...)
	c.and(b, "ctl", "is_store", op("00000011")...)
	c.sop(b, "ctl", "reads_src", "reads_src_n", []string{"is_move"}, []string{"is_store"}, []string{"is_jump"})
	c.sop(b, "ctl", "writes_dst", "writes_dst_n", []string{"is_move"}, []string{"is_load"})
	c.and(b, "ctl", "is_setl", cat(op("10"))...)
	c.and(b, "ctl", "is_seth", cat(op("11"))...)

	// Carry: written in state 7 of add and shift. Its input is the adder's
	// carry out or the last bit shifted out.
	c.NOR(b, "shift", "sh_c_l", "ir_9", "sh3_n_hi")   // left: position 16
	c.NOR(b, "shift", "sh_c_r", "ir_n_9", "sh3_n_lo") // right: position -1
	c.NOR(b, "shift", "sh_c_n", "sh_c_l", "sh_c_r")
	c.NOR(b, "adder", "carry_add", "sel_add_n", "co_n_15")
	c.NOR(b, "adder", "carry_sh", "sel_sh_n", "sh_c_n")
	c.NOR(b, "adder", "carry_d_n", "carry_add", "carry_sh")
	c.Not(b, "adder", "carry_d", "carry_d_n")
	c.sop(b, "adder", "", "carry_wn", cat([]string{"w_e", "is_alu"}, op("xx00")))
	c.latch(b, "adder", "carry", "carry_n", "carry_d", "carry_d_n", "carry_wn")

	// Jump condition, latched in state 7 of every instruction (src is on
	// the bus in step E). mm = ir[9:8].
	c.sop(b, "ctl", "taken", "taken_n",
		cat([]string{"is_jump"}, op("xxxxxx00")),
		cat([]string{"is_jump", "nz_n_15"}, op("xxxxxx01")),
		cat([]string{"is_jump", "nz_15"}, op("xxxxxx10")),
		cat([]string{"is_jump", "bus_15"}, op("xxxxxx11")),
		cat([]string{"is_jumpc"}, op("xxxxxx00")),
		cat([]string{"is_jumpc", "carry_n"}, op("xxxxxx01")),
		cat([]string{"is_jumpc", "carry"}, op("xxxxxx10")),
	)
	c.latch(b, "ctl", "take", "take_n", "taken", "taken_n", "w_e_n")
	c.inv(b, "ctl", "w_e")

	// Halt: set in state 7 of a halt, cleared only by reset.
	c.NOR(b, "ctl", "halted_s", "is_halt_n", "w_e_n")
	c.NOR(b, "ctl", "halted", "halted_n", "rst")
	c.NOR(b, "ctl", "halted_n", "halted_s", "halted")
	c.pair("halted", "halted_n")
	c.inv(b, "ctl", "is_halt")

	// Register reads and writes. Enables and decoded register fields are
	// computed once; each register's unit reads them through taps.
	c.and(b, "ctl", "rd_src", "step_e", "reads_src")
	c.and(b, "ctl", "wr_dst", "w_e", "writes_dst")
	c.and(b, "ctl", "wr_setl", "w_e", "is_setl")
	c.and(b, "ctl", "wr_seth", "w_e", "is_seth")
	c.and(b, "ctl", "wr_alu_x", cat([]string{"w_e", "is_alu"}, op("xxxxxxx0"))...)
	c.and(b, "ctl", "wr_alu_y", cat([]string{"w_e", "is_alu"}, op("xxxxxxx1"))...)
	c.and(b, "ctl", "rd_jump", "step_p", "take")
	for code_, r := range regs {
		c.and(b, "ctl", "src_"+r, code(4, code_)...)
		c.and(b, "ctl", "dst_"+r, code(0, code_)...)
		c.and(b, "ctl", "set_"+r, code(8, code_)...)
		reads := []string{c.and(b, r, r+"_rd_p0", "rd_src", "src_"+r)}
		if r == "y" {
			reads = append(reads, "rd_jump")
		}
		c.NOR(b, r, r+"_rdn", reads...)
		lo := []string{
			c.and(b, r, r+"_wr_p0", "wr_dst", "dst_"+r),
			c.and(b, r, r+"_wr_p1", "wr_setl", "set_"+r),
		}
		switch r {
		case "x":
			lo = append(lo, "wr_alu_x")
		case "y":
			lo = append(lo, "wr_alu_y")
		}
		hi := append(append([]string{}, lo...), c.and(b, r, r+"_wr_p2", "wr_seth", "set_"+r))
		c.NOR(b, r, r+"_wn_lo", lo...)
		c.NOR(b, r, r+"_wn_hi", hi...)
	}
	c.sop(b, "tmp", "", "tmp_rdn", []string{"step_p", "take_n"})
	c.sop(b, "imm", "imm_lo", "imm_lo_n", []string{"step_e", "is_setl"})
	c.sop(b, "imm", "", "imm_hi_n", []string{"step_e", "is_seth"})
	c.and(b, "bus", "mem_read", "step_e", "is_load")
	c.and(b, "bus", "mem_write", "w_e", "is_store")
	c.sop(b, "result", "", "alu_rdn", []string{"step_e", "is_alu"})

	// ALU control, from the opcode alone: ooo = ir[13:11], mm = ir[10:9].
	c.sop(b, "alu_in", "ia", "ia_n", op("xx1xxx1x"), op("xxx1xx1x"), op("xxxx1x1x"))
	for name, pat := range map[string]string{"add": "xx000", "sh": "xx001", "and": "xx010", "or": "xx011", "xor": "xx100"} {
		c.sop(b, "result", "", "sel_"+name+"_n", op(pat))
	}

	// ROM: always reading. Constants for its enables.
	c.NOR(b, "pc", "rom_read")
	c.Not(b, "pc", "rom_write", "rom_read")
}

func (c *cpu) transistors() int {
	n := 0
	for _, g := range c.gates {
		n += len(g.In)
	}
	return n
}
