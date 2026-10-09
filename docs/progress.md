# Progress

What works, what does not, and decisions made, in the order they happened. It is a log: later sections can supersede earlier ones (for example, ebitenui was chosen in M0 and dropped in the UI rework). For the current state, see the Status section of [PLAN.md](../PLAN.md) and the specification in [SPEC.md](SPEC.md).

## M0 Setup

**Works:**

- Module `github.com/Castux/mipsim2`, Go 1.25 (since moved to 1.26), Ebitengine v2.10.4.
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
- Spikes lived in `spikes/` as runnable references. They were removed in the cleanup before M9, once the canvas shader and the hand-rolled chrome had replaced them; they remain in git history.

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

**Decided by the owner: thin wires.** (Was: thin wires or isolated sources?) Compare the goldens of `thick_block`, `flush_wire`, `t_on_thick`, `ring_on_high` and `source_border`. Summary:

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

## M4 Runner and CLI

**Works:**

- `runner.Runner` wraps the simulator by name:
  - `Set` pins a net (high, low or float) or a bus (a number, or float);
  - `Value`, `Number` and `Format` read them;
  - `Names` lists every net and bus name, sorted;
  - a clock net (default `clock`) is pinned low at start; `HalfTick` toggles it and settles, and `Tick(n)` runs n full periods.

  Buses come from the `name_N` convention on full hierarchical names, built in net order with no map iteration.
- `mipsim-run FILE [--set a=5,b=high] [--ticks N] [--watch a,sum] [--clock NAME] [--trace] [--isolated]` takes `.mip` or `.fix` files. Flags may come before or after the file. It prints one `tick N: name=value ...` line per tick; nets print as 0, 1, z or x and buses as numbers, or `?` if a bit is not clean. Compile diagnostics go to stderr, and errors exit with status 1.
- `testdata/runner/adder4.fix`: a 4-bit ripple-carry adder, built hierarchically.
  - Each XOR is 4 NANDs; each full adder is 2 XORs and 3 NANDs, with two bridges; the adder is 4 full adders stacked, with the carry routed between them.
  - 88 transistors, no diagnostics.
  - All 512 inputs pass both through the Go API and through the CLI, on a `.mip` file saved from the fixture. This is the M4 completion check.

**Decisions:**

- A bus with a missing bit (`x_0`, `x_2` but no `x_1`) can be listed but not read or set; that is an error naming the bit.
- If a name is both a net and a bus base (`a` and `a_0`), the plain net wins in `Value`, `Set` and `Format`.
- `Runner.Settle` is plain simulator settling for now; device servicing joins it in M8.

## M5 Minimal editor

**Works:**

- `editor` (no graphics), tested with synthetic events:
  - Edit and simulate modes.
  - Pencil: a click toggles a pixel; a drag paints the value of the first pixel. Drags are rasterised 4-connected, so fast diagonal drags never leave diagonal-only joints. Holding alt locks the stroke to the dominant axis once the cursor is 2 pixels from the start.
  - Each stroke is one undoable command recording the definitions it touched; undo and redo are already wired in. Edits inside an instance change its definition.
  - Strokes recompile live while compiling takes under 8 ms, and on release otherwise; the renderer overlays the uncompiled stroke meanwhile.
  - Simulate mode refuses circuits with errors and names the first one. In simulate mode, left click pins high, right pins low and middle releases. Hover shows position, role, net, names, value and pin. Choosing a tool returns to edit mode.
  - Pan and zoom maths (`View`): integer zoom levels from 1× to 64× and filtered levels from 1/2 to 1/32. Zoom keeps the point under the cursor fixed.
- `ui`:
  - One Kage shader pass draws the whole canvas from a role/net texture plus a per-net state texture, using the M0 recipe.
  - Transistor centres light up when conducting; sources keep a tint of their kind in simulate mode; the hovered net is highlighted; a grid shows at 8× and above.
  - The status bar shows mode, tool, zoom, net, transistor and diagnostic counts, hover details and a key help line.
  - All keys live in one keymap table (`ui/keymap.go`).
