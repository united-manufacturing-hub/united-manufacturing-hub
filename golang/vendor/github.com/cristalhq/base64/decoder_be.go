//go:build armbe || arm64be || mips || mips64 || mips64p32 || ppc || ppc64 || sparc || sparc64 || s390 || s390x
// +build armbe arm64be mips mips64 mips64p32 ppc ppc64 sparc sparc64 s390 s390x

package base64

import "unsafe"

func putTail(ptr uintptr, tail *[4]byte, n int) {
	switch n {
	case 3:
		*(*byte)(unsafe.Pointer(ptr)) = tail[3]
		*(*byte)(unsafe.Pointer(ptr + 1)) = tail[2]
		*(*byte)(unsafe.Pointer(ptr + 2)) = tail[1]
	case 2:
		*(*byte)(unsafe.Pointer(ptr)) = tail[3]
		*(*byte)(unsafe.Pointer(ptr + 1)) = tail[2]
	case 1:
		*(*byte)(unsafe.Pointer(ptr)) = tail[3]
	}
}
