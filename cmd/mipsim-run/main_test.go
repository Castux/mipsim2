package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Castux/mipsim2/doc"
	"github.com/Castux/mipsim2/internal/fixture"
)

var adderFix = filepath.Join("..", "..", "testdata", "runner", "adder4.fix")

func runCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, &out, &errOut)
	return out.String(), errOut.String(), code
}

// TestAdderFromCLI checks all 512 inputs of the 4-bit adder through the CLI,
// using a .mip file saved from the fixture.
func TestAdderFromCLI(t *testing.T) {
	fx, err := fixture.ParseFile(adderFix)
	if err != nil {
		t.Fatal(err)
	}
	data, err := fx.Doc.Save()
	if err != nil {
		t.Fatal(err)
	}
	mip := filepath.Join(t.TempDir(), "adder4.mip")
	if err := os.WriteFile(mip, data, 0o644); err != nil {
		t.Fatal(err)
	}
	for a := range 16 {
		for b := range 16 {
			for c := range 2 {
				set := fmt.Sprintf("a=%d,b=%d,cin=%d", a, b, c)
				out, errOut, code := runCLI(t, mip, "--set", set, "--watch", "sum,cout")
				if code != 0 {
					t.Fatalf("%s: exit %d: %s", set, code, errOut)
				}
				s := a + b + c
				want := fmt.Sprintf("tick 0: sum=%d cout=%d\n", s%16, s/16)
				if out != want {
					t.Fatalf("%s: got %q, want %q", set, out, want)
				}
			}
		}
	}
}

func TestClockAndTrace(t *testing.T) {
	inv := filepath.Join("..", "..", "testdata", "sim", "inverter.fix")
	out, errOut, code := runCLI(t, "--clock", "in", inv, "--ticks", "2", "--watch", "in,out")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	want := "tick 0: in=0 out=1\ntick 1: in=0 out=1\ntick 2: in=0 out=1\n"
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}

	out, _, code = runCLI(t, inv, "--clock", "in", "--ticks", "1", "--trace", "--watch", "out")
	if code != 0 || !strings.Contains(out, "out high -> low") || !strings.Contains(out, "out low -> high") {
		t.Errorf("trace output:\n%s", out)
	}
}

func TestCLIErrors(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{}, "usage"},
		{[]string{"nope.mip"}, "nope.mip"},
		{[]string{adderFix, "--watch", "nothing"}, "no net or bus"},
		{[]string{adderFix, "--set", "a"}, "name=value"},
		{[]string{adderFix, "--set", "a=99"}, "does not fit"},
		{[]string{adderFix, "--ticks", "1"}, "needs a clock"},
		{[]string{adderFix, "--ticks", "-3"}, "negative"},
		{[]string{adderFix, "--dump", "nope"}, "no memory device"},
	}
	for _, c := range cases {
		_, errOut, code := runCLI(t, c.args...)
		if code == 0 || !strings.Contains(errOut, c.want) {
			t.Errorf("%v: exit %d, stderr %q, want it to mention %q", c.args, code, errOut, c.want)
		}
	}

	bad := filepath.Join(t.TempDir(), "bad.fix")
	os.WriteFile(bad, []byte("###\n###\n###\n###\n"), 0o644)
	_, errOut, code := runCLI(t, bad)
	if code != 1 || !strings.Contains(errOut, "E_THICK") {
		t.Errorf("circuit with errors: exit %d, stderr %q", code, errOut)
	}
}

// TestRAMFromCLI is the M8 completion check from the CLI: writing a byte
// through the buses shows in the memory dump, and reading returns the
// contents loaded from an init file next to the document.
func TestRAMFromCLI(t *testing.T) {
	ram := filepath.Join("..", "..", "testdata", "runner", "ram.fix")
	out, errOut, code := runCLI(t, ram, "--set", "sel=1,we=1,addr=17,data=0xab", "--watch", "data,q", "--dump", "ram")
	if code != 0 {
		t.Fatalf("write: exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "tick 0: data=171 q=84") || !strings.Contains(out, "ram 0010: 00 ab 00") {
		t.Errorf("write output:\n%s", out)
	}

	// A .mip copy whose memory loads ram.bin from beside it.
	fx, err := fixture.ParseFile(ram)
	if err != nil {
		t.Fatal(err)
	}
	c := &fx.Doc.Devices[0]
	*c, _ = doc.NewDeviceConfig([]byte(strings.Replace(string(c.Raw), `"width":8`, `"width":8,"init":"ram.bin"`, 1)))
	data, err := fx.Doc.Save()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	mip := filepath.Join(dir, "ram.mip")
	os.WriteFile(mip, data, 0o644)
	init := make([]byte, 256)
	for i := range init {
		init[i] = byte(255 - i)
	}
	os.WriteFile(filepath.Join(dir, "ram.bin"), init, 0o644)
	for _, addr := range []int{0, 5, 200, 255} {
		out, errOut, code := runCLI(t, mip, "--set", fmt.Sprintf("sel=1,we=0,addr=%d", addr), "--watch", "data")
		if code != 0 {
			t.Fatalf("read %d: exit %d: %s", addr, code, errOut)
		}
		if want := fmt.Sprintf("tick 0: data=%d\n", 255-addr); out != want {
			t.Errorf("read %d: %q, want %q", addr, out, want)
		}
	}

	// A device error stops the run with a message.
	_, errOut, code = runCLI(t, ram, "--set", "sel=1,we=0")
	if code != 1 || !strings.Contains(errOut, "floating") {
		t.Errorf("floating address: exit %d, %q", code, errOut)
	}
}