- `mipsim [file.mip|file.fix]` opens a file and fits it to the window; ctrl+s saves `.mip` files in place. Flags for scripted checks and demos: `-simulate`, `-set`, `-zoom`, `-filter`, `-screenshot`, `-frames`. The M5 screenshots were rendered this way. They showed the pre-rework UI and were removed in the cleanup before M9 (in git history); `docs/screenshot.png` is the current one.
- File writes go through `platform.WriteFile`, which is asynchronous (callback). It has a native implementation and a wasm stub, so `ui` has no direct filesystem access.
- `go run ./internal/tools/synth` writes the synthetic inverter-chain circuit as a `.mip` file.

**Zoomed-out rendering spike:**

- Method: supersampling in the shader. Up to 4×4 samples per screen pixel are coloured, then combined with one of three filters, switchable with `b`:
  - plain average;
  - contrast boost (square root of coverage);
  - any-on.
- Measured on the 830k-pixel synthetic circuit in simulate mode, vsync off, 120 frames, including the per-frame state upload for 46k nets: **0.6–0.8 ms per frame** at 1/2, 1/4, 1/8 and 1/16 with every filter. The "colour then mip" candidate is not needed for speed.
- Screenshots at 1/4 and 1/16 with each filter (in git history, under `docs/m5/`):
  - Average reads as a dim, even texture.
  - Contrast boost keeps the high/low pattern of the chains visible.
  - Any-on saturates into solid blocks, as the plan predicted.
- **Default: contrast boost, confirmed by the owner.**

**Not verified by the agent:** interactive use. No one was at the keyboard, so mouse drawing, alt-drag, panning, wheel zoom and clicking pins were tested through the editor's unit tests and screenshots, not by hand. **The M5 completion check, "the owner can draw an inverter in the desktop editor and toggle it", needs the owner:** `go run ./cmd/mipsim test.mip`, draw, press `e`, click.

**Known limits:**

- Circuits wider or taller than 8192 pixels are not drawn; the texture would need tiling.
- Opening files still uses `os` in `cmd/mipsim`; M6 moves it into `platform` with an in-app Save As prompt.
- The status bar text is not yet an ebitenui panel; panels arrive with the diagnostics list in M6.

## M6 Editing conveniences

