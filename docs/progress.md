# Progress

What works, what does not, and decisions made, milestone by milestone. See [PLAN.md](../PLAN.md) for the milestones and [SPEC.md](SPEC.md) for the specification.

## M0 Setup

**Works:**

- Module `github.com/Castux/mipsim2`, Go 1.25, Ebitengine v2.10.4.
- Package skeleton for every package in the plan's layout.
- `internal/archtest` enforces the dependency rules: core packages never depend on Ebitengine, ebitenui or `syscall/js` (checked for the host and for wasm), and imports between module packages only point downward. Both checks were confirmed to fail on a deliberate violation.
- CI on GitHub Actions:
  - Linux: vet, staticcheck, `go test -race`, compile-only wasm build.
  - Windows: build and test.
- `go run ./cmd/mipsim` opens a resizable 1280×800 window with a blank canvas. `-screenshot out.png` saves a frame and exits; this is how rendering is checked without a person watching.
- `cmd/mipsim-run` is a stub until M4.

### Spike: Kage lookup texture — PASS

`go run ./spikes/kage` colours a 4096×4096 role/net-ID texture by looking up each pixel's net state in a 512×512 state texture (262,144 nets). It reads the result back and compares all 16.7M pixels with a CPU reference.

| Backend | Result | First draw + readback | State upload + full redraw |
| --- | --- | --- | --- |
| DirectX (Windows default) | PASS | 205 ms | 3.1 ms |
| OpenGL (forced) | PASS | 252 ms | 3.8 ms |

Findings the canvas shader must follow:

- Source images of different sizes are only allowed with `DrawTrianglesShader` and `//kage:unit pixels`; `DrawRectShader` and texel mode require equal sizes.
- Positions passed to `imageSrc1At` and the other image-1-to-3 functions are in **image 0's coordinate space**: Ebitengine converts them with `pos - origin0 + origin1`. To read texel (x, y) of image 1, pass `imageSrc0Origin() + vec2(x, y) + 0.5`, not `imageSrc1Origin() + ...`. Using `imageSrc1Origin` silently reads the wrong texel; this cost the first spike run.
- Pixel encoding used: R, G, B hold 24 bits, with role in the top 3 bits and net ID in the low 21 (up to 2M nets). Alpha is 255, so premultiplication does not interfere. Integer decoding in Kage (`int(floor(x*255 + 0.5))`, `/` and multiplication) is exact.

### Spike: ebitenui — PASS

`go run ./spikes/ebitenui` lays out a fixed-width tool panel (labels, buttons, a text input with placeholder, a status line) next to a custom-drawn pixel canvas, using the built-in dark theme. It took about 140 lines with no custom images: the bundled `themes.GetBasicDarkTheme()` covers buttons, labels and text inputs. `input.UIHovered` tells the canvas when the cursor is over a widget, so canvas clicks and widget clicks don't conflict.

Verified by screenshot (`-shot out.png`): layout, theme and text rendering. **Not verified by the agent:** interactive clicking and typing, since no one was at the keyboard. The owner can try `go run ./spikes/ebitenui`.

Decision: use ebitenui for panels. It adds `golang.org/x/image` (for the Go fonts) and `golang.org/x/exp` (an ebitenui dependency) to `go.mod`.

**Decisions:**

- The spec was split out of `PLAN.md` into `docs/SPEC.md`, and cross-references now use section headings instead of numbers.
- The dependency rules in `PLAN.md` were made precise to match `internal/archtest`. The runner may import `doc` and `netlist` (device configs, net names), and `devices` imports no module package.
- `.gitattributes` forces LF line endings, so Windows checkouts do not upset `gofmt`.
- Spikes live in `spikes/` and stay as runnable references; they are not part of the product.

## M1 Bitmap and document

**Works:**

- `bitmap`: unbounded one-bit images in 64×64 chunks. Includes raster-order iteration, bounds, crop, or, clear, `AnyIn`, a `Dense` packed view for the compiler, and the `#`/`.` row codec. Setting 1M pixels takes 4 ms; iterating them takes 2 ms.
- `doc`:
  - `Orient` and integer `Affine` maps, tested against a step-by-step reference for all 8 orientations and all 64 compositions.
  - The document model and `Validate`, which checks every invariant with located messages; each invariant has a rejecting test.
  - `Flatten`, which produces world pixels, hierarchical label names with their scope, and instance rectangles.
  - `Locate`, which finds the deepest definition under a world point.
  - `Clone` and `Equal`.
