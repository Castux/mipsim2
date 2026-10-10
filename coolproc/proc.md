# My cool proc

A 16 bit processor to implement with mipsim. Very minimalist by design to simplify the processor. Many operations are assumed to be done via software by combining these elementary instructions.

Specs:

- Memory: 16 bit address space, each addresses a 16 bit word, ie. 128 kB of memory, for data (convention up to developer)
- Program: separate memory, 16 bit program counter, each address holds one 2 byte instruction
- Registers: A, B, X, Y, C, D. All 16 bits.
  - A and B are ALU inputs, X and Y ALU outputs, with their own wiring to the ALU.
  - Y is the address register: it is wired directly to the memory address and is the target of jumps.
  - C and D are general purpose.
  - All registers are connected to a common data bus, also used by memory data and the instruction argument.
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
| `0001 0 mm` | jump, mm: condition |
| `0001 1 mm` | jumpc, mm: condition |
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
| 001 | shift | shift A by B positions. m0: left/right, m1 reserved |
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
