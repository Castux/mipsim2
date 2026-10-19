package isa

// The assembler. One instruction per line; ";" starts a comment; "name:"
// defines a label, the address of the next instruction. Registers are a,
// b, x, y, c, d. Numbers are decimal, or 0x hexadecimal, 0b binary.
//
//	halt
//	move SOURCE, DESTINATION   copy SOURCE into DESTINATION
//	load DESTINATION           DESTINATION = memory[Y]
//	store SOURCE               memory[Y] = SOURCE
//	setl REGISTER, N           REGISTER = N (0 to 255)
//	seth REGISTER, N           the high byte of REGISTER = N
//	jump                       program counter = Y
//	jumpz R, jumpnz R, jumpn R jump if R is 0, not 0, negative
//	jumpc                      jump (the jump on carry form, always)
//	jumpcz, jumpcnz            jump if the carry is 0, 1
//
// Arithmetic instructions name their destination (x or y), then modes:
//
//	add DESTINATION [nb] [c]           A + B; nb uses not B, c adds 1
//	shift DESTINATION [right]          A shifted by B[3:0], left or right
//	and, or, xor DESTINATION [na] [nb] na uses not A, nb not B
//
// Shorthands:
//
//	set REGISTER, N            setl then seth (two instructions; N may be a label)
//	goto LABEL                 set y, LABEL then jump (three instructions)
//	word N                     the 16-bit word N, as it is
//	sub D = add D nb c         A - B
//	subb D = add D nb          A - B - 1
//	inc D = add D c            A + B + 1
//	shl D = shift D, shr D = shift D right
//	nand D = or D na nb, nor D = and D na nb, xnor D = xor D na

import (
	"fmt"
	"strconv"
	"strings"
)

// shorthand is an arithmetic mnemonic for an operation and its modes.
type shorthand struct {
	operation Operation
	modes     Modes
}

var arithmeticMnemonics = map[string]shorthand{
	"add":   {Add, 0},
	"shift": {Shift, 0},
	"and":   {And, 0},
	"or":    {Or, 0},
	"xor":   {Xor, 0},
	"sub":   {Add, InvertB | CarryIn},
	"subb":  {Add, InvertB},
	"inc":   {Add, CarryIn},
	"shl":   {Shift, 0},
	"shr":   {Shift, ShiftRight},
	"nand":  {Or, InvertA | InvertB},
	"nor":   {And, InvertA | InvertB},
	"xnor":  {Xor, InvertA},
}

// modeWords are the mode words after an arithmetic destination.
var modeWords = map[string]Modes{"na": InvertA, "c": CarryIn, "right": ShiftRight, "nb": InvertB}

var jumpMnemonics = map[string]Condition{"jump": Always, "jumpz": IfZero, "jumpnz": IfNotZero, "jumpn": IfNegative}

var carryJumpMnemonics = map[string]Condition{"jumpc": Always, "jumpcz": IfCarryClear, "jumpcnz": IfCarrySet}

// statement is one line's instruction, before labels are known.
type statement struct {
	line     int
	mnemonic string
	operands []string
}

// size is how many words a statement assembles to.
func (s statement) size() int {
	switch s.mnemonic {
	case "set":
		return 2
	case "goto":
		return 3
	}
	return 1
}

// Assemble turns source code into program words.
func Assemble(source string) ([]uint16, error) {
	var statements []statement
	labels := map[string]int{}
	address := 0
	for number, text := range strings.Split(source, "\n") {
		if comment := strings.IndexByte(text, ';'); comment >= 0 {
			text = text[:comment]
		}
		text = strings.TrimSpace(text)
		for {
			colon := strings.IndexByte(text, ':')
			if colon < 0 {
				break
			}
			label := strings.TrimSpace(text[:colon])
			if _, duplicate := labels[label]; duplicate {
				return nil, fmt.Errorf("line %d: label %s defined twice", number+1, label)
			}
			labels[label] = address
			text = strings.TrimSpace(text[colon+1:])
		}
		if text == "" {
			continue
		}
		fields := strings.Fields(strings.ReplaceAll(text, ",", " "))
		s := statement{line: number + 1, mnemonic: strings.ToLower(fields[0]), operands: fields[1:]}
		statements = append(statements, s)
		address += s.size()
	}
	var words []uint16
	for _, s := range statements {
		w, err := s.assemble(labels)
		if err != nil {
			return nil, fmt.Errorf("line %d: %s: %w", s.line, s.mnemonic, err)
		}
		words = append(words, w...)
	}
	return words, nil
}

func (s statement) operand(k int) (string, error) {
	if k >= len(s.operands) {
		return "", fmt.Errorf("missing operand %d", k+1)
	}
	return s.operands[k], nil
}

func (s statement) register(k int) (Register, error) {
	text, err := s.operand(k)
	if err != nil {
		return 0, err
	}
	r, ok := ParseRegister(strings.ToLower(text))
	if !ok {
		return 0, fmt.Errorf("unknown register %q", text)
	}
	return r, nil
}

