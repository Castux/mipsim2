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

**Not done / next:** M1 (bitmap and document model).
