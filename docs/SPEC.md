# MiPSim v2 specification

This is the authoritative specification of the pixel language, the simulation model, components, devices and the file format. It was split out of `PLAN.md` in M0. Any behaviour change updates this file and adds a fixture in the same commit.

## Tile semantics (spec)

The circuit is a set of "on" pixels on an unbounded integer grid. Connectivity is orthogonal only: diagonal neighbours never connect. Wires are one pixel wide. Every on or off pixel classifies into exactly one role below, or produces a diagnostic.

The five patterns, `#` on and `.` off (the centre pixel is the one being classified):

```
High source   Low source    Transistor    Bridge        Wire junction
  ###           ###           .#.           .#.           .#.
  ###           #.#           ###           #.#           ###
  ###           ###           ...           .#.           .#.
```

- **High source:** a 3×3 fully on square. Drives its net high.
- **Low source:** a 3×3 ring (8 on, centre off). Drives its net low.
- **Transistor:** an on pixel with exactly three on orthogonal neighbours, outside any source. The two collinear arms are the channel (source/drain, interchangeable); the perpendicular arm is the gate. This is deterministic, unlike v1, which picked randomly when ambiguous. In the picture above, left and right are the channel and the top arm is the gate.
- **Bridge:** an off pixel whose four orthogonal neighbours are on and four diagonal neighbours are off. Connects north to south and west to east, independently.
- **Wire:** every other on pixel. A cross with its centre on is a plain 4-way junction; a 3-way split is drawn as a cross with one arm left as a one-pixel stub.

