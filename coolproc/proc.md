# My cool proc

A 16 bit processor to implement with mipsim. Very minimalist by design to simplify the processor. Many operations are assumed to be done via software by combining these elementary instructions.

Specs:

- Memory: 16 bit address space, each addresses a 16 bit word, ie. 128 kB of memory, for data (convention up to developer)
- Program: 16 bit program counter, each instruction is always 3 bytes, even if argument is unused
- Registers: A, B, X, Y. All 16 bits. A and B are ALU inputs, X and Y ALU outputs. Y is also the address register for memory access.
- PC is a separate register, not addressable (only jump instruction)
- Instructions: fixed length, 3 bytes
  - Op is one byte: 4 bits to select op, and two modifiers u and v each two bits. Often source/destination registers (0: A, 1: B, 2: X, 3: Y). src is always u and dest always v to be able to reuse the decoder.
  - Argument is two bytes, not always used
- No stack pointer, this is handled in software (assembler/compiler can use own convention since this runs single programs)

## Instructions

Memory:

| Op | u | v | Meaning |
| --- | --- | --- |---|
| load | - | dest | retrieve word at address Y and store in register dest |
| store | src | - | store word in register src to address Y |

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

Carry: a one bit cell, written by add and shift only, left unchanged by every other instruction. add stores its carry out. shift stores the last bit shifted out of A (0 if B is 0).

Assembler can provide aliases for some modes, like sub, not, nand, nor, nxor, shl, shr, etc.

Control:

| Op | u | v | Meaning |
| --- | --- | --- |---|
| jump | src | mode | set PC to 16 bit argument. mode: unconditional, if src is 0, if src is non 0, if src is negative (bit 15 set). src: any register A, B, X, Y |
| jumpr | src | - | set PC to address in register src (argument unused) |
| jumpc | - | mode | set PC to 16 bit argument according to the carry bit out of the ALU. mode: unconditional, if zero, if non zero |
| halt | - | - | stop execution |

Assembler can alias jumpz jumpnz jumpn jumpcz jumpcnz
