//go:build 386 || amd64 || amd64p32 || arm || arm64 || mipsle || mips64le || mips64p32le || ppc64le || riscv || riscv64 || wasm
// +build 386 amd64 amd64p32 arm arm64 mipsle mips64le mips64p32le ppc64le riscv riscv64 wasm

package base64

import (
	"math/bits"
	"unsafe"
)

//go:nosplit
func bswap32(ptr uintptr) uint32 {
	return bits.ReverseBytes32(*(*uint32)(unsafe.Pointer(ptr)))
}

//go:nosplit
func stou32(cp uintptr, x uint32) {
	*(*uint32)(unsafe.Pointer(cp)) = x
}

//go:nosplit
func ctou32(cp uintptr) uint32 {
	return *(*uint32)(unsafe.Pointer(cp))
}
