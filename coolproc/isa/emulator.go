package isa

// MemoryWords is the size of the data memory: one word per 16-bit address.
const MemoryWords = 1 << 16

// Emulator runs programs instruction by instruction: the reference the
// circuits are tested against.
type Emulator struct {
	Program        []uint16 // the program memory; zero (halt) after it
	Registers      [6]uint16
	ProgramCounter uint16
	Carry          bool
	Memory         []uint16
	Halted         bool
	Steps          int // instructions run, the halt included
}

// NewEmulator returns an emulator after reset: everything zero.
func NewEmulator(program []uint16) *Emulator {
	return &Emulator{Program: program, Memory: make([]uint16, MemoryWords)}
}

// Register returns a register's value.
func (e *Emulator) Register(r Register) uint16 { return e.Registers[r] }

// Fetch returns the word at the program counter.
func (e *Emulator) Fetch() uint16 {
	if int(e.ProgramCounter) < len(e.Program) {
		return e.Program[e.ProgramCounter]
	}
	return 0
}

// Run runs until the program halts, at most limit instructions. It reports
// whether the program halted.
func (e *Emulator) Run(limit int) bool {
	for range limit {
		if e.Halted {
			break
		}
		e.Step()
	}
	return e.Halted
}

// Step runs one instruction.
func (e *Emulator) Step() {
	if e.Halted {
		return
	}
	in := Decode(e.Fetch())
	e.Steps++
	next := e.ProgramCounter + 1
	switch in.Kind {
	case Halt:
		e.Halted = true // the program counter stays on the halt
		return
	case Move:
		e.Registers[in.Destination] = e.Registers[in.Source]
	case Load:
		e.Registers[in.Destination] = e.Memory[e.Registers[Y]]
	case Store:
		e.Memory[e.Registers[Y]] = e.Registers[in.Source]
	case Jump:
		if JumpTaken(in.Condition, e.Registers[in.Source]) {
			next = e.Registers[Y]
		}
	case JumpOnCarry:
		if CarryJumpTaken(in.Condition, e.Carry) {
			next = e.Registers[Y]
		}
	case Arithmetic:
		result, carry, writesCarry := Compute(in.Operation, in.Modes, e.Registers[A], e.Registers[B])
		e.Registers[in.Destination] = result
		if writesCarry {
			e.Carry = carry
		}
	case SetLow:
		e.Registers[in.Destination] = uint16(in.Value)
	case SetHigh:
		low := e.Registers[in.Destination] & 0x00ff
		e.Registers[in.Destination] = uint16(in.Value)<<8 | low
	}
	e.ProgramCounter = next
}

// JumpTaken is a jump's decision on its source register.
func JumpTaken(c Condition, source uint16) bool {
	switch c {
	case Always:
		return true
	case IfZero:
		return source == 0
	case IfNotZero:
		return source != 0
	case IfNegative:
		return source&0x8000 != 0
	}
	return false
}

// CarryJumpTaken is a jump on carry's decision. Its fourth condition never
// jumps.
func CarryJumpTaken(c Condition, carry bool) bool {
	switch c {
	case Always:
		return true
	case IfCarryClear:
		return !carry
	case IfCarrySet:
		return carry
	}
	return false
}

// Compute is the ALU: the result of an operation on A and B, its carry,
// and whether the operation writes the carry (add and shift do).
func Compute(o Operation, modes Modes, a, b uint16) (result uint16, carry, writesCarry bool) {
	switch o {
	case Add:
		if modes&InvertB != 0 {
			b = ^b
		}
		sum := uint32(a) + uint32(b)
		if modes&CarryIn != 0 {
			sum++
		}
		return uint16(sum), sum>>16 != 0, true
	case Shift:
		// By B[3:0]; the carry is the last bit shifted out (0 when the
		// amount is 0).
		amount := b & 15
		if modes&ShiftRight != 0 {
			return a >> amount, amount > 0 && a>>(amount-1)&1 != 0, true
		}
		return a << amount, amount > 0 && a>>(16-amount)&1 != 0, true
	}
	if modes&InvertA != 0 {
		a = ^a
	}
	if modes&InvertB != 0 {
		b = ^b
	}
	switch o {
	case And:
		return a & b, false, false
	case Or:
		return a | b, false, false
	case Xor:
		return a ^ b, false, false
	}
	return 0, false, false
}