- `.mip` files:
  - Save is deterministic and diff-friendly (one row, label or instance per line; root first). Saving a loaded file is byte-identical.
  - Load is strict: unknown fields, bad rows, sizes, orientations and invariant violations are rejected with the definition and item named.
  - A 1.5M-pixel document round-trips (save 28 ms, load 20 ms).
- Property tests: 300 random oriented instance trees satisfy flatten/re-split, meaning every world pixel resolves to an on pixel of its definition and the counts match; 100 random documents survive save/load.
- `internal/fixture`: the ASCII fixture parser, with `def`, `root`, `label`, `input`, `output` and `place` directives. Other `# word` lines are passed through for the tests that use them (`expect`). `testdata/doc/inverter.fix` is the plan's inverter.
- `scripts/check.sh` runs every CI check locally (gofmt, vet, staticcheck, tests, wasm build) and fails on the first error.

**Decisions:**

- Root labels and instance positions are stored in world coordinates in `.mip` files; the root's `origin` only anchors its pixel rows and is recomputed on save. The plan's example had the label relative to the origin; SPEC now shows world coordinates.
- Label and instance names: ASCII letters, digits and underscores only. At most one label per pixel. Instance IDs are unique among siblings. (SPEC updated.)
- No `Version` field in the in-memory document; versions are a file concern, and migrations will go in `Load`.
- Device configs are kept as raw JSON plus `kind` and `name`; the runner will interpret them in M8.
- Fixtures declare `input` and `output` labels instead of plain `label`, so behaviour tests know what to pin and what to check. In fixtures, a line made only of `#` and `.` is always a drawing row; directives need `# ` with a space.
- The dependency check ignores `syscall/js` imported by the standard library (on wasm, `time` and `os` import it) and flags only non-standard packages that import it directly.

**Mistake:** commit `M1: document model...` was pushed with gofmt, staticcheck and archtest failures, because the command did not stop on errors. It was fixed in the next commit, and `scripts/check.sh` now gates commits.

## M2 Pattern compiler

**Works:**

- `netlist.Compile` implements both candidate rule sets behind `Options.Variant` (thin wires by default, isolated sources), in the spec's order: thick regions, rings, transistors, bridges, wires. It then builds nets with union-find, numbers nets and transistors canonically in raster order, applies hierarchical labels, and runs every lint rule in the spec table plus `E_DUPLICATE_NAME` and `E_SOURCE_BORDER`.
- Per-pixel data is indexed by raster rank among the on pixels (`bitmap.Dense.Index`), so memory scales with the number of on pixels, not the bounding box. Horizontal neighbours are `i±1`; vertical ones are looked up once per pixel.
- `RoleAt`, `NetAt` and `RoleMap` support rendering, hit-testing and goldens.
- 24 fixtures in `testdata/netlist`, each with a golden for both variants (role map, counts, diagnostics). Goldens are regenerated with `go test ./netlist -update` and were reviewed by hand. Covered:
  - every pattern;
  - every reachable lint code, each with a triggering fixture and near-misses (clean fixtures declare `# diag none`);
  - the variant differences;
  - the plan's inverter;
  - `hier.fix`: nested instances (a pair of inverters, placed twice, once rotated) with the expected hierarchical names, and a high source split across an instance edge.
- Orientation invariance: every fixture, in both variants, is wrapped in an instance in all 8 orientations. Roles move with the pixels, and net, transistor and diagnostic counts are unchanged.
- `internal/synth.InverterChains` generates processor-scale synthetic circuits: chains of inverter instances, labelled `in_r`/`out_r`.

**Performance** (152×152 inverter chains: 830k pixels, 46k nets, 23k transistors, this laptop):

| Stage | Time |
| --- | --- |
| Flatten | 45 ms |
| Compile | 67 ms (90 ms before precomputing vertical neighbours) |

Together this is about 110 ms, roughly twice the 50 ms budget. The profile is now spread evenly across the stages. Next steps, in order:

1. Flatten straight into the compiler's dense grid, skipping the chunked bitmap.
2. The plan's "compile each definition once and stamp" approach, if still needed.

