//go:build 386 || amd64 || amd64p32 || arm || arm64 || mipsle || mips64le || mips64p32le || ppc64le || riscv || riscv64 || wasm
// +build 386 amd64 amd64p32 arm arm64 mipsle mips64le mips64p32le ppc64le riscv riscv64 wasm

package base64

import "unsafe"

func putTail(ptr uintptr, tail *[4]byte, n int) {
	switch n {
	case 3:
		*(*byte)(unsafe.Pointer(ptr)) = tail[0]
		*(*byte)(unsafe.Pointer(ptr + 1)) = tail[1]
		*(*byte)(unsafe.Pointer(ptr + 2)) = tail[2]
	case 2:
		*(*byte)(unsafe.Pointer(ptr)) = tail[0]
		*(*byte)(unsafe.Pointer(ptr + 1)) = tail[1]
	case 1:
		*(*byte)(unsafe.Pointer(ptr)) = tail[0]
	}
}
