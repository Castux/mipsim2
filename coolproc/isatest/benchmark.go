package isatest

import (
	"testing"

	"github.com/Castux/mipsim2/coolproc/isa"
)

// BenchmarkProgram is what the benchmarks run: sorting eight words, which
// loops, branches, and reads and writes memory.
var BenchmarkProgram = sortProgram([]uint16{9, 0x8000, 3, 0xffff, 0, 77, 3, 1234}).Source

// Benchmark measures a processor running BenchmarkProgram: booting is not
// counted. It reports instructions per second.
func Benchmark(b *testing.B, boot Boot) {
	program := Assemble(b, BenchmarkProgram)
	reference := isa.NewEmulator(program)
	reference.Run(1 << 20)
	instructions := 0
	for b.Loop() {
		b.StopTimer()
		m := boot(b, program)
		b.StartTimer()
		if n := m.Run(reference.Steps + 2); n != reference.Steps {
			b.Fatalf("ran %d instructions, want %d", n, reference.Steps)
		}
		instructions += reference.Steps
	}
	b.ReportMetric(float64(instructions)/b.Elapsed().Seconds(), "instructions/s")
}
