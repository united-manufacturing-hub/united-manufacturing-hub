//go:build armbe || arm64be || mips || mips64 || mips64p32 || ppc || ppc64 || sparc || sparc64 || s390 || s390x
// +build armbe arm64be mips mips64 mips64p32 ppc ppc64 sparc sparc64 s390 s390x

package base64

import (
	"math/bits"
	"unsafe"
)

//go:nosplit
func bswap32(ptr uintptr) uint32 {
	return *(*uint32)(unsafe.Pointer(ptr))
}

//go:nosplit
func stou32(cp uintptr, x uint32) {
	*(*uint32)(unsafe.Pointer(cp)) = bits.ReverseBytes32(x)
}

//go:nosplit
func ctou32(cp uintptr) uint32 {
	return bits.ReverseBytes32(*(*uint32)(unsafe.Pointer(cp)))
}
