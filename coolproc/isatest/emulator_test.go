package isatest

import (
	"testing"

	"github.com/Castux/mipsim2/coolproc/isa"
)

// emulated is a Machine that is the emulator itself: running the suite on
// it checks the suite's programs and their own checks.
type emulated struct{ *isa.Emulator }

func (e emulated) Run(limit int) int {
	if !e.Emulator.Run(limit) {
		return -1
	}
	return e.Steps
}

func (e emulated) ProgramCounter() uint16 { return e.Emulator.ProgramCounter }
func (e emulated) Carry() bool            { return e.Emulator.Carry }

func (e emulated) Memory() []uint64 {
	words := make([]uint64, len(e.Emulator.Memory))
	for address, v := range e.Emulator.Memory {
		words[address] = uint64(v)
	}
	return words
}

func bootEmulator(_ testing.TB, program []uint16) Machine { return emulated{isa.NewEmulator(program)} }

func TestSuiteOnEmulator(t *testing.T) { Run(t, bootEmulator) }

// BenchmarkEmulator measures the reference emulator on the benchmark
// program.
func BenchmarkEmulator(b *testing.B) { Benchmark(b, bootEmulator) }
