// Package isa is the coolproc instruction set (proc.md): its registers,
// instructions and their encoding, an assembler and disassembler for it,
// and a reference emulator.
//
// An instruction is one 16-bit word: the opcode in the high byte, the
// argument in the low byte. The argument is either two register fields,
// source (high nibble) and destination (low nibble), or an 8-bit value
// (set).
//
//	0000 0000   halt
//	0000 0001   move      source, destination
//	0000 0010   load      destination
//	0000 0011   store     source
//	0001 0 x cc jump      source, cc: condition on the source register
//	0001 1 x cc jumpc     cc: condition on the carry
//	01 ooo mm d alu       ooo: operation, mm: modes, d: destination (X or Y)
//	1 h 000 rrr set       h: high byte, rrr: register; the argument is the value
//
// Bits marked x are ignored. Every other opcode is reserved: what it does
// is not defined.
package isa

import "fmt"

// Register is a register number, as instructions encode it.
type Register uint8

const (
	A Register = iota // ALU input
	B                 // ALU input
	X                 // ALU output
	Y                 // ALU output, memory address and jump target
	C                 // general purpose
	D                 // general purpose
)

// Registers lists the registers in number order.
var Registers = []Register{A, B, X, Y, C, D}

var registerNames = []string{"a", "b", "x", "y", "c", "d"}

func (r Register) String() string {
	if int(r) < len(registerNames) {
		return registerNames[r]
	}
	return fmt.Sprintf("register_%d", r)
}

// ParseRegister reads a register name (a, b, x, y, c, d).
func ParseRegister(name string) (Register, bool) {
	for k, n := range registerNames {
		if n == name {
			return Register(k), true
		}
	}
	return 0, false
}

// Kind is what an instruction does.
type Kind uint8

const (
	Halt Kind = iota
	Move
	Load
	Store
	Jump        // on a condition of the source register
	JumpOnCarry // on a condition of the carry
	Arithmetic  // an ALU operation, to X or Y
	SetLow      // the register becomes the value
	SetHigh     // the register's high byte becomes the value
	Reserved    // does nothing
)

// Operation is an ALU operation.
type Operation uint8

const (
	Add Operation = iota
	Shift
	And
	Or
	Xor
)

var operationNames = []string{"add", "shift", "and", "or", "xor"}

func (o Operation) String() string {
	if int(o) < len(operationNames) {
		return operationNames[o]
	}
	return fmt.Sprintf("operation_%d", o)
}

// Modes are the two mode bits of an ALU instruction. The low bit means
// something different for each operation; the high bit always inverts B.
type Modes uint8

const (
	InvertA    Modes = 1 // and, or, xor: use not A
	CarryIn    Modes = 1 // add: add 1
	ShiftRight Modes = 1 // shift: right instead of left
	InvertB    Modes = 2 // every operation: use not B
)

// Condition is a jump's condition.
type Condition uint8

const (
	Always     Condition = iota
	IfZero               // jump: the source is 0
	IfNotZero            // jump: the source is not 0
	IfNegative           // jump: bit 15 of the source is set
)

const (
	IfCarryClear Condition = 1 // jumpc: the carry is 0
	IfCarrySet   Condition = 2 // jumpc: the carry is 1
)

// Instruction is a decoded instruction. Only the fields its kind uses are
// set.
type Instruction struct {
	Kind        Kind
	Source      Register  // move, store, jump
	Destination Register  // move, load, set; for arithmetic, X or Y
	Value       uint8     // set
	Condition   Condition // jump, jumpc
	Operation   Operation // arithmetic
	Modes       Modes     // arithmetic
}

// The encoding: opcodes and their fields.
const (
	opcodeHalt  = 0b0000_0000
	opcodeMove  = 0b0000_0001
	opcodeLoad  = 0b0000_0010
	opcodeStore = 0b0000_0011

	jumpGroup      = 0b0001_0000 // 0001 c x cc
	jumpGroupMask  = 0b1111_0000
	jumpOnCarryBit = 0b0000_1000
	conditionMask  = 0b0000_0011

	arithmeticGroup     = 0b0100_0000 // 01 ooo mm d
	arithmeticGroupMask = 0b1100_0000
	operationShift      = 3
	operationMask       = 0b111
	modesShift          = 1
	modesMask           = 0b11
	resultToYBit        = 0b0000_0001

	setGroup     = 0b1000_0000 // 1 h 000 rrr
	setGroupMask = 0b1000_0000
	setHighBit   = 0b0100_0000
	setRegister  = 0b0000_0111

	registerField = 0b0111 // the register in a nibble; its top bit is reserved
)

// Word builds an instruction word from its opcode and argument bytes.
func Word(opcode, argument uint8) uint16 { return uint16(opcode)<<8 | uint16(argument) }

// Opcode and Argument split an instruction word.
func Opcode(word uint16) uint8   { return uint8(word >> 8) }
func Argument(word uint16) uint8 { return uint8(word) }

func registers(source, destination Register) uint8 {
	return uint8(source)<<4 | uint8(destination)
}

