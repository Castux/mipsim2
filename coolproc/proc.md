# My cool proc

A 16 bit processor to implement with mipsim. Very minimalist by design to simplify the processor. Many operations are assumed to be done via software by combining these elementary instructions.

Specs:

- Memory: 16 bit address space, 64 kB of memory, used for both program and data (convention up to developer)
- Registers: A, B, X, Y. All 16 bits. A and B are ALU inputs, X and Y ALU outputs. Y is also the address register for memory access.
- PC is a separate register, not addressable (only jump instruction)
- Instructions: variable length, 1 to 3 bytes
  - Op is one byte: 4 bits to select op, and two modifiers u and v each two bits. Often source/destination registers (0: A, 1: B, 2: X, 3: Y). src is always u and dest always v to be able to reuse the decoder.
  - Argument is one or two bytes
- No stack pointer, this is handled in software (assembler/compiler can use own convention since this runs single programs)

## Instructions

Memory:

| Op | u | v | Meaning |
| --- | --- | --- |---|
| load | - | dest | retrieve word at address Y and store in register dest |
| store | src | - | store word in register src to address Y |

Possible modifier: store/load high byte or low byte instead of 16 bits word

Registers:

| Op | u | v | Meaning |
| --- | --- | --- |---|
| set | - | dest | store 16 bits argument into register dest |
| move | src | dest | copy word from register src to dest |

ALU:

| Op | u | v | Meaning |
| --- | --- | --- |---|
| add | mode | dest | add A and B. u0: carry in 0/1, u1: B/not B, dest: X or Y. Gives A+B, A+B+1, A-B-1, A-B |
| shift | mode | dest | shift A by B position. mode: left/right. dest: X or Y |
| and | mode | dest | A and B. u0: A/not A, u1: B/not B. dest: X or Y |
| or | mode | dest | A or B. u0: A/not A, u1: B/not B. dest: X or Y |
| xor | mode | dest | A xor B. u0: A/not A, u1: B/not B. dest: X or Y |

For all ALU ops, v0 selects dest (X or Y) and v1 selects the second operand: 0 = register B (1 byte instruction), 1 = 16 bits argument in place of B (3 bytes instruction). Modes apply to the argument the same way, so A - k is `add` with carry in 1 and not B.

Assembler can provide aliases for some modes, like sub, not, nand, nor, nxor, shl, shr, etc.

Control:

| Op | u | v | Meaning |
| --- | --- | --- |---|
| jump | src | mode | set PC to 16 bit argument. mode: unconditional, if src is 0, if src is non 0, if src is negative (bit 15 set). src: any register A, B, X, Y |
| jumpr | src | - | set PC to address in register src (no argument, 1 byte) |
| halt | - | - | stop execution |

Assembler can alias jumpz jumpnz jumpn
