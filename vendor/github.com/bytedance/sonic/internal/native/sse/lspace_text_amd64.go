// +build amd64
// Code generated by asm2asm, DO NOT EDIT.

package sse

var _text_lspace = []byte{
	// .p2align 4, 0x90
	// _lspace
	0x55, // pushq        %rbp
	0x48, 0x89, 0xe5, //0x00000001 movq         %rsp, %rbp
	0x48, 0x89, 0xd0, //0x00000004 movq         %rdx, %rax
	0x48, 0x39, 0xd6, //0x00000007 cmpq         %rdx, %rsi
	0x0f, 0x84, 0x39, 0x00, 0x00, 0x00, //0x0000000a je           LBB0_1
	0x4c, 0x8d, 0x04, 0x37, //0x00000010 leaq         (%rdi,%rsi), %r8
	0x48, 0xba, 0x00, 0x26, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, //0x00000014 movabsq      $4294977024, %rdx
	0x90, 0x90, //0x0000001e .p2align 4, 0x90
	//0x00000020 LBB0_3
	0x0f, 0xbe, 0x0c, 0x07, //0x00000020 movsbl       (%rdi,%rax), %ecx
	0x83, 0xf9, 0x20, //0x00000024 cmpl         $32, %ecx
	0x0f, 0x87, 0x28, 0x00, 0x00, 0x00, //0x00000027 ja           LBB0_7
	0x48, 0x0f, 0xa3, 0xca, //0x0000002d btq          %rcx, %rdx
	0x0f, 0x83, 0x1e, 0x00, 0x00, 0x00, //0x00000031 jae          LBB0_7
	0x48, 0x83, 0xc0, 0x01, //0x00000037 addq         $1, %rax
	0x48, 0x39, 0xc6, //0x0000003b cmpq         %rax, %rsi
	0x0f, 0x85, 0xdc, 0xff, 0xff, 0xff, //0x0000003e jne          LBB0_3
	0xe9, 0x06, 0x00, 0x00, 0x00, //0x00000044 jmp          LBB0_6
	//0x00000049 LBB0_1
	0x48, 0x01, 0xf8, //0x00000049 addq         %rdi, %rax
	0x49, 0x89, 0xc0, //0x0000004c movq         %rax, %r8
	//0x0000004f LBB0_6
	0x49, 0x29, 0xf8, //0x0000004f subq         %rdi, %r8
	0x4c, 0x89, 0xc0, //0x00000052 movq         %r8, %rax
	//0x00000055 LBB0_7
	0x5d, //0x00000055 popq         %rbp
	0xc3, //0x00000056 retq         
}
 
