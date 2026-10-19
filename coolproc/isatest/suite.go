// Package isatest checks a coolproc processor against the reference
// emulator (coolproc/isa): every instruction in every form, random
// programs, and real programs (multiplication, division, sorting...). Both
// generated processors run it from their tests.
package isatest

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/Castux/mipsim2/coolproc/isa"
)

// Machine is a processor booted with a program, after reset.
type Machine interface {
	// Run runs until the processor halts, at most limit instructions, and
	// returns the number of instructions run, the halt included (-1 if it
	// did not halt).
	Run(limit int) int
	Register(r isa.Register) uint16
	ProgramCounter() uint16
	Carry() bool
	Memory() []uint64
}

// Boot loads a program into a processor and resets it.
type Boot func(tb testing.TB, program []uint16) Machine

// Run checks the processor that boot starts against the emulator.
func Run(t *testing.T, boot Boot) {
	t.Run("set", func(t *testing.T) { compare(t, boot, setProgram()) })
	t.Run("move", func(t *testing.T) { compare(t, boot, moveProgram()) })
	t.Run("load_and_store", func(t *testing.T) { compare(t, boot, memoryProgram()) })
	for _, operation := range []isa.Operation{isa.Add, isa.And, isa.Or, isa.Xor} {
		for modes := isa.Modes(0); modes <= isa.InvertA|isa.InvertB; modes++ {
			for _, destination := range []isa.Register{isa.X, isa.Y} {
				in := isa.Instruction{Kind: isa.Arithmetic, Operation: operation, Modes: modes, Destination: destination}
				t.Run(strings.ReplaceAll(in.String(), " ", "_"), func(t *testing.T) {
					compare(t, boot, arithmeticProgram(in, interestingValues, interestingValues))
				})
			}
		}
	}
	for _, modes := range []isa.Modes{0, isa.ShiftRight} {
		for _, destination := range []isa.Register{isa.X, isa.Y} {
			in := isa.Instruction{Kind: isa.Arithmetic, Operation: isa.Shift, Modes: modes, Destination: destination}
			t.Run(strings.ReplaceAll(in.String(), " ", "_"), func(t *testing.T) {
				compare(t, boot, arithmeticProgram(in, interestingValues, shiftAmounts))
			})
		}
	}
	t.Run("jumps", func(t *testing.T) { compare(t, boot, jumpProgram()) })
	t.Run("jumps_on_carry", func(t *testing.T) { compare(t, boot, carryJumpProgram()) })
	for seed := int64(1); seed <= 8; seed++ {
		t.Run(fmt.Sprintf("random_%d", seed), func(t *testing.T) { compare(t, boot, randomProgram(seed, 300)) })
	}
	for _, p := range Programs {
		t.Run(p.Name, func(t *testing.T) {
			e := compare(t, boot, p.Source)
			if err := p.Check(e); err != nil {
				t.Error(err)
			}
		})
	}
}

// Assemble assembles a program, failing the test on errors.
func Assemble(tb testing.TB, source string) []uint16 {
	tb.Helper()
	words, err := isa.Assemble(source)
	if err != nil {
		tb.Fatalf("assembling: %v", err)
	}
	return words
}

// compare runs a program on the emulator and the processor, and compares
// what they leave: registers, program counter, carry, memory, and the
// number of instructions run. It returns the emulator.
func compare(t *testing.T, boot Boot, source string) *isa.Emulator {
	t.Helper()
	program := Assemble(t, source)
	const limit = 200000
	want := isa.NewEmulator(program)
	if !want.Run(limit) {
		t.Fatalf("the emulator did not halt")
	}
	m := boot(t, program)
	if n := m.Run(want.Steps + 2); n != want.Steps {
		t.Fatalf("halted after %d instructions, want %d", n, want.Steps)
	}
	for _, r := range isa.Registers {
		if got := m.Register(r); got != want.Register(r) {
			t.Errorf("register %s = %#04x, want %#04x", r, got, want.Register(r))
		}
	}
	if got := m.ProgramCounter(); got != want.ProgramCounter {
		t.Errorf("program counter = %#04x, want %#04x", got, want.ProgramCounter)
	}
	if got := m.Carry(); got != want.Carry {
		t.Errorf("carry = %v, want %v", got, want.Carry)
	}
	differences := 0
	for address, value := range m.Memory() {
		if expected := want.Memory[address]; uint64(expected) != value {
			if differences++; differences <= 10 {
				t.Errorf("memory[%#04x] = %#04x, want %#04x", address, value, expected)
			}
		}
	}
	if differences > 10 {
		t.Errorf("... %d words differ", differences)
	}
	return want
}

