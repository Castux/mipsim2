// Package archtest checks the package dependency rules from PLAN.md: core
// packages never import graphics or syscall/js, and internal imports only
// point downward.
package archtest

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	// Imported only so that changing any core package invalidates the cached
	// result of this test; the checks themselves run go list.
	_ "github.com/Castux/mipsim2/bitmap"
	_ "github.com/Castux/mipsim2/devices"
	_ "github.com/Castux/mipsim2/doc"
	_ "github.com/Castux/mipsim2/editor"
	_ "github.com/Castux/mipsim2/netlist"
	_ "github.com/Castux/mipsim2/runner"
	_ "github.com/Castux/mipsim2/sim"
)

const module = "github.com/Castux/mipsim2/"

// allowed lists, for each core package, the module packages it may import
// directly. Anything not listed here (ui, cmd, platform, spikes) is not core.
var allowed = map[string][]string{
	"bitmap":  {},
	"doc":     {"bitmap"},
	"netlist": {"bitmap", "doc"},
	"sim":     {"netlist"},
	"devices": {},
	"runner":  {"bitmap", "doc", "netlist", "sim", "devices"},
	"editor":  {"bitmap", "doc", "netlist", "sim", "devices", "runner"},
}

// Core packages must not depend on these at any depth.
var forbiddenPrefixes = []string{
	"github.com/hajimehoshi/ebiten",
	"github.com/ebitenui/",
}

// syscall/js is checked separately: on GOOS=js the standard library itself
// (time, os, ...) imports it, so only non-standard packages importing it
// directly count.
const syscallJS = "syscall/js"

func corePackages() []string {
	var pkgs []string
	for _, p := range []string{"bitmap", "doc", "netlist", "sim", "devices", "runner", "editor"} {
		pkgs = append(pkgs, module+p)
	}
	return pkgs
}

func goList(t *testing.T, env []string, args ...string) []string {
	t.Helper()
	cmd := exec.Command("go", append([]string{"list"}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list %v: %v\n%s", args, err, ee.Stderr)
		}
		t.Fatalf("go list %v: %v", args, err)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}

func TestCoreHasNoGraphicsDependencies(t *testing.T) {
	// Check both the host platform and wasm, since syscall/js only exists there.
	for _, env := range [][]string{nil, {"GOOS=js", "GOARCH=wasm"}} {
		lines := goList(t, env, append([]string{"-deps", "-f", "{{.ImportPath}} {{.Standard}} {{join .Imports \" \"}}"}, corePackages()...)...)
		var deps []string
		for _, line := range lines {
			fields := strings.Fields(line)
			pkg, standard, imports := fields[0], fields[1] == "true", fields[2:]
			deps = append(deps, pkg)
			if !standard {
				for _, imp := range imports {
					if imp == syscallJS {
						t.Errorf("%s imports syscall/js (env %v)", pkg, env)
					}
				}
			}
		}
		for _, f := range forbiddenPrefixes {
			for _, d := range deps {
				if strings.HasPrefix(d, f) {
					t.Errorf("core packages depend on %s (env %v); find the importer with: go list -deps -f '{{.ImportPath}}: {{.Imports}}' ./...", d, env)
					break
				}
			}
		}
	}
}

func TestCoreImportsPointDownward(t *testing.T) {
	lines := goList(t, nil, append([]string{"-f", "{{.ImportPath}} {{join .Imports \" \"}}"}, corePackages()...)...)
	for _, line := range lines {
		fields := strings.Fields(line)
		pkg := strings.TrimPrefix(fields[0], module)
		ok := map[string]bool{}
		for _, a := range allowed[pkg] {
			ok[a] = true
		}
		for _, imp := range fields[1:] {
			if !strings.HasPrefix(imp, module) {
				continue
			}
			dep := strings.TrimPrefix(imp, module)
			if !ok[dep] {
				t.Errorf("%s imports %s, which the dependency rules do not allow", pkg, dep)
			}
		}
	}
}