Edits recompile on mouse-up, so this is tolerable for M5. It is tracked by `TestCompileBudget` and `BenchmarkCompile1M`.

**Decisions:**

- `E_BRIDGE_ARM` cannot trigger: a bridge arm that is a transistor centre needs its two side neighbours on, and those are the gap's diagonals, which makes the pattern `E_GAP_AMBIGUOUS`. The check is kept, and SPEC notes it.
- New error `E_DUPLICATE_NAME`: one full name on two different nets. In v1 the last label silently won; here name lookups must be unambiguous. **Flag for the owner:** is an error right, or should same-named labels connect their nets implicitly?
- Transistor channel ends are ordered west then east, or north then south, so `A`/`B` are deterministic.
- Pixels of a malformed thick region get role `X`, still join nets as wire (so hover and highlighting stay sensible), and are never transistors.
- The isolated-sources variant's border rules are specified precisely in SPEC (corners off; no two border pixels touching, even diagonally; rings inside thick wire are holes).

**Owner decision pending: thin wires or isolated sources?** Compare the goldens of `thick_block`, `flush_wire`, `t_on_thick`, `ring_on_high` and `source_border`. Summary:

- **Thin wires** rejects anything 2 pixels thick (`E_THICK`), which catches accidental blobs.
- **Isolated sources** allows thick buses, but a wire drawn flush along a source silently turns the source into plain wire (no error), and a ring touching a high source becomes wire too.

The thin-wires variant stays the default until decided.

## M3 Simulator

**Works:**

- `sim.Sim` ports v1's breadth-first `update` onto the netlist, as the spec describes:
  - a persistent FIFO queue; `Update`, `Pin` and `Unpin` only queue;
  - `Step` processes one entry and `Settle` drains the queue;
  - flip counting with the v1 threshold semantics (more than 20);
  - v1's sd1/sd2 push rule;
  - `Unstable` locking within a settle and clearing at the next one;
  - `Reset` as v1's setup;
  - an `OnChange` hook for traces.

  The group order is breadth-first from the popped net over adjacency lists in ascending transistor order, so runs are deterministic without sorting.
- Behaviour fixtures in `testdata/sim`: inverter, NAND, NOR, XOR (four NAND instances), bridge crossing into two inverters, SR latch (two NOR instances, one rotated 180°), D latch (pass transistors with complementary enables `E`/`En`), and a 3-inverter ring oscillator. Each fixture runs:
  - with its `expect` lines;
  - twice, comparing full traces;
  - again in all 8 orientations, wrapped in an instance.
- Unit tests:
  - a ring oscillator goes unstable, a pin that breaks the loop makes it stable, and releasing the pin oscillates again;
  - low beats a high pin;
  - single-stepping gives the same result as `Settle`;
  - `Reset` clears pins;
  - a netlist with errors is refused.
- Fixture format: `line X,Y X,Y` and `px X,Y` directives for drawing wires without full rows. The `any-unstable NAMES` check is used for oscillators. `internal/tools/classify` prints a fixture's role map, nets and diagnostics, as an aid for drawing circuits by hand.

**Performance:** 152×152 inverter chains (23k transistors, 46k nets), where every half tick flips every transistor: 4.5 ms per full tick, about 210 ticks/s. This is a worst case; a processor switches a fraction of its transistors per tick. Time goes to `flood` and `groupValue`, as expected. To revisit with the M9 workload.

**Decisions:**

- When a settle releases an `Unstable` net, it also queues the channel ends of the transistors that net gates. Without this, a ring oscillator broken by a pin kept a stale value: when a net goes `Unstable`, its transistors stop conducting but (as in v1) their channels are not re-evaluated, and releasing it to `Floating` does not count as a conduction change either. SPEC updated.
- Which net of an oscillating loop trips first depends on evaluation order, so oscillator fixtures check that *some* net is unstable rather than naming one.
- The D latch has complementary enable inputs instead of an internal inverter. The latch behaviour is the same, and the fixture stays small.
- Behaviour `expect` lines apply pins in the order written, then settle once. Tests avoid changing an enable and data in the same line, because the result then depends on queue order, as it would in hardware.

**Not done / next:** M4 (runner and CLI).