// Encode returns the instruction's word.
func (in Instruction) Encode() uint16 {
	switch in.Kind {
	case Halt:
		return Word(opcodeHalt, 0)
	case Move:
		return Word(opcodeMove, registers(in.Source, in.Destination))
	case Load:
		return Word(opcodeLoad, registers(0, in.Destination))
	case Store:
		return Word(opcodeStore, registers(in.Source, 0))
	case Jump:
		return Word(jumpGroup|uint8(in.Condition), registers(in.Source, 0))
	case JumpOnCarry:
		return Word(jumpGroup|jumpOnCarryBit|uint8(in.Condition), 0)
	case Arithmetic:
		opcode := arithmeticGroup | uint8(in.Operation)<<operationShift | uint8(in.Modes)<<modesShift
		if in.Destination == Y {
			opcode |= resultToYBit
		}
		return Word(opcode, 0)
	case SetLow:
		return Word(setGroup|uint8(in.Destination), in.Value)
	case SetHigh:
		return Word(setGroup|setHighBit|uint8(in.Destination), in.Value)
	}
	panic(fmt.Sprintf("cannot encode %+v", in))
}

// Decode reads an instruction word.
func Decode(word uint16) Instruction {
	opcode, argument := Opcode(word), Argument(word)
	source := Register(argument >> 4 & registerField)
	destination := Register(argument & registerField)
	switch {
	case opcode == opcodeHalt:
		return Instruction{Kind: Halt}
	case opcode == opcodeMove:
		return Instruction{Kind: Move, Source: source, Destination: destination}
	case opcode == opcodeLoad:
		return Instruction{Kind: Load, Destination: destination}
	case opcode == opcodeStore:
		return Instruction{Kind: Store, Source: source}
	case opcode&jumpGroupMask == jumpGroup:
		condition := Condition(opcode & conditionMask)
		if opcode&jumpOnCarryBit != 0 {
			return Instruction{Kind: JumpOnCarry, Condition: condition}
		}
		return Instruction{Kind: Jump, Source: source, Condition: condition}
	case opcode&arithmeticGroupMask == arithmeticGroup:
		in := Instruction{
			Kind:        Arithmetic,
			Operation:   Operation(opcode >> operationShift & operationMask),
			Modes:       Modes(opcode >> modesShift & modesMask),
			Destination: X,
		}
		if opcode&resultToYBit != 0 {
			in.Destination = Y
		}
		if in.Operation > Xor {
			return Instruction{Kind: Reserved}
		}
		return in
	case opcode&setGroupMask == setGroup:
		in := Instruction{Kind: SetLow, Destination: Register(opcode & setRegister), Value: argument}
		if opcode&setHighBit != 0 {
			in.Kind = SetHigh
		}
		return in
	}
	return Instruction{Kind: Reserved}
}

// Pattern is how a circuit recognises an instruction kind or ALU
// operation: the opcode bits from 7 down to 0, as '0', '1' or 'x' (any).
type Pattern string

// Patterns of the instruction kinds.
var (
	HaltPattern        Pattern = "00000000"
	MovePattern        Pattern = "00000001"
	LoadPattern        Pattern = "00000010"
	StorePattern       Pattern = "00000011"
	JumpPattern        Pattern = "00010xxx"
	JumpOnCarryPattern Pattern = "00011xxx"
	ArithmeticPattern  Pattern = "01xxxxxx"
	SetLowPattern      Pattern = "10xxxxxx"
	SetHighPattern     Pattern = "11xxxxxx"
)

// Opcode bits that are fields, for circuits: bit numbers in the opcode.
const (
	ResultToYBit   = 0 // arithmetic: the destination (X or Y)
	ModeLowBit     = 1 // arithmetic: InvertA, CarryIn or ShiftRight
	ModeHighBit    = 2 // arithmetic: InvertB
	ConditionLow   = 0 // jumps: the condition's two bits
	ConditionHigh  = 1
	SetRegisterLow = 0 // set: the register's three bits
)

// Word bits where the register fields start (each is three bits), for
// circuits.
const (
	SourceFieldBit      = 4                  // move, store, jump
	DestinationFieldBit = 0                  // move, load
	SetRegisterFieldBit = 8 + SetRegisterLow // set
)

// Patterns of an arithmetic instruction's destination.
var (
	ResultToXPattern Pattern = "xxxxxxx0"
	ResultToYPattern Pattern = "xxxxxxx1"
)

// ConditionPattern is the pattern of a jump condition (its cc field).
func ConditionPattern(c Condition) Pattern {
	return Pattern(fmt.Sprintf("xxxxxx%02b", uint8(c)))
}

// OperationPattern is the pattern of an ALU operation (its ooo field).
func OperationPattern(o Operation) Pattern {
	return Pattern(fmt.Sprintf("xx%03bxxx", uint8(o)))
}

// Matches reports whether an opcode matches the pattern.
func (p Pattern) Matches(opcode uint8) bool {
	for k := range 8 {
		bit := opcode >> (7 - k) & 1
		switch p[k] {
		case '0':
			if bit != 0 {
				return false
			}
		case '1':
			if bit != 1 {
				return false
			}
		}
	}
	return true
}