// interestingValues are 16-bit operands at the edges: zero, one, the signs,
// all ones, patterns.
var interestingValues = []uint16{0, 1, 2, 0x7fff, 0x8000, 0x8001, 0xffff, 0x1234, 0xabcd, 0x5555, 0x00ff, 0xff00}

// shiftAmounts are every amount B[3:0] can give, and values above 15 (only
// their low four bits count).
var shiftAmounts = []uint16{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 0x8003, 0xffff}

// programBuilder writes an assembly program.
type programBuilder struct {
	text   strings.Builder
	labels int
}

func (p *programBuilder) line(format string, arguments ...any) {
	fmt.Fprintf(&p.text, format+"\n", arguments...)
}

func (p *programBuilder) instruction(in isa.Instruction) { p.line("%s", in) }

func (p *programBuilder) newLabel() string {
	p.labels++
	return fmt.Sprintf("label_%d", p.labels)
}

func (p *programBuilder) String() string { return p.text.String() }

// storeRegister writes register r to memory at address. Y holds the
// address, so Y itself is copied to C first.
func (p *programBuilder) storeRegister(r isa.Register, address int) {
	if r == isa.Y {
		p.line("move y, c")
		r = isa.C
	}
	p.line("set y, %d", address)
	p.line("store %s", r)
}

// storeCarry writes the carry (0 or 1) to memory at address, using D and Y.
func (p *programBuilder) storeCarry(address int) {
	clear := p.newLabel()
	p.line("setl d, 0")
	p.line("set y, %s", clear)
	p.line("jumpcz")
	p.line("setl d, 1")
	p.line("%s:", clear)
	p.storeRegister(isa.D, address)
}

// giveDistinctValues gives every register a value of its own.
func (p *programBuilder) giveDistinctValues() {
	for k, r := range isa.Registers {
		p.line("set %s, %d", r, 0x1111*(k+1)+3)
	}
}

// setCarry makes the carry 1 or 0 (with an addition into X).
func (p *programBuilder) setCarry(carry bool) {
	if carry {
		p.line("set a, 0xffff")
	} else {
		p.line("setl a, 0")
	}
	p.line("setl b, 1")
	p.line("add x")
}

// setProgram writes every register with setl and seth, alone and combined.
func setProgram() string {
	var p programBuilder
	for k, r := range isa.Registers {
		p.line("setl %s, %d", r, 0x11*(k+1))
		p.storeRegister(r, 0x100+3*k)
		p.line("seth %s, %d", r, 0xa0+k) // keeps the low byte
		p.storeRegister(r, 0x101+3*k)
		p.line("setl %s, 0xfe", r) // clears the high byte
		p.storeRegister(r, 0x102+3*k)
	}
	p.line("halt")
	return p.String()
}

// moveProgram copies every register into every register.
func moveProgram() string {
	var p programBuilder
	address := 0x200
	for _, source := range isa.Registers {
		for _, destination := range isa.Registers {
			p.giveDistinctValues()
			p.instruction(isa.Instruction{Kind: isa.Move, Source: source, Destination: destination})
			p.storeRegister(destination, address)
			address++
		}
	}
	p.line("halt")
	return p.String()
}

// memoryProgram stores every register and loads into every register but Y
// (load y is forbidden: the address would change while loading).
func memoryProgram() string {
	var p programBuilder
	for k, source := range isa.Registers {
		p.giveDistinctValues()
		p.line("set y, %d", 0x300+k)
		p.line("store %s", source) // store y stores the address itself
	}
	for k, destination := range isa.Registers {
		if destination == isa.Y {
			continue
		}
		p.giveDistinctValues()
		p.line("set y, %d", 0x300+(k+1)%len(isa.Registers))
		p.line("load %s", destination)
		p.storeRegister(destination, 0x310+k)
	}
	p.line("halt")
	return p.String()
}

// arithmeticProgram runs one arithmetic instruction on every pair of
// operands, keeping the result and the carry. The carry is set before each
// one, so an operation that must leave it alone and does not shows.
func arithmeticProgram(in isa.Instruction, as, bs []uint16) string {
	var p programBuilder
	address := 0x400
	for _, a := range as {
		for _, b := range bs {
			p.setCarry(true)
			p.line("set a, %d", a)
			p.line("set b, %d", b)
			p.instruction(in)
			p.storeRegister(in.Destination, address)
			p.storeCarry(address + 1)
			address += 2
		}
	}
	p.line("halt")
	return p.String()
}

