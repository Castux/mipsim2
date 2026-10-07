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

**Not done / next:** M2 (pattern compiler).
