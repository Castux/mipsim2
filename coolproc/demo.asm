; Demo for coolproc: the first 20 Fibonacci numbers, stored in RAM from
; address 0. Y is both the RAM address and the jump target, so the next
; address is kept in RAM at 0xffff. Watch the RAM tab fill up.
	setl c, 0        ; f(n-1)
	setl d, 1        ; f(n)
loop:
	set y, 0xffff    ; y = ram[0xffff], the next address
	load a
	move a, y
	store d          ; ram[address] = f(n)
	setl b, 1        ; ram[0xffff] = address + 1
	add x
	set y, 0xffff
	store x
	move c, a        ; c, d = d, c + d
	move d, b
	add x
	move d, c
	move x, d
	load a           ; loop while the address is not 20
	setl b, 20
	sub x
	set y, loop
	jumpnz x
	halt