// number reads a label or a number that fits in bits (negative numbers
// wrap, so -1 is all ones).
func (s statement) number(k int, labels map[string]int, bits int) (int, error) {
	text, err := s.operand(k)
	if err != nil {
		return 0, err
	}
	if address, ok := labels[text]; ok {
		return address, nil
	}
	v, err := strconv.ParseInt(text, 0, 32)
	limit := int64(1) << bits
	if err != nil || v < -limit/2 || v >= limit {
		return 0, fmt.Errorf("bad value %q (%d bits)", text, bits)
	}
	return int(v) & int(limit-1), nil
}

func (s statement) assemble(labels map[string]int) ([]uint16, error) {
	encode := func(ins ...Instruction) []uint16 {
		var w []uint16
		for _, in := range ins {
			w = append(w, in.Encode())
		}
		return w
	}
	set := func(r Register, v int) []uint16 {
		return encode(Instruction{Kind: SetLow, Destination: r, Value: uint8(v)}, Instruction{Kind: SetHigh, Destination: r, Value: uint8(v >> 8)})
	}
	switch s.mnemonic {
	case "halt":
		return encode(Instruction{Kind: Halt}), nil
	case "word":
		v, err := s.number(0, labels, 16)
		return []uint16{uint16(v)}, err
	case "move":
		source, err := s.register(0)
		if err != nil {
			return nil, err
		}
		destination, err := s.register(1)
		return encode(Instruction{Kind: Move, Source: source, Destination: destination}), err
	case "load":
		destination, err := s.register(0)
		return encode(Instruction{Kind: Load, Destination: destination}), err
	case "store":
		source, err := s.register(0)
		return encode(Instruction{Kind: Store, Source: source}), err
	case "setl", "seth":
		r, err := s.register(0)
		if err != nil {
			return nil, err
		}
		v, err := s.number(1, labels, 8)
		kind := map[string]Kind{"setl": SetLow, "seth": SetHigh}[s.mnemonic]
		return encode(Instruction{Kind: kind, Destination: r, Value: uint8(v)}), err
	case "set":
		r, err := s.register(0)
		if err != nil {
			return nil, err
		}
		v, err := s.number(1, labels, 16)
		return set(r, v), err
	case "goto":
		v, err := s.number(0, labels, 16)
		return append(set(Y, v), encode(Instruction{Kind: Jump, Condition: Always})...), err
	}
	if condition, ok := jumpMnemonics[s.mnemonic]; ok {
		in := Instruction{Kind: Jump, Condition: condition}
		if condition != Always {
			var err error
			if in.Source, err = s.register(0); err != nil {
				return nil, err
			}
		}
		return encode(in), nil
	}
	if condition, ok := carryJumpMnemonics[s.mnemonic]; ok {
		return encode(Instruction{Kind: JumpOnCarry, Condition: condition}), nil
	}
	if a, ok := arithmeticMnemonics[s.mnemonic]; ok {
		destination, err := s.register(0)
		if err != nil {
			return nil, err
		}
		if destination != X && destination != Y {
			return nil, fmt.Errorf("the destination must be x or y")
		}
		modes := a.modes
		for _, word := range s.operands[1:] {
			m, ok := modeWords[word]
			if !ok {
				return nil, fmt.Errorf("unknown mode %q", word)
			}
			modes |= m
		}
		return encode(Instruction{Kind: Arithmetic, Operation: a.operation, Modes: modes, Destination: destination}), nil
	}
	return nil, fmt.Errorf("unknown instruction")
}

// String is the instruction in assembler syntax.
func (in Instruction) String() string {
	switch in.Kind {
	case Halt:
		return "halt"
	case Move:
		return fmt.Sprintf("move %s, %s", in.Source, in.Destination)
	case Load:
		return fmt.Sprintf("load %s", in.Destination)
	case Store:
		return fmt.Sprintf("store %s", in.Source)
	case SetLow:
		return fmt.Sprintf("setl %s, 0x%02x", in.Destination, in.Value)
	case SetHigh:
		return fmt.Sprintf("seth %s, 0x%02x", in.Destination, in.Value)
	case Jump:
		for name, c := range jumpMnemonics {
			if c == in.Condition {
				if c == Always {
					return name
				}
				return fmt.Sprintf("%s %s", name, in.Source)
			}
		}
	case JumpOnCarry:
		for name, c := range carryJumpMnemonics {
			if c == in.Condition {
				return name
			}
		}
	case Arithmetic:
		text := fmt.Sprintf("%s %s", in.Operation, in.Destination)
		if in.Modes&InvertA != 0 { // the low mode bit, named after the operation
			switch in.Operation {
			case Add:
				text += " c"
			case Shift:
				text += " right"
			default:
				text += " na"
			}
		}
		if in.Modes&InvertB != 0 {
			text += " nb"
		}
		return text
	}
	return "reserved"
}

// Disassemble is a word in assembler syntax, as "word N" when no
// instruction gives that word back (reserved opcodes, ignored bits set).
func Disassemble(word uint16) string {
	text := Decode(word).String()
	if back, err := Assemble(text); err != nil || len(back) != 1 || back[0] != word {
		return fmt.Sprintf("word %#04x", word)
	}
	return text
}