A source connects to any on pixel orthogonally adjacent to its outer edge. A transistor arm may touch a source (the arm pixel is a wire pixel adjacent to the source's edge). A transistor centre directly adjacent to a source's edge always forms a 2×2 block with it and is `E_THICK`, except at a source corner.

Because recognition is purely local, any 3×3 loop of wire is a low source, and a wire grid with a pitch of 2 pixels produces `E_RING_OVERLAP`. This is intended; the classification goldens include both cases and the user guide calls it out.

### Recognition order

The compiler classifies in this order so the patterns cannot steal pixels from each other:

1. **Thick regions.** Find every 2×2 block of on pixels and union the overlapping ones into thick regions. Each thick region must be exactly a 3×3 square, which becomes a high source. Anything else is error `E_THICK` (for example a 3×4 block, or a wire running flush along a source's side).
2. **Rings.** Every 3×3 window with 8 on pixels and an off centre is a low source. Overlapping rings, or a ring sharing pixels with a high source, is error `E_RING_OVERLAP`.
3. **Transistors.** Among remaining on pixels, those with exactly 3 on orthogonal neighbours (counting source pixels as on).
4. **Bridges.** Off pixels with 4 on orthogonal and 0 on diagonal neighbours. An off pixel with 4 on orthogonal neighbours and 1 to 3 on diagonals is error `E_GAP_AMBIGUOUS`, because the diagonal pixel would silently short two arms.
5. **Wires.** Everything else that is on.

### Lint rules

These keep the language unambiguous. Errors block simulation; warnings do not.

| Code | Level | Condition |
| --- | --- | --- |
| `E_THICK` | error | A thick region that is not exactly 3×3 |
| `E_RING_OVERLAP` | error | Rings overlapping each other or a high source |
| `E_GAP_AMBIGUOUS` | error | Bridge-like gap with on diagonals |
| `E_ADJ_TRANSISTOR` | error | A transistor arm is itself a transistor centre |
| `E_BRIDGE_ARM` | error | A bridge arm pixel is a transistor centre. Unreachable in practice: such an arm needs both of its side neighbours on, and those are the gap's diagonals, so the pattern is `E_GAP_AMBIGUOUS` first. Kept as a defensive check |
| `E_LABEL_OFF_NET` | error | A label sits on an off pixel or a transistor centre (for example after the pixel under it was erased) |
| `E_DUPLICATE_NAME` | error | The same full name labels two different nets, so looking it up would be ambiguous. Two labels with one name on the same net are fine |
| `W_CHANNEL_SHORT` | warning | A transistor's two channel arms are already the same net |
| `W_GATE_ON_CHANNEL` | warning | Gate net equals a channel net |
| `W_FLOATING_GATE` | warning | Gate net has no source, no label and no transistor channel feeding it |
| `W_DIAGONAL_TOUCH` | warning | Two on pixels in different nets touch only diagonally, unless both are arms of the same transistor or the same bridge |
| `W_SOURCE_SHORT` | warning | One net contains both a high and a low source (it is permanently low) |
| `W_LABEL_CONFLICT` | warning | One net carries two different labels from the same scope (both kept as aliases). Labels from different levels of the hierarchy, such as `a` in the parent and `g1.a` on an instance port, are the normal way to connect components and do not warn |

**Two candidate rules, compared in M2; decided: thin wires.** The owner chose thin wires after the M2 comparison. The isolated-sources variant stays in the compiler as an option (`netlist.IsolatedSources`, `mipsim-run --isolated`) only so the comparison fixtures keep documenting the difference; the editor always uses thin wires. The rules above are derived from the owner's base ideas (3×3 square, 3×3 ring, T, cross without centre). Implement recognition behind a switch with two variants and pick one from fixtures:

- **Thin wires (default):** `E_THICK` as above. Every wire is one pixel wide; simple and unambiguous.
- **Isolated sources:** a source must have an empty one-pixel border, except for single-pixel wire attachments that are not adjacent to each other. Thick regions elsewhere are then legal wire, allowing wide buses for visual effect or more elegant components. In this variant, a pixel is only a transistor if neither it nor its three arms belong to a 2×2 block, and a thick region that matches a source shape but lacks the border is error `E_SOURCE_BORDER`.

As implemented in M2, the isolated-sources variant also follows these rules:

- The border is the 16 cells around the 3×3. The 4 corner cells must be off. The 12 side cells may hold attachments, but no two on border cells may touch, even diagonally (two attachments on either side of a corner would both touch the corner pixel).
- Because two side-by-side attachments form a 2×2 block with the source, they make the whole region bigger than 3×3. It is then classified as thick wire, not as a source with an error. This is the variant's main hazard: a wire drawn flush along a source silently turns the source into wire.
- A ring is only a low source if none of its pixels is in a 2×2 block; inside thick wire, a one-pixel hole is just a hole.

The fixture set must include cases that the two variants classify differently (a 3×4 block, a wire flush along a source, a T on the edge of a thick wire), so the comparison is concrete. These are `thick_block`, `flush_wire`, `t_on_thick`, `ring_on_high` and `source_border` in `testdata/netlist`, each with goldens for both variants.

Classification goldens print each pixel's role as one letter: `H` high source, `L` low source, `T` transistor centre, `B` bridge gap, `w` wire, `X` pixel of a malformed thick region (`E_THICK`), `.` off.

## Simulation model (spec)

The simulator runs on a compiled netlist, never on pixels. It keeps v1's breadth-first algorithm from `Simulator:update`, ported with deterministic ordering.

### Netlist

The compiler unions wire and source pixels by orthogonal adjacency into **nets**. Transistor centres do not join their neighbours. Bridge gaps join their north arm to their south arm and their west arm to their east arm. NetIDs are assigned canonically: nets are numbered in raster order (y, then x) of their first pixel, and transistors in raster order of their centre. The same bitmap always yields the same netlist, whatever order the chunks were stored or the pixels were visited in. The output is:

```go
type NetID int32

type Net struct {
    Drive  Drive    // None, High, Low (from sources it contains)
    Names  []string // hierarchical labels, see Components and instances
}

type Transistor struct {
    Gate NetID
    A, B NetID // channel, interchangeable
}

type Netlist struct {
    Nets        []Net
    Transistors []Transistor
    GatedBy     [][]int32 // net -> transistors it gates (CSR in practice)
    ChannelOf   [][]int32 // net -> transistors it is a channel of
    PixelNet    PixelMap  // for rendering and hit-testing
    Diagnostics []Diagnostic
}
```

### State and values

Each net holds one of `Floating`, `High`, `Low`, `Unstable`. A transistor conducts only when its gate net is `High`; a `Floating`, `Low` or `Unstable` gate does not conduct, as in v1. A **group** is the set of nets reachable from a net through conducting transistors. A group's value is `Low` if it contains a low source or a net pinned low, else `High` if it contains a high source or a net pinned high, else `Floating`. Low wins, as in v1, because high sources model pull-up resistors.

### Update (breadth-first)

The simulator owns a persistent FIFO queue of nets. `Update(net)` only pushes onto it; nothing is evaluated until the queue is drained. `Step()` processes one queue entry:

1. Pop a net; flood-fill its group through conducting transistors; compute the group value.
2. If any transistor gated by a net in the group has a flip count above the flip threshold (default 20, configurable; "above" as in v1, so the 21st flip trips it), the value is `Unstable`.
3. Assign the value to every net in the group, skipping nets already `Unstable` (they stay locked for the rest of this settle).
4. For each net whose value changed, for each transistor it gates whose conduction flipped: increment its flip count and push channel net A; push B too only when the transistor turned off (when it turns on, A and B are in one group already). This is v1's `sd1`/`sd2` rule. When a gate goes `Unstable`, its transistors stop conducting but their channel nets are not pushed, as in v1.

`Settle()` loops `Step()` until the queue is empty, then resets all flip counts to zero. A settle is therefore the unit v1 calls one `update()` call.

**Unstable clears at the next settle (differs from v1).** When a settle begins (the first `Step()` after the queue was empty), every `Unstable` net becomes `Floating` and is pushed, in ascending ID order, after the entries already queued; after each such net, the channel ends (A then B) of every transistor it gates are pushed too. When the net went `Unstable`, those transistors stopped conducting without their channels being re-evaluated (v1's rule), so the channels may hold stale values; without this step a circuit whose oscillation is broken by a pin can stay wrong. A real oscillator goes `Unstable` again within that settle; a transient glitch recovers. v1 kept `Unstable` until a full reset. Duplicate entries in the queue are allowed, as in v1; deduplicating is an optimisation that must not change traces.

The editor's slow-motion view calls `Step()` and redraws between calls, replacing v1's coroutine yield. A step is one queue entry, which is coarser than v1's yield per component; that is acceptable.

**Determinism is a hard requirement.** v1 iterates Lua tables with `pairs`, so ties resolve arbitrarily; Go map iteration is randomised on purpose. Use slices indexed by `NetID`, iterate in ascending order, and never range over a map in `bitmap/`, `doc/` (flatten), `netlist/`, `sim/` or `runner/`. The chunked bitmap iterates pixels in raster order (y, then x), whatever order they were set in; flatten walks instances in their slice order. Two runs of the same circuit and inputs must produce identical traces.

### Pins and clock

`Pin(net, High|Low)` forces a net and `Unpin(net)` releases it; both only record the pin and call `Update(net)` (push). The caller decides when to `Settle()` or `Step()`. The editor settles immediately after a click, except in slow motion; the runner settles once after all pins for a half period are set. Pinned nets act as sources in step 1, and low still wins, so pinning high cannot override a conducting path to ground (v1 behaviour). The runner drives a clock by toggling a pinned net, by default the one labelled `clock` (configurable): each half period is pin, then the settle loop in Memory devices and buses. `Tick(n)` runs n full periods. Initial setup sets every net `Floating`, pushes every source net in ascending ID order, and settles, as v1 does.

The same netlist and simulator back the editor, the CLI and the tests. Nothing in `sim/` knows about pixels or graphics.

## Components and instances

A document is a tree of **definitions**. Each instance is a placement of a definition; there is no privileged "original", only the shared definition that every instance displays and edits.

```go
type Definition struct {
    ID        DefID
    Name      string
    W, H      int            // the rectangle; pixels live in [0,W)x[0,H)
    Pixels    Bitmap         // local coordinates
    Labels    []Label        // {X, Y, Name}, on wire or source pixels
    Instances []Instance     // children
}

type Instance struct {
    ID   InstID
    Def  DefID
    X, Y   int    // top-left of the placed (oriented) rectangle, in the parent's coordinates
    Orient Orient // one of the 8 rotations and mirrorings of the square
    Name   string // optional: empty means named after its component (see "Instance names")
}

type Orient struct {
    Flip bool  // mirror left-right in local coordinates first: x -> W-1-x
    Rot  uint8 // then rotate clockwise by Rot*90 degrees (0..3)
}

type Document struct {
    Root    DefID         // unbounded: W,H ignored, pixels in world coordinates
    Defs    map[DefID]*Definition
    Devices []DeviceConfig // see Memory devices and buses
}
```

**Orientation.** Instances can be rotated and mirrored from v2.0. An instance's placed rectangle is W×H, or H×W when `Rot` is odd. All five patterns are symmetric under every rotation and mirroring, so recognition needs no orientation logic: flatten applies the transform, and the compiler sees ordinary pixels. Orientations compose: a rotated instance inside a mirrored instance is transformed by both, applied from the innermost outward. All geometry (flatten, hit-testing, label positions, rectangle checks) goes through the `Orient` type in `doc` (`Then`, `Inverse`, `Size`) and the integer `Affine` maps built from it, tested exhaustively over all 8×8 pairs.

**Invariants**, checked after every edit command and on load (rectangles are the placed, oriented ones):

- The definition graph is acyclic (a definition cannot contain itself at any depth).
- Sibling instance rectangles do not overlap.
- A parent has no pixels or labels inside a child instance's rectangle. Instance rectangles are opaque.
- Every child instance lies fully inside its parent definition's rectangle (the root is unbounded).
- The root definition is never instanced.
- Effective instance names (see "Instance names") are unique among siblings. Explicit instance names and label names use only ASCII letters, digits and underscores (dots are reserved for hierarchical names).
- Instance IDs are non-empty and unique among siblings.
- At most one label per pixel within a definition.

Definitions with no instances stay in the document and the palette until the user deletes them from the palette.

**Flattening.** For compilation and rendering, the document flattens into one world bitmap. The instance path behind a world pixel is not stored per pixel; it is found on demand by descending the instance tree, which is the same walk editing uses. Patterns spanning an instance boundary are legal and recognised normally, since recognition runs on the flat bitmap. A wire in the parent touching a child's edge pixel connects. Store the world bitmap in chunks (64×64 bits per chunk in a map keyed by chunk coordinate) so the canvas is unbounded and empty space costs nothing.

**Editing.** Every editor action resolves a world coordinate to (definition, local coordinate) by descending to the deepest instance whose rectangle contains it, mapping through each instance's inverse orientation on the way down. A pixel edit therefore changes the definition (in its own unrotated frame), and every instance updates on the next flatten. Root-level pixels resolve to the root definition. Commands that would break an invariant are rejected with a status message.

**Component operations:**

- *Make component*: selection rectangle becomes a new definition; its pixels, labels and fully contained instances move into it; one instance with identity orientation replaces them in place.
- *Rotate, mirror*: on a selected instance, composes with its orientation and keeps the rectangle's top-left corner in place (rotating about the centre cannot return to the start after four turns when W and H differ in parity); on a mixed selection, transforms pixels and labels and composes every contained instance's orientation and position.
- *Place instance*: from a component palette, or by pasting a copied instance (paste keeps the link by default).
- *Explode*: replace one instance with a copy of its content in the parent, with its orientation applied; the definition is unaffected.
- *Resize definition*: drag an instance's edge; applies to all instances; rejected if it causes overlaps or cuts off content. The dragged edge is mapped through the instance's orientation to an edge of the definition's local frame. Growing or shrinking the local right or bottom edge only changes `W`/`H`. Moving the local left or top edge shifts the definition's content by the delta, and every instance's `X`/`Y` is adjusted (through its own orientation) so its content stays fixed in the world, all in one undoable command.
- *Rename* instance or definition.

**Instance names.** An instance name is optional, and usually left empty. An unnamed instance is named after its component: the component's display name, with characters other than letters, digits and underscores turned into underscores. A lone unnamed instance of a component among its siblings takes that name exactly (`fa`). Several unnamed instances of one component are numbered from 0 in placement order (`fa0` ... `fa7`, matching bus bit numbers), with an underscore before the number when the base name ends in a digit (`part1_0`). Explicit names always win, and automatic numbers skip names taken by an explicit sibling. Automatic names can change when siblings are added or removed, and hierarchical net names with them; give an instance an explicit name to pin its nets' names. Make component and placing from the palette create unnamed instances; renaming one to the empty name returns it to its automatic name. In `.mip` files the `name` field is omitted for unnamed instances, and in fixtures `place DEF - X,Y` places one.

**Labels in components.** A label inside a definition repeats in every instance, so net names are hierarchical: the instance path joined with dots, then the label, e.g. `alu.add3.sum_2`. Root labels have no prefix. The `name_N` number convention applies to the full name (N without leading zeros: `a_01` is a plain name), so `alu.result_0` to `alu.result_7` read as the number `alu.result`. A net touched by several labels keeps all of them as aliases.

**Performance.** Recompile the whole flat bitmap after each edit command to start with. A pencil or line drag is one command and recompiles on mouse-up, not on every pixel. The budget is under 50 ms for 1 million on pixels on a laptop; profile before optimising. If it is exceeded, compile each definition once and stamp its netlist per instance, with a boundary pass to stitch nets and recognise patterns along instance edges.

## Memory devices and buses

Devices are Go objects that read and pin labelled nets between settles, generalising `BFHost.handleIO` from v1. The circuit exposes buses as `name_N` labels; the document lists which device attaches to which names.

```go
type Bus interface { // the nets a device sees, by label name
    Has(name string) bool
    Value(name string) Level   // Floating, High, Low or Unstable
    Pin(name string, l Level)  // the device's own pin layer
    Unpin(name string)
    Pinned(name string) Level
}

type Device interface {
    Name() string
    Kind() string
    Attach(bus Bus) error // check the nets exist; called once by runner.New
    // Called after the circuit settles. Reads nets, pins or unpins nets.
    // Returns true if it changed any pin, so the runner settles again.
    Service(bus Bus) (changed bool, err error)
}
```

The package `devices` imports no other module package; the runner adapts its nets to `Bus`. Device configurations are parsed by `devices.Parse(kind, raw, readFile)`, where `readFile` loads init files relative to the document (the web port must preload them, since it is synchronous).

**Pin layers.** The runner keeps one pin layer for the user (clicks, `--set`, the clock) and one per device. A net's effective pin combines the layers with low winning, like conducting paths. Two devices pinning the same net at once is a runtime error naming both. Releasing a pin in one layer leaves the others in force.

**Memory device**, the one shipped in v2.0, configured per document:

```json
{
  "kind": "memory",
  "name": "data_ram",
  "addr": "port_addr",
  "data": "port_data",
  "select": "port_sel_mem",
  "write": "port_write",
  "words": 256,
  "width": 8,
  "init": "hello.bin",
  "readonly": false
}
```

`words` is 1 to 2^24, `width` 1 to 64. `init` is optional: a binary file read little-endian, ceil(width/8) bytes per word, from address 0; a shorter file leaves the rest zero. Unknown fields are rejected.

Semantics, matching v1's BF host: if `select` is not high, the device unpins `data`. If `select` is high and `write` is low, it pins `data` to the word at `addr`. If both are high, it writes in two phases: if it is still pinning `data`, it unpins and returns `changed=true` without storing, so the runner settles with the circuit alone driving the bus; on the next service it reads `data` and stores it. Writes are level-sensitive, taken at each settle point. Several devices may share a bus; two devices driving the same bus at once is a runtime error reported by the runner.

On load, the runner checks that the nets `addr_0` to `addr_{k-1}` exist with k = ceil(log2(words)) (none for a 1-word memory), that `data_0` to `data_{width-1}` exist, and that `select` and `write` exist (all names with their configured prefixes). A missing net rejects the configuration with an error naming it.

A memory device only reads bits it needs: `addr` while `select` is high, and `data` on the store phase of a write. If any bit it needs is `Floating` or `Unstable`, the runner stops with an error naming the device, the net and the tick; the editor pauses and shows it in the status bar. This is stricter than v1, which read anything not high as 0. An `Unstable` `select` is the same error, and so is a `Floating` or `Unstable` `write` while selected. A `Floating` `select` counts as not selected, so a memory whose select line is not yet driven sits idle (owner-approved relaxation of the M0 rule). An address at or beyond `words` is an error, and so is a write to a `readonly` memory.

**Settle loop** in the runner: settle; service every device in declaration order; if any reported a change, settle again; give up after 16 rounds with an error naming the devices involved. `runner.New` attaches devices but does not service them; the first `Settle()` does.

Configuring: in edit mode, the panel's Devices tab lists the document's devices. Add memory creates a 256-byte RAM on `addr`, `data`, `sel` and `we` with a free name (`ram`, `ram1`, ...). Each field is an inline box (read-only toggles on click; the init file can be typed or chosen with the file dialog, and is stored relative to the document's folder when inside it). Every change is one undoable document edit. Below each device a status line says what it attaches to, or lists every problem: missing nets, an unreadable init file, a duplicate name, an invalid value. It is the same check `runner.New` makes. In simulate mode, clicking a memory's header in the Memory tab reloads its init file.

Tools: `mipsim-run --dump NAME` prints a memory's words in hex after the run, 16 per line (`ram 0010: 00 ab ...`). Fixtures attach devices with `# device {JSON}`. In the editor, the panel's Memory tab (simulate mode, when the document has memories) shows each memory as a hex grid, 8 words per row, with the last access highlighted (light blue read, pink write). While the clock is paused, clicking a word edits it (hex, or `0x`, `0b`, `0d` prefixed); the circuit settles at once.

Later device kinds (not v2.0):

- **Console I/O:** byte in/out on a strobe.
- **Host callback:** a device for Go tests.
- **Video memory:** a memory device (same bus semantics as above) whose contents are also shown as an image. Extra parameters: `screen_width`, `screen_height`, `depth` (bits per pixel: 1, 2, 4 or 8) and an optional palette. Pixels are packed row-major from address 0, least significant bits first within a word, and the configuration is rejected unless `words × width ≥ screen_width × screen_height × depth`. The editor shows it as a scalable screen panel, redrawn after each settle loop; the CLI can dump frames as PNG on given ticks.

Keep the `Device` interface and `DeviceConfig` general enough that these slot in without changes to the runner.

In v2.0 the editor shows each memory as a hex panel, editable while paused.

## File format

One JSON file per document, extension `.mip`, designed to diff well in git and to be readable by an AI agent. Pixels are stored as rows of `#` and `.` strings within each definition's rectangle; the root stores its rows relative to `origin`, which save sets to the top-left of its pixels' bounding box, with x rounded down to a multiple of 16 (so drawing a little left of the circuit does not rewrite every row in a diff; the loader accepts any origin). Root labels and instance positions are in world coordinates, not relative to `origin`. A definition may have a display `name`, which defaults to its ID. The file version is not part of the in-memory document. This is verbose but compresses well and makes small circuits legible in a pull request.

```json
{
  "format": "mipsim",
  "version": 1,
  "root": "top",
  "defs": {
    "top": {
      "origin": [-12, -8],
      "rows": ["...#.....", "..###....", "...#...."],
      "labels": [{"x": -9, "y": -8, "name": "clock"}],
      "instances": [{"id": "i1", "def": "nand2", "x": 20, "y": 4, "orient": "r90", "name": "g1"}]
    },
    "nand2": {
      "w": 11, "h": 9,
      "rows": ["..."],
      "labels": [{"x": 0, "y": 4, "name": "a"}],
      "instances": []
    }
  },
  "devices": [{"kind": "memory", "name": "data_ram", "addr": "port_addr", "...": "..."}]
}
```

Rules: unknown fields are rejected, and so are duplicate keys and anything after the document (a leading UTF-8 byte order mark is ignored); syntax errors are reported with a line and column; `orient` is one of `r0`, `r90`, `r180`, `r270`, `f0`, `f90`, `f180`, `f270` (`f` = mirrored first, then rotated clockwise), and may be omitted for `r0`; trailing `.` characters on a row may be omitted; rows beyond the last on pixel may be omitted; the loader validates every invariant from Components and instances and rejects the file with a located error rather than repairing it.

Limits, so that a few kilobytes cannot describe an unbounded circuit (checked by `Validate`, so they hold for edits too): coordinates of pixels, labels and instances within ±2^24; definition sizes up to 65536; the flattened circuit up to 2^20 instances and 2^28 on pixels.

Devices: each entry is a JSON object with a `kind` and a `name`; names follow the label rules (letters, digits, underscores) and are unique in the document. The rest of the object belongs to the device kind and is checked by the runner (the document keeps it verbatim, compacted). A `version` bump comes with a migration function and a test. Memory init files are raw binary, referenced by path relative to the `.mip` file (on web, uploaded alongside). The same row format is used for test fixtures and for the clipboard, so a user can paste ASCII art directly.

