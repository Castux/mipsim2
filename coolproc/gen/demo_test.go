package main

import (
	"os"
	"testing"
)

func TestDemo(t *testing.T) {
	src, err := os.ReadFile("../demo.asm")
	if err != nil {
		t.Fatal(err)
	}
	m := boot(t, string(src))
	m.run(1000)
	w := m.ram.Words()
	a, b := uint64(0), uint64(1)
	for i := range 20 {
		if w[i] != b {
			t.Fatalf("ram[%d] = %d, want %d", i, w[i], b)
		}
		a, b = b, a+b
	}
	if w[20] != 0 || w[0xffff] != 20 {
		t.Errorf("ram[20] = %d, ram[0xffff] = %d", w[20], w[0xffff])
	}
}
