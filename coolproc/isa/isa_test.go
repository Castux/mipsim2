package isa

import (
	"strings"
	"testing"
)

// every instruction form the encoding has, each once.
func everyInstruction() []Instruction {
	var all []Instruction
	all = append(all, Instruction{Kind: Halt})
	for _, s := range Registers {
		for _, d := range Registers {
			all = append(all, Instruction{Kind: Move, Source: s, Destination: d})
		}
		all = append(all, Instruction{Kind: Load, Destination: s}, Instruction{Kind: Store, Source: s})
		for c := Always; c <= IfNegative; c++ {
			in := Instruction{Kind: Jump, Condition: c}
			if c != Always {
				in.Source = s
			}
			all = append(all, in)
		}
		for _, v := range []uint8{0, 1, 0x80, 0xff} {
			all = append(all, Instruction{Kind: SetLow, Destination: s, Value: v}, Instruction{Kind: SetHigh, Destination: s, Value: v})
		}
	}
	for c := Always; c <= IfCarrySet; c++ {
		all = append(all, Instruction{Kind: JumpOnCarry, Condition: c})
	}
	for o := Add; o <= Xor; o++ {
		for m := Modes(0); m <= InvertA|InvertB; m++ {
			for _, d := range []Register{X, Y} {
				all = append(all, Instruction{Kind: Arithmetic, Operation: o, Modes: m, Destination: d})
			}
		}
	}
	return all
}

func TestEncodeDecodeAssemble(t *testing.T) {
	for _, in := range everyInstruction() {
		word := in.Encode()
		if back := Decode(word); back != in {
			t.Errorf("%v: decodes as %+v", in, back)
		}
		words, err := Assemble(in.String())
		if err != nil || len(words) != 1 || words[0] != word {
			t.Errorf("%q assembles as %04x, %v; want %04x", in.String(), words, err, word)
		}
	}
}

func TestDisassembleEveryWord(t *testing.T) {
	for w := range 1 << 16 {
		text := Disassemble(uint16(w))
		words, err := Assemble(text)
		if err != nil || len(words) != 1 || words[0] != uint16(w) {
			t.Fatalf("%04x: %q assembles as %04x, %v", w, text, words, err)
		}
	}
}

func TestPatterns(t *testing.T) {
	for _, in := range everyInstruction() {
		opcode := Opcode(in.Encode())
		kinds := map[Kind]Pattern{Halt: HaltPattern, Move: MovePattern, Load: LoadPattern, Store: StorePattern,
			Jump: JumpPattern, JumpOnCarry: JumpOnCarryPattern, Arithmetic: ArithmeticPattern, SetLow: SetLowPattern, SetHigh: SetHighPattern}
		for kind, p := range kinds {
			if p.Matches(opcode) != (kind == in.Kind) {
				t.Errorf("%v (%08b): pattern of kind %d matches %v", in, opcode, kind, p.Matches(opcode))
			}
		}
		if in.Kind == Arithmetic && !OperationPattern(in.Operation).Matches(opcode) {
			t.Errorf("%v: operation pattern does not match", in)
		}
	}
}

func TestAssemblerShorthands(t *testing.T) {
	words, err := Assemble(`
	start:
		set c, 0x1234
		sub x       ; add x nb c
		nand y
		goto start`)
	if err != nil {
		t.Fatal(err)
	}
	var text []string
	for _, w := range words {
		text = append(text, Disassemble(w))
	}
	want := "setl c, 0x34|seth c, 0x12|add x c nb|or y na nb|setl y, 0x00|seth y, 0x00|jump"
	if got := strings.Join(text, "|"); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestEmulator(t *testing.T) {
	for _, tc := range []struct {
		source string
		r      Register
		want   uint16
		carry  bool
	}{
		{"setl a, 3\n setl b, 10\n sub x", X, 0xfff9, false},
		{"set a, 0x8001\n setl b, 1\n shl x", X, 2, true},
		{"setl a, 1\n setl b, 1\n shr x", X, 0, true},
		{"setl a, 0x34\n seth a, 0x12", A, 0x1234, false},
		{"setl a, 0xff\n setl b, 1\n inc x", X, 0x101, false},
		{"set a, 0xffff\n setl b, 1\n add y\n and x", X, 1, true}, // and leaves the carry
		{"setl c, 7\n move c, d\n set y, 0x100\n store d\n setl d, 0\n load d", D, 7, false},
	} {
		words, err := Assemble(tc.source + "\n halt")
		if err != nil {
			t.Fatal(err)
		}
		e := NewEmulator(words)
		if !e.Run(100) || e.Register(tc.r) != tc.want || e.Carry != tc.carry {
			t.Errorf("%q: %s = %#x, carry %v", tc.source, tc.r, e.Register(tc.r), e.Carry)
		}
	}
}
