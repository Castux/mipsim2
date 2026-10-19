# My cool proc

A 16 bit processor to implement with mipsim. Very minimalist by design to simplify the processor. Many operations are assumed to be done via software by combining these elementary instructions.

Specs:

- Memory: 16 bit address space, each addresses a 16 bit word, ie. 128 kB of memory, for data (convention up to developer)
- Program: separate memory, 16 bit program counter, each address holds one 2 byte instruction
- Registers: A, B, X, Y, C, D. All 16 bits.
  - A and B are ALU inputs, X and Y ALU outputs, with their own wiring to the ALU.
  - Y is the address register: it is wired directly to the memory address and is the target of jumps.
  - C and D are general purpose.
  - All registers are connected to a common data bus, also accessible by memory data and the instruction argument.
- PC is a separate register, not addressable (only jump instructions)
- No stack pointer, this is handled in software (assembler/compiler can use own convention since this runs single programs)

## Instruction format

Instructions are 2 bytes:

- Byte 0: opcode. Its bits are organized as fields so the decoder only recognizes a few groups and wires the rest to control lines.
- Byte 1: two nibbles, src (high nibble) and dest (low nibble), or an 8 bit immediate for set.

Register numbers: 0: A, 1: B, 2: X, 3: Y, 4: C, 5: D. 6, 7 and the top bit of each nibble are reserved.

src is always the high nibble and dest always the low nibble, to be able to reuse the decoder.

## Opcodes

| Opcode bits | Instruction |
| --- | --- |
| `0000 0000` | halt |
| `0000 0001` | move |
| `0000 0010` | load |
| `0000 0011` | store |
| `0001 0 x mm` | jump, mm: condition |
| `0001 1 x mm` | jumpc, mm: condition |
| `01 ooo mm d` | ALU, ooo: operation, mm: mode, d: dest (0: X, 1: Y) |
| `1 h 000 ddd` | set, h: low/high byte, ddd: dest register |

All other opcodes are reserved. An all zero instruction is halt, so running off the end of the program stops.

## Instructions

Memory:

| Instruction | src | dest | Meaning |
| --- | --- | --- | --- |
| load | - | dest | retrieve word at address Y and store in register dest |
| store | src | - | store word in register src to address Y |

Registers:

| Instruction | src | dest | Meaning |
| --- | --- | --- | --- |
| move | src | dest | copy word from register src to dest |
| set | (immediate) | | h = 0 (setl): store 8 bits immediate, zero extended, into register ddd. h = 1 (seth): store 8 bits immediate into the high byte of ddd, low byte unchanged |

A 16 bit constant is setl then seth. Constants 0..255 are a single setl.

ALU (src and dest nibbles unused):

| ooo | Operation | Meaning |
| --- | --- | --- |
| 000 | add | add A and B. m0: carry in 0/1, m1: B/not B. Gives A+B, A+B+1, A-B-1, A-B |
| 001 | shift | shift A by B[3:0] positions (B mod 16). m0: left/right, m1 reserved |
| 010 | and | A and B. m0: A/not A, m1: B/not B |
| 011 | or | A or B. m0: A/not A, m1: B/not B |
| 100 | xor | A xor B. m0: A/not A, m1: B/not B |

Carry: a one bit cell, written by add and shift only, left unchanged by every other instruction. add stores its carry out. shift stores the last bit shifted out of A (0 if B is 0).

Assembler can provide aliases for some modes, like sub, not, nand, nor, nxor, shl, shr, etc.

Control (dest nibble unused):

| Instruction | src | Meaning |
| --- | --- | --- |
| jump | src | set PC to Y. mm: unconditional, if src is 0, if src is non 0, if src is negative (bit 15 set) |
| jumpc | - | set PC to Y according to the carry bit. mm: unconditional, if carry is 0, if carry is 1 |

Assembler can alias jumpz jumpnz jumpn jumpcz jumpcnz, and provide `jump label` as a pseudo instruction expanding to setl Y, seth Y and jump.

## Forbidden combinations

Registers are gated D latches, so an instruction must not write a register its own result depends on while the write is enabled:

- `load Y`: the loaded value would change the address it is loaded from. Use `load` into another register and `move` it to Y.

## Implementation

`coolproc/gen` generates the processor as `coolproc/cpu.mip` (`go run ./coolproc/gen`), with the program `coolproc/demo.asm` assembled into `coolproc/prog.bin` for the ROM. Its tests run programs on the generated circuit in the headless simulator (`go test ./coolproc/gen`).

- **Gates:** everything is NOR gates (a pull-up power cell and one pull-down transistor per input), laid out as a gate matrix: signals are vertical lines, gates are horizontal rows, crossings are bridges. Data only drives transistor gates, never channels, so "low wins" never corrupts a value.
- **Layout:** the control block (instruction register, sequencer, decoder) on top, then 16 bit slices, bit 15 first. Each register bit is a gated D latch of four NOR gates.
- **Bus:** one NOR gate per bit, pulled up, with a pull-down per driver, so it reads all ones when nothing drives it. The ALU result goes to X or Y over the bus (the bus is free during ALU instructions), not over separate wiring.
- **ALU:** A and B each go through an inverter and mux (the not A and not B modes), then nand, nor and xor gates and a ripple-carry adder. `and` is the nor of the inverted operands, `or` the nand, so the instruction set is unchanged. The shifter is a barrel shifter over positions -1 to 16, so the last bit shifted out lands in position 16 (left) or -1 (right) for the carry.
- **Instruction word:** the opcode is the high byte. The ROM init file holds little-endian 16-bit words.
- **Timing:** a Johnson counter of 6 master-slave flip-flops gives 12 states, so an instruction takes 12 clock ticks, in 4 steps of 3 states (route, route and write, route):
  - fetch: the ROM word into IR
  - PC + 1 into TMP
  - execute
  - TMP into PC, or Y for a taken jump (the jump condition is latched during execute)
- **Reset:** pin `reset` high for a tick, then release it. It clears every register, PC and the sequencer.
- **Halt:** stops the sequencer (the clock keeps running) and raises `halted`.
- **Memories:** `rom` (64K words of 16 bits, read only) on `pc` and `rom`; `ram` (64K words) on `y` and `bus`, with `mem_read` and `mem_write`.

The assembler (`coolproc/gen/asm.go`) takes `move src, dest`, `load dest`, `store src`, `setl r, n`, `seth r, n`, `set r, n` (two instructions), ALU instructions with their destination and modes (`add x`, `sub y`, `and x na nb`, `shl x`...), the jumps, `goto label` and labels.

## Tests

`coolproc/isatest` checks a processor against a reference emulator of this instruction set (written from this document), comparing registers, program counter, carry, data memory and the number of instructions run. Both generated processors run it (`go test ./coolproc/gen ./coolproc/readable`):

- every set, move, load and store, between every pair of registers;
- every ALU operation in every mode and to both destinations, on pairs of interesting operands, keeping the result and the carry after each (the carry is set before each one, so an operation that must leave it alone is caught);
- every shift amount (and amounts above 15) in both directions;
- every jump condition on every register, true and false, and the jumps on carry;
- random programs of 300 instructions with forward jumps;
- real programs: multiplication, division (with remainder), bubble sort, population count, copying memory, subroutine calls. Each also checks its own result.