// jumpProgram tries every jump condition on every register, with values
// that make it true and false, and keeps whether it jumped.
func jumpProgram() string {
	var p programBuilder
	address := 0x600
	for condition := isa.Always; condition <= isa.IfNegative; condition++ {
		for _, source := range isa.Registers {
			for _, value := range []uint16{0, 1, 0x7fff, 0x8000, 0xffff} {
				marker := isa.C
				if source == isa.C {
					marker = isa.D
				}
				p.line("set %s, %d", source, value)
				jump := isa.Instruction{Kind: isa.Jump, Condition: condition, Source: source}
				if condition == isa.Always {
					jump.Source = 0
				}
				p.jumpOrNot(jump.String(), marker, address)
				address++
			}
		}
	}
	p.line("halt")
	return p.String()
}

// jumpOrNot runs a jump instruction, then writes 2 to memory at address if
// it jumped, 1 if not, through marker. Y is the jump target, so when the
// jump tests Y, its value is the target.
func (p *programBuilder) jumpOrNot(jump string, marker isa.Register, address int) {
	jumped, end := p.newLabel(), p.newLabel()
	p.line("set y, %s", jumped)
	p.line("%s", jump)
	p.line("setl %s, 1", marker)
	p.line("set y, %s", end)
	p.line("jump")
	p.line("%s:", jumped)
	p.line("setl %s, 2", marker)
	p.line("%s:", end)
	p.storeRegister(marker, address)
}

// carryJumpProgram tries the jumps on carry with the carry set and clear.
func carryJumpProgram() string {
	var p programBuilder
	address := 0x700
	for condition := isa.Always; condition <= isa.IfCarrySet; condition++ {
		for _, carry := range []bool{false, true} {
			p.setCarry(carry)
			p.jumpOrNot(isa.Instruction{Kind: isa.JumpOnCarry, Condition: condition}.String(), isa.C, address)
			address++
		}
	}
	p.line("halt")
	return p.String()
}

// randomProgram is a random program without loops: sets, moves, loads and
// stores, every arithmetic instruction, and forward jumps of every kind.
func randomProgram(seed int64, length int) string {
	random := rand.New(rand.NewSource(seed))
	var p programBuilder
	anyRegister := func() isa.Register { return isa.Registers[random.Intn(len(isa.Registers))] }
	notY := func() isa.Register { return []isa.Register{isa.A, isa.B, isa.X, isa.C, isa.D}[random.Intn(5)] }
	labelsAt := map[int][]string{} // instruction index -> labels placed there
	for i := range length + 5 {
		for _, label := range labelsAt[i] {
			p.line("%s:", label)
		}
		if i >= length {
			continue
		}
		switch k := random.Intn(20); {
		case k < 4:
			p.instruction(isa.Instruction{Kind: isa.SetLow, Destination: anyRegister(), Value: uint8(random.Intn(256))})
		case k < 6:
			p.instruction(isa.Instruction{Kind: isa.SetHigh, Destination: anyRegister(), Value: uint8(random.Intn(256))})
		case k < 8:
			p.instruction(isa.Instruction{Kind: isa.Move, Source: anyRegister(), Destination: anyRegister()})
		case k < 9:
			p.instruction(isa.Instruction{Kind: isa.Store, Source: anyRegister()})
		case k < 10:
			p.instruction(isa.Instruction{Kind: isa.Load, Destination: notY()})
		case k < 17:
			p.instruction(isa.Instruction{
				Kind:        isa.Arithmetic,
				Operation:   isa.Operation(random.Intn(int(isa.Xor) + 1)),
				Modes:       isa.Modes(random.Intn(4)),
				Destination: []isa.Register{isa.X, isa.Y}[random.Intn(2)],
			})
		default:
			label := p.newLabel()
			target := i + 1 + random.Intn(4)
			labelsAt[target] = append(labelsAt[target], label)
			p.line("set y, %s", label)
			if random.Intn(3) == 0 {
				p.instruction(isa.Instruction{Kind: isa.JumpOnCarry, Condition: isa.Condition(random.Intn(3))})
			} else {
				jump := isa.Instruction{Kind: isa.Jump, Condition: isa.Condition(random.Intn(4)), Source: anyRegister()}
				if jump.Condition == isa.Always {
					jump.Source = 0
				}
				p.instruction(jump)
			}
		}
	}
	p.line("halt")
	return p.String()
}