**Works** (`editor`, tested with scripted event sequences; every command's undo and redo are checked against document snapshots):

- **Select** (`s`): drag a rectangle. The selection lives in the definition where the drag started, clipped to it. It holds that definition's pixels and labels in the rectangle and the instances entirely inside it. A click without a drag selects the top-level instance under the pointer.
- **Move:** drag inside the selection; a ghost preview follows. Moves happen in the selection's own definition, through its orientation.
- **Copy, cut, paste** (`c`, `x`, `v`):
  - The clipboard holds pixels, labels and linked instances.
  - The paste preview follows the pointer; left click places, right click or escape cancels.
  - Pasted instances keep their definition (still linked) and get fresh IDs, plus a fresh name if theirs is taken.
  - Paste lands in the definition under the pointer.
  - The clipboard also converts to and from ASCII rows (`SetClipboardRows`, `ClipboardRows`).
- **Delete** (backspace or delete).
- **Mirror, rotate** (`m`, `r`): on the selection, or on the paste preview before it is placed. The top-left corner stays fixed and contained instances compose their orientation. A single selected instance therefore just rotates; its definition is unchanged.
- **Labels** (`n`): click a pixel, type, enter. Clicking an existing label edits it; an empty name removes it; invalid names are refused; escape cancels.
- **Undo and redo** for every command:
  - Strokes record individual pixel changes.
  - Every other command snapshots the definitions it touches. The document is validated after the command; an edit that breaks an invariant is reverted with the reason in the status bar.
  - Tested rejections: moving pixels into an instance, pasting an instance over another, pasting a definition into itself.
- `ui`:
  - The diagnostics panel lists errors and warnings on the right whenever there are any; click one to centre on it, wheel to scroll. Diagnostics are also outlined on the canvas.
  - Label names are drawn next to their pixels from 6× zoom; the full hierarchical name shows in the hover text.
  - Selection outline, move and paste ghosts, and the label being typed.
  - Native file handling through `platform`: ctrl+s saves in place, ctrl+shift+s opens a Save As prompt and ctrl+o an Open prompt, both typed in the status bar. A new document asks for a name on its first save.

**Decisions:**

- **Paste is opaque for pixels and labels**: the target rectangle is cleared first, as in pixel editors, so an old wire under the pasted area cannot merge with the new content. Paste never removes instances. Pasting or moving onto an instance breaks an invariant and is rejected, rather than silently deleting the instance.
- Shift-adding to a selection is not implemented; a selection is one rectangle. Multi-selection would complicate move, rotate and context handling, and nothing in the plan needs it before M7.
- Undo and redo clear the selection, because the selected definition may have changed shape.
- There is no system clipboard yet: Ebitengine has no clipboard API, and adding one means a new dependency. ASCII paste exists in the editor API for when it lands.
- Typing (labels and prompts) takes the keyboard, so single-letter keys type rather than switch tools.

**Not verified by the agent:** as for M5, interactive mouse and keyboard use (drag-selecting, moving, typing labels, prompts). The logic is covered by `editor` tests, and rendering was checked with screenshots of the diagnostics panel, markers and labels.

**Not done / next:** M7 (component UI). The plan says to pause here for the owner.

## After M6: owner feedback

- Thin wires kept (SPEC and PLAN updated).
- The canvas and chrome now use MiPSim v1's palette from its `style.css`:
  - white background with a light grid;
  - silver wires, red power, blue ground, purple transistor centres, half-transparent silver bridge gaps;
  - in simulate mode: pink high, light blue low, brown unstable; a light green outline on conducting transistors; pinned wires outlined `#ff7d7d` (high) or `#6d6dff` (low).

  Outlines follow the shape of the whole net, not each pixel (from 4× zoom), using a neighbour check in the shader. Pin state is now encoded in the state texture alongside the value.

## UI rework (before M7)

Agreed with the owner: tools in a left column, `space` to run or pause, and the rework done before M7.

**Works:**

- **Top bar:** the Edit/Simulate switch, the file name with a modified dot, and undo, redo, open and save buttons.
- **Left column:** icon buttons with name and key for every action of the current mode. Unavailable actions are greyed out (copy without a selection, run without a `clock` net). Hovering a button names it and its key in the hint line.
- **Simulate mode:**
  - Run/pause on a `space` tap; space held still pans.
  - Step (`.`) runs one simulator step. If nothing is queued, it first toggles the clock, so a clock edge can be watched propagating (v1's slow motion).
  - Slower and faster (`[`, `]`) halve or double the clock rate, from 0.25 to 1024 Hz. The clock rate and tick count are shown in the column.
  - The editor advances the clock from real elapsed time, at most 64 half ticks per frame, so a slow circuit at a high rate drops its backlog instead of freezing.
- **Right panel tabs:**
  - Watch: every labelled net and bus, in v1 colours, with pin markers. Click a net to pin it (left high, right low, middle release); click a bus to type a value.
  - Diagnostics.
- **Status lines:** a gesture hint for the current state, then position, counts and messages.
- The canvas is framed in v1 pink in simulate mode. Labels are placed on a free side of their pixel.
- Icons are 12×12 one-bit pixel art defined as `#`/`.` rows in `ui/icons.go`.
- `runner.ToggleClock` splits a half tick into "toggle" and "settle"; `editor` gains `Advance`, `Running`, `Hz`, `Modified`/`MarkSaved`, `Enabled` and `Hint`, all tested.

**Decision:** the chrome is hand-rolled immediate-mode drawing, not ebitenui. It is a handful of rectangles and texts driven by the keymap table, which keeps buttons and keys in one place and matches the v1 look without fighting a theme. ebitenui stays only in its M0 spike.

**Not verified by the agent:** clicking, hovering and typing in the real window. The layout and states were checked with screenshots in both modes, and the editor logic behind every button is unit-tested.

## Canvas tweaks (owner requests)

- **Conducting transistors** extend their purple into the two channel arms as flat triangles. The base is the edge shared with the centre and the tip reaches half way across the arm. This replaces v1's green outline. The shader finds the channel axis from the centre's one off neighbour, so the texture format is unchanged. The triangles show from 4× zoom; zoomed out nothing marks conduction (owner: fine).
- **Bridge gaps**, from 4× zoom, are drawn as a cross. Each beam is half a wire wide and coloured like the net it joins (north–south, with west–east on top), so a crossing shows which wire goes where. Below 4× the gap is a flat light grey.
- Fix: a transistor centre no longer takes the pin outline of its pinned gate wire.

## M7 Component UI

**Works** (`editor`, tested; every command is a validated snapshot, so it can be undone):

- **Make component** (`k`): the selection becomes a new definition (pixels, labels and the instances inside it), replaced by one instance. The editor then asks for the component's name.
- **Explode** (`K`): the selected instances are replaced by copies of their content, with their orientation applied. The definition stays in the palette.
- **Place**: from the Components tab, through the paste preview, so instances can be mirrored or rotated before placing.
- **Rename**: an instance with F2 (instance names must be valid names); a definition's display name with a right click in the palette.
- **Delete** an unused definition with a middle click in the palette. Used definitions are refused, with the instance count.
- **Resize**: a selected instance shows four square handles just outside its edges. Dragging one resizes the definition, and every instance keeps its content fixed in the world, whatever its orientation (tested on a plain and a rotated instance). Shrinking past content is rejected with the reason.
- **Cues**: instances are outlined in v1's light purple. The instance under the pointer is outlined in purple and labelled "path : component", and every other instance of the same definition is outlined too, so an edit that changes them all is visible.
- **Completion check:** `testdata/runner/adder8.fix` is an 8-bit adder built from 8 instances of one full adder. A runner test checks 1,500 random sums plus edge cases. `TestAdder8SharedEdit` erases one pixel inside one instance and checks that all 8 copies lose it, in the flattened circuit and in simulation (the sum stops reading as a number), and that undo restores it.

**Decisions:**

- The left column grew to 150 points to fit the component buttons.
- New definitions get IDs `part1`, `part2`, ... and a display name you type. IDs never change, so renaming a definition does not touch its instances.
- Clicking selects top-level instances only. Selecting a nested instance means dragging a rectangle inside its parent.

**Not verified by the agent:** the interactive feel of handle dragging and palette clicks. Logic and rendering were checked by tests and screenshots.

## Watch panel: editable buses (owner request)

Buses in the Watch tab are inline number fields, like v1's number inputs.

- Click a field to edit it in place. The first key typed replaces the value; decimal, `0x` hex and `0b` binary are accepted.
- Enter applies, and so does clicking anywhere else. Escape cancels.
- Up and down step the value by one (wrapping at the bus width) and apply it at once.
- Opening a field and leaving it without typing changes nothing, so clicking an output such as `sum` does not pin it.
- Middle click releases a bus.

## Pixel font and outlines (owner choice)

- All ui text uses one pixel font at one size: buttons, panel, status lines and canvas labels. The font is **raylib's default font** (zlib licence, Copyright Ramon Santamaria), decoded from raylib's `rtext.c` by `internal/tools/raylibfont` into glyph rows in `ui/fonts/raylib.go`. It is drawn at 2× (times the screen's scale factor), so text is pixel-exact.
- Pixelify Sans was chosen first, then dropped. It is a TTF drawn on a loose grid (about 91 units per pixel, with per-glyph offsets), and rasterising it never lined up exactly with the pixel grid: `v` and parentheses came out uneven. A true bitmap font avoids the problem by construction.
- Icons were removed; buttons are text with their key.
- Every box, button and tab has a single-pixel dark border (one ui pixel, so it scales with the text).
- Also compared and dropped: VT323, Silkscreen, Tiny5.
- Layout sizes (bars, rows, columns, tabs) are derived from the font's line height and measured text, not fixed pixel values.

## Selecting nested components (owner request)

A click with the select tool now picks the **innermost** instance under the pointer, in its parent's definition. Moving, rotating, deleting or exploding it therefore rearranges the component that contains it, everywhere that component is used. Clicking again inside the selection goes one level up, and wraps back to the innermost after the top level. Dragging a rectangle still selects in the definition where the drag starts. Tested on `hier.fix`: deleting `g1` inside `pair` removes it from both placed pairs.

## File browser (owner request)

Open (ctrl+o), Save As (ctrl+shift+s) and the first save of an untitled document open an in-app file dialog, drawn in the pixel style. The owner chose this over a native OS dialog for a consistent look and no new dependency.

- **Listing:** folders first, then `.mip` files (and `.fix` fixtures in Open, which load like documents; saving one writes a `.mip` next to it); hidden entries are skipped.
- **Choosing:** click to select, click again or press enter to choose. Up/down and page up/down move the selection. Alt+up, or backspace on an empty name, goes to the parent folder.
- **Typing:** typing edits the name field. A folder or absolute path, such as `D:\`, navigates there.
- **Save:** adds `.mip` when no extension is given, and asks for a second confirm before replacing an existing file. **Open** refuses missing files.
- Escape or a click outside the dialog cancels.
- The logic is in `ui/filebrowser` (no graphics, unit-tested on a real temp folder). Listing and existence checks go through `platform.ListDir` and `platform.Exists`; the wasm build has stubs, and the web port will use the browser's own picker.
- The top bar now shows only the file name; the window title keeps the full path.

## Optional instance names (owner request)

Instance names are now optional and empty by default: an unnamed instance is named after its component. A lone one takes the component's name (`fa`); several among siblings are numbered from 0 in placement order (`fa0` ... `fa7`, like bus bits), with `_` before the number when the base ends in a digit. Explicit names win, and automatic numbers skip them. The rule is implemented once in `doc.InstanceNames` and used for validation (effective names must be unique), flattening (hierarchical net names), selection messages and hover cues.

- Make component and the palette create unnamed instances. Renaming to an empty name returns to the automatic name. Paste keeps unnamed instances unnamed.
- Files omit `name` for unnamed instances; fixtures use `place DEF - X,Y`.
- Trade-off, noted in SPEC: automatic numbers shift when siblings are added or removed, so give an instance an explicit name to keep its nets' names stable.

## M8 Memory devices

Done criterion met: `testdata/runner/ram.fix` (a 256-byte RAM on 8-bit address and data buses, the data bus also feeding eight inverters to `q`) is written and read through its buses from the CLI (`TestRAMFromCLI`, including an init file beside a saved `.mip` and `--dump`), in the runner (`TestRAMReadWrite`, all 256 addresses) and in the editor (`TestRAMInEditor`: pins by clicking and setting buses as the Watch panel does).

- **`devices` package:** `Device` and `Bus` interfaces, `Level`, `Parse`, and the memory device. It imports no module package (archtest), so devices can be tested against a fake bus.
- **Runner:** separate pin layers for the user and for each device, combined low-wins; two devices driving one net is an error. `Settle()` now returns an error and runs the device loop (up to 16 rounds). `HalfTick`/`Tick` stop on device errors.
- **Floating select is idle**, not an error: otherwise every circuit with a memory errors before its control logic drives `select`. Unstable select still errors. Recorded in SPEC.
- **CLI:** devices from the document, init files relative to it, `--dump NAME`, exit 1 on a device error.
- **Editor:** device errors stop the clock with "stopped: ..." in the status bar. `Editor.ReadFile` loads init files (the ui reads them beside the open document through `platform.ReadFile`).
- **Memory tab:** hex grid with last read/write highlighted; click a word to edit it while paused. `mipsim -tab memory` opens on it (for screenshots).
- Not done by hand: interactive editing in a running window was checked with screenshots only.

## Unsaved-changes guard and New (owner request)

- **New** (ctrl+n, top bar) replaces the document with an empty untitled one.
- **Guard:** New and Open first check for unsaved changes and ask "Save before ...?" in a small modal: Save (enter or s), Discard (d), Cancel (esc or a click outside). Save goes through the file dialog when the document is untitled, and the pending action runs only after the save succeeds. A failed or cancelled save drops it.
- Fixed while there: the file dialog's backdrop was meant to dim the canvas but hid it, because `color.RGBA` is premultiplied (now `NRGBA`).
- Fixed just before: opening a file could show only labels and boxes. The canvas caches its pixel texture by the editor's compile count, which `ReplaceDocument` restarted.
- Closing the window goes through the same guard ("Save before quitting?"). The web port will need `beforeunload` instead.

## Devices tab (owner request)

Memories are configured in the editor, not only by editing the `.mip` JSON.

- **Devices tab** (edit mode): Add memory, delete, and an inline box per field (name, address bus, data bus, select, write, words, bits per word, read-only, init file). Edits are undoable `deviceEdit` commands on the document's device list.
- **Status line** per device, checked against the circuit with `runner.Check` (the same `Attach` check as at simulation start): "ok: addr_0..7, data_0..7, sel, we", or every problem at once ("init file prog.bin not found; missing nets cs"). Hovering shows the full text.
- **Init file:** typed, or chosen with the file dialog's new Pick mode (lists every file). Stored relative to the document's folder when inside it. Saving the document elsewhere does not move the path, so a relative init file may then be missing (the status line says so).
- **Reload:** in simulate mode, clicking a memory's header in the Memory tab re-reads its init file, for a rebuilt program.
- The check result is cached per document version and compile, so a file created on disk shows as found only after the next edit.

## Code review round (before M9)

Four parallel reviews: core, format and devices, editor and ui, performance. Fixed:

- **Format hardening** (before real files exist, since tightening later would reject them): limits on coordinates (±2^24), definition sizes (65536) and the flattened circuit (2^20 instances, 2^28 pixels), so a 3 KB file can no longer hang Flatten. Duplicate keys and trailing data are rejected, a BOM is accepted, and syntax errors have a line and column. Device kind and name come from one constructor and are validated (present, valid, unique). The root origin's x snaps to 16 for stable diffs.
- **Bugs:** pressing e mid-stroke crashed (stale runner netlist); a dialog opening mid-stroke lost the stroke from history; tool buttons were swallowed while typing; undo back to the saved state still showed modified; Watch-tab errors were dropped; stale pin-conflict errors; a 1-word memory wanted an address bit; `Change.Step` off by one; `Or` with itself; init paths that are absolute in the CLI.
- **Performance** (1M-pixel synthetic circuit): flatten 50 → 22 ms; worst-case tick 4.0 → 2.5 ms (about 400 ticks/s); instance cues 5–8 ms → 0.26 ms per frame; strokes on slow circuits no longer recompile every frame; clock work capped at 8 ms per frame; compile refuses a bounding box above 2^28 pixels (`E_TOO_LARGE`) instead of allocating gigabytes.
- **Architecture:** archtest also checks ui/filebrowser, forbids `os` in core packages and keeps the CLI headless.

Open, by decision: compile is still about 60 ms (+22 ms flatten) against the 50 ms budget; the canvas texture stops at 8192 px; ui (3000 lines) has no tests and duplicated widgets; undo snapshots copy whole definitions; wasm-port items (synchronous init-file reads, path-based document identity).

## Cleanup before M9

- Removed: the M0 spikes (and with them the ebitenui dependency), the M5 screenshots of the old UI, and a duplicate inverter fixture.
- One loader for `.mip` and `.fix` files (`fixture.LoadDocument`), used by the editor command, the file dialog and the CLI, which had three versions with slightly different rules. (Since replaced: fixtures are now `.mip` files.)
- README rewritten with a screenshot; PLAN has a Status section and matches what was built.

## Importing components (owner request)

The Components tab ends with an Import components... button. It opens the file dialog (`.mip` and `.fix`). The chosen document's components are then listed with check boxes.

- Checking a component also checks every component it uses, at any depth. Those show greyed and cannot be unchecked while something uses them. Hovering a row names what it uses.
- Imported definitions get fresh IDs. A display name already used in the document gets a suffix (`fa` becomes `fa_2`, then `fa_3`), and the status line lists the renames. Instances inside the imported components point at the imported copies, never at same-named originals.
- The whole import is one undoable command. Devices and the source's root are not imported.
- The selection logic is `editor.ImportSelection` and the copy is `Editor.Import`, both headless and tested on `adder8.fix` (importing into itself, so every name clashes).
- Not done: recognising that an identical component already exists and reusing it instead of importing a copy.

## One file format (owner request)

Test fixtures were a second format (`.fix`: drawing rows plus `# ` directives). They are now ordinary `.mip` documents, and the `.fix` parser is gone.

- **Format version 2** adds three optional fields: `note` (a description, replacing the fixture's first comment line), `tests` (a script: each step sets nets and buses, settles, checks expected values and optionally that some net is unstable) and `checks` (the pattern tests' assertions: diagnostic codes, net and transistor counts, same and different nets). Version 1 files load unchanged. The editor carries the fields through edits and saves.
- **Save uses Go's standard encoder** (`json.Encoder` with two-space indent) instead of a hand-written layout. Files are longer, since each label and instance field takes its own line, but there is less code to maintain. The root origin's x still snaps to 16.
- **Conversion:** the 34 fixtures were converted by a one-off tool that used the old parser, checking each round trip. `line`, `px` and relative `def` blocks became rows; `input` and `output` became plain labels, since a step says which names it sets and which it checks. Classification goldens are unchanged.
- **Loading from disk** is `internal/mipfile.Load` (core packages may not use `os`). The editor command, the file dialog and the CLI only take `.mip` files.
- **Newly run:** the `expect` lines of `adder4` and `adder8` were never executed before. The behaviour test now runs every document in `testdata/sim` and `testdata/runner` that has tests, in all 8 orientations.

## Typed cells (owner decision)

The one-bit pattern language made circuits bulky (an inverter was 3×11) and every attempt to shrink it ran into ambiguity: marker dots and hole transistors were tried on the `markers` branch and dropped. Cells are now typed, as in v1, with stricter rules than v1:

- **Kinds:** wire `#`, power `H`, ground `L`, transistor `T`, bridge `B`. Wire, power and ground conduct to each other in any shape (sources join their neighbours, so a gate's output can leave its power cell). A transistor needs exactly three neighbours (the odd one is the gate), a bridge exactly four; neither may touch another transistor or bridge. Breaking a rule is an error at the cell, which is drawn with a red outline.
- **Sizes:** inverter 2×3 (power, transistor, ground in a column), NAND 2×5, NOR 5×4, four-NAND XOR 20×14, gated NAND D latch 17×12 (`testdata/cells`, all with behaviour tests).
- **Storage:** `bitmap` keeps its one-bit occupancy plane and adds three kind bit-planes per chunk, allocated only when a chunk holds a cell other than wire. `At`/`Put`/`ForEachCell` carry kinds; `Get`/`Set` still mean "not empty" and "wire".
- **Files:** format version 3, rows of `.#HLTB`. Versions 1 and 2 are migrated on load by `doc.MigratePatterns`, per definition, with the old thin-wires rules: 3×3 squares become power, rings ground, T centres transistors, bridge gaps bridge cells, everything else wire. `internal/tools/upgrade` rewrites files in place. Every fixture was migrated this way, and all behaviour tests (including the exhaustive 4-bit adder through the CLI) pass unchanged.
- **Compiler:** pattern recognition is gone (no thick regions, rings, gaps or isolated-sources variant), so `E_THICK`, `E_RING_OVERLAP`, `E_GAP_AMBIGUOUS` and `--isolated` are removed. New codes: `E_TRANSISTOR_ARMS`, `E_BRIDGE_ARMS`; `E_ADJ_TRANSISTOR` and `E_BRIDGE_ARM` now mean touching cells. Netlist fixtures for removed rules were deleted; new ones cover the neighbour rules, thick wires and joining sources.
- **Editor:** Wire, Power, Ground, Transistor and Bridge buttons (keys 1 to 5) choose what the pencil paints and select it. A stroke starting on a cell of the chosen kind erases; otherwise it paints over whatever is there. Undo, copy, paste, rotate, mirror and component moves keep kinds.
- **Limit of the migration:** a pattern split across an instance edge is not recognised (`hier.mip`'s source spanning an edge was fixed by hand).
- **The gated NAND D latch** in `coolproc` (`nand_d_latch`) did not hold: with the gate low its pass transistors leave the NAND inputs floating, which a NAND reads as low. `testdata/cells/nand_d_latch.mip` is the textbook four-NAND circuit, which holds.
