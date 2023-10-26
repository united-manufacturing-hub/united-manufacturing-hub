package base64

import (
	"math"
	"unsafe"
)

type sliceHeader struct {
	data uintptr
	len  int
	cap  int
}

type stringHeader struct {
	data uintptr
	len  int
}

func b2s(value []byte) string {
	return *(*string)(unsafe.Pointer(&value))
}

func s2b(value string) (b []byte) {
	bh := (*sliceHeader)(unsafe.Pointer(&b))
	sh := (*stringHeader)(unsafe.Pointer(&value))
	bh.data = sh.data
	bh.len = sh.len
	bh.cap = sh.len
	return b
}

func makeLuts(lutSe [64]byte) ([4096]uint32, [256]uint32, [256]uint32, [256]uint32, [256]uint32) {
	lutXe := [4096]uint32{}
	lutXd0 := [256]uint32{}
	lutXd1 := [256]uint32{}
	lutXd2 := [256]uint32{}
	lutXd3 := [256]uint32{}
	for i := 0; i < 256; i++ {
		lutXd0[i] = math.MaxUint32
		lutXd1[i] = math.MaxUint32
		lutXd2[i] = math.MaxUint32
		lutXd3[i] = math.MaxUint32
	}
	for i, ichar := range lutSe {
		for j, jchar := range lutSe {
			lutXe[j+i*64] = uint32(ichar) | uint32(jchar)<<8
		}
		lutXd0[ichar] = uint32(i * 4)
		d1 := uint32(i * 16)
		lutXd1[ichar] = (d1<<8)&0x0000FF00 | (d1>>8)&0x00000000FF
		d2 := uint32(i * 64)
		lutXd2[ichar] = (d2<<16)&0x00FF0000 | d2&0x0000FF00
		lutXd3[ichar] = uint32(i) << 16
	}
	return lutXe, lutXd0, lutXd1, lutXd2, lutXd3
}

var (
	stdLutSe = [64]byte{
		'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O', 'P',
		'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z', 'a', 'b', 'c', 'd', 'e', 'f',
		'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v',
		'w', 'x', 'y', 'z', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '+', '/',
	}
	urlLutSe = [64]byte{
		'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O', 'P',
		'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z', 'a', 'b', 'c', 'd', 'e', 'f',
		'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v',
		'w', 'x', 'y', 'z', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '-', '_',
	}
)
