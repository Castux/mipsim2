# MiPSim v2

A switch-level n-MOS circuit simulator where circuits are drawn as one-bit pixel art. It is a Go rewrite of [MiPSim](https://github.com/Castux/mipsim), adding reusable components and attachable memory.

Circuits are made of a single kind of pixel. What a pixel does comes from the pattern around it:

```
High source   Low source    Transistor    Bridge        Wire junction
  ###           ###           .#.           .#.           .#.
  ###           #.#           ###           #.#           ###
  ###           ###           ...           .#.           .#.
```

A 3×3 square pulls its wire high, and a 3×3 ring pulls it low (low wins). A T shape is a transistor whose stem is the gate. A gap surrounded by four wire ends lets two wires cross. The full rules are in [docs/SPEC.md](docs/SPEC.md).

## Status

In development. Done so far:

- the document model (`.mip` files, components with rotation and mirroring);
- the pattern compiler, which reports located errors and warnings;
- the switch-level simulator;
- a headless runner and CLI;
- a desktop editor: draw, select, move, copy and paste, rotate and mirror, labels, undo, a diagnostics panel, and simulate mode.

Still to come: the editor UI for making and placing components, memory devices, and the web version. See [PLAN.md](PLAN.md) for the milestones and [docs/progress.md](docs/progress.md) for details.

## Building

You need Go 1.25 or later. On Linux, Ebitengine also needs the X11 and OpenGL development packages; see the [Ebitengine install guide](https://ebitengine.org/en/documents/install.html).

```sh
go test ./...                                   # run the tests
go run ./cmd/mipsim circuit.mip                 # edit a circuit (created on first save)
go run ./cmd/mipsim testdata/sim/xor.fix        # open a test fixture
go run ./cmd/mipsim-run testdata/runner/adder4.fix --set a=3,b=5,cin=0 --watch sum,cout
scripts/check.sh                                # every check CI runs
```

In the editor every action is a button showing its key, and the bottom line explains what the mouse does in the current mode. In simulate mode, left click pins a wire high, right click pins it low and middle click releases it; tap space to run the clock.

## Layout

| Path | Contents |
| --- | --- |
| `bitmap/` | Unbounded one-bit images |
| `doc/` | Documents, components, orientations, `.mip` files |
| `netlist/` | Pattern compiler: pixels to nets and transistors, with diagnostics |
| `sim/`, `runner/`, `devices/` | Simulator, clocked runner, memory devices (in progress) |
| `editor/`, `ui/`, `platform/` | Editor logic, Ebitengine front end, file access (in progress) |
| `cmd/` | `mipsim` (editor) and `mipsim-run` (headless CLI) |
| `testdata/` | ASCII circuit fixtures and classification goldens |

## License

[MIT](LICENSE)
