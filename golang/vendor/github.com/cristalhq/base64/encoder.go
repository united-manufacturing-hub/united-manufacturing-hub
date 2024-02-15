package base64

import "unsafe"

//go:nosplit
func (e *Encoding) encode(dst []byte, src []byte, outlen uintptr) {
	inlen := len(src)
	ip := (*sliceHeader)(unsafe.Pointer(&src)).data
	ipstart := ip
	op := (*sliceHeader)(unsafe.Pointer(&dst)).data
	opstart := op
	if outlen >= 8+12 {
		u0x := bswap32(ip)
		u1x := bswap32(ip + 3)
		for op <= (opstart+outlen)-(128+12) {
			{
				_u0, _u1 := u0x, u1x
				u0x = bswap32(ip + 6 + 0*6)
				u1x = bswap32(ip + 6 + 0*6 + 3)
				_u0 = e.lutXe[(_u0>>8)&0xfff]<<16 | e.lutXe[_u0>>20]
				stou32(op+0*8, _u0)
				_u1 = e.lutXe[(_u1>>8)&0xfff]<<16 | e.lutXe[_u1>>20]
				stou32(op+0*8+4, _u1)
			}
			{
				_u0, _u1 := u0x, u1x
				u0x = bswap32(ip + 6 + 1*6)
				u1x = bswap32(ip + 6 + 1*6 + 3)
				_u0 = e.lutXe[(_u0>>8)&0xfff]<<16 | e.lutXe[_u0>>20]
				stou32(op+1*8, _u0)
				_u1 = e.lutXe[(_u1>>8)&0xfff]<<16 | e.lutXe[_u1>>20]
				stou32(op+1*8+4, _u1)
			}
			{
				_u0, _u1 := u0x, u1x
				u0x = bswap32(ip + 6 + 2*6)
				u1x = bswap32(ip + 6 + 2*6 + 3)
				_u0 = e.lutXe[(_u0>>8)&0xfff]<<16 | e.lutXe[_u0>>20]
				stou32(op+2*8, _u0)
				_u1 = e.lutXe[(_u1>>8)&0xfff]<<16 | e.lutXe[_u1>>20]
				stou32(op+2*8+4, _u1)
			}
			{
				_u0, _u1 := u0x, u1x
				u0x = bswap32(ip + 6 + 3*6)
				u1x = bswap32(ip + 6 + 3*6 + 3)
				_u0 = e.lutXe[(_u0>>8)&0xfff]<<16 | e.lutXe[_u0>>20]
				stou32(op+3*8, _u0)
				_u1 = e.lutXe[(_u1>>8)&0xfff]<<16 | e.lutXe[_u1>>20]
				stou32(op+3*8+4, _u1)
			}
			{
				_u0, _u1 := u0x, u1x
				u0x = bswap32(ip + 6 + 4*6)
				u1x = bswap32(ip + 6 + 4*6 + 3)
				_u0 = e.lutXe[(_u0>>8)&0xfff]<<16 | e.lutXe[_u0>>20]
				stou32(op+4*8, _u0)
				_u1 = e.lutXe[(_u1>>8)&0xfff]<<16 | e.lutXe[_u1>>20]
				stou32(op+4*8+4, _u1)
			}
			{
				_u0, _u1 := u0x, u1x
				u0x = bswap32(ip + 6 + 5*6)
				u1x = bswap32(ip + 6 + 5*6 + 3)
				_u0 = e.lutXe[(_u0>>8)&0xfff]<<16 | e.lutXe[_u0>>20]
				stou32(op+5*8, _u0)
				_u1 = e.lutXe[(_u1>>8)&0xfff]<<16 | e.lutXe[_u1>>20]
				stou32(op+5*8+4, _u1)
			}
			{
				_u0, _u1 := u0x, u1x
				u0x = bswap32(ip + 6 + 6*6)
				u1x = bswap32(ip + 6 + 6*6 + 3)
				_u0 = e.lutXe[(_u0>>8)&0xfff]<<16 | e.lutXe[_u0>>20]
				stou32(op+6*8, _u0)
				_u1 = e.lutXe[(_u1>>8)&0xfff]<<16 | e.lutXe[_u1>>20]
				stou32(op+6*8+4, _u1)
			}
			{
				_u0, _u1 := u0x, u1x
				u0x = bswap32(ip + 6 + 7*6)
				u1x = bswap32(ip + 6 + 7*6 + 3)
				_u0 = e.lutXe[(_u0>>8)&0xfff]<<16 | e.lutXe[_u0>>20]
				stou32(op+7*8, _u0)
				_u1 = e.lutXe[(_u1>>8)&0xfff]<<16 | e.lutXe[_u1>>20]
				stou32(op+7*8+4, _u1)
			}
			{
				_u0, _u1 := u0x, u1x
				u0x = bswap32(ip + 6 + 8*6)
				u1x = bswap32(ip + 6 + 8*6 + 3)
				_u0 = e.lutXe[(_u0>>8)&0xfff]<<16 | e.lutXe[_u0>>20]
				stou32(op+8*8, _u0)
				_u1 = e.lutXe[(_u1>>8)&0xfff]<<16 | e.lutXe[_u1>>20]
				stou32(op+8*8+4, _u1)
			}
			{
				_u0, _u1 := u0x, u1x
				u0x = bswap32(ip + 6 + 9*6)
				u1x = bswap32(ip + 6 + 9*6 + 3)
				_u0 = e.lutXe[(_u0>>8)&0xfff]<<16 | e.lutXe[_u0>>20]
				stou32(op+9*8, _u0)
				_u1 = e.lutXe[(_u1>>8)&0xfff]<<16 | e.lutXe[_u1>>20]
				stou32(op+9*8+4, _u1)
			}
			{
				_u0, _u1 := u0x, u1x
				u0x = bswap32(ip + 6 + 10*6)
				u1x = bswap32(ip + 6 + 10*6 + 3)
				_u0 = e.lutXe[(_u0>>8)&0xfff]<<16 | e.lutXe[_u0>>20]
				stou32(op+10*8, _u0)
				_u1 = e.lutXe[(_u1>>8)&0xfff]<<16 | e.lutXe[_u1>>20]
				stou32(op+10*8+4, _u1)
			}
			{
				_u0, _u1 := u0x, u1x
				u0x = bswap32(ip + 6 + 11*6)
				u1x = bswap32(ip + 6 + 11*6 + 3)
				_u0 = e.lutXe[(_u0>>8)&0xfff]<<16 | e.lutXe[_u0>>20]
				stou32(op+11*8, _u0)
				_u1 = e.lutXe[(_u1>>8)&0xfff]<<16 | e.lutXe[_u1>>20]
				stou32(op+11*8+4, _u1)
			}
			{
				_u0, _u1 := u0x, u1x
				u0x = bswap32(ip + 6 + 12*6)
				u1x = bswap32(ip + 6 + 12*6 + 3)
				_u0 = e.lutXe[(_u0>>8)&0xfff]<<16 | e.lutXe[_u0>>20]
				stou32(op+12*8, _u0)
				_u1 = e.lutXe[(_u1>>8)&0xfff]<<16 | e.lutXe[_u1>>20]
				stou32(op+12*8+4, _u1)
			}
			{
				_u0, _u1 := u0x, u1x
				u0x = bswap32(ip + 6 + 13*6)
				u1x = bswap32(ip + 6 + 13*6 + 3)
				_u0 = e.lutXe[(_u0>>8)&0xfff]<<16 | e.lutXe[_u0>>20]
				stou32(op+13*8, _u0)
				_u1 = e.lutXe[(_u1>>8)&0xfff]<<16 | e.lutXe[_u1>>20]
				stou32(op+13*8+4, _u1)
			}
			{
				_u0, _u1 := u0x, u1x
				u0x = bswap32(ip + 6 + 14*6)
				u1x = bswap32(ip + 6 + 14*6 + 3)
				_u0 = e.lutXe[(_u0>>8)&0xfff]<<16 | e.lutXe[_u0>>20]
				stou32(op+14*8, _u0)
				_u1 = e.lutXe[(_u1>>8)&0xfff]<<16 | e.lutXe[_u1>>20]
				stou32(op+14*8+4, _u1)
			}
			{
				_u0, _u1 := u0x, u1x
				u0x = bswap32(ip + 6 + 15*6)
				u1x = bswap32(ip + 6 + 15*6 + 3)
				_u0 = e.lutXe[(_u0>>8)&0xfff]<<16 | e.lutXe[_u0>>20]
				stou32(op+15*8, _u0)
				_u1 = e.lutXe[(_u1>>8)&0xfff]<<16 | e.lutXe[_u1>>20]
				stou32(op+15*8+4, _u1)
			}
			op += 128
			ip += (128 / 4) * 3
		}
		for op <= (opstart+outlen)-(16+12) {
			{
				_u0, _u1 := u0x, u1x
				u0x = bswap32(ip + 6 + 0*6)
				u1x = bswap32(ip + 6 + 0*6 + 3)
				_u0 = e.lutXe[(_u0>>8)&0xfff]<<16 | e.lutXe[_u0>>20]
				stou32(op+0*8, _u0)
				_u1 = e.lutXe[(_u1>>8)&0xfff]<<16 | e.lutXe[_u1>>20]
				stou32(op+0*8+4, _u1)
			}
			{
				_u0, _u1 := u0x, u1x
				u0x = bswap32(ip + 6 + 1*6)
				u1x = bswap32(ip + 6 + 1*6 + 3)
				_u0 = e.lutXe[(_u0>>8)&0xfff]<<16 | e.lutXe[_u0>>20]
				stou32(op+1*8, _u0)
				_u1 = e.lutXe[(_u1>>8)&0xfff]<<16 | e.lutXe[_u1>>20]
				stou32(op+1*8+4, _u1)
			}
			op += 16
			ip += (16 / 4) * 3
		}
		if op <= (opstart+outlen)-(8+12) {
			_u0 := e.lutXe[(u0x>>8)&0xfff]<<16 | e.lutXe[u0x>>20]
			stou32(op+0*8, _u0)
			_u1 := e.lutXe[(u1x>>8)&0xfff]<<16 | e.lutXe[u1x>>20]
			stou32(op+0*8+4, _u1)
			op += 8
			ip += (8 / 4) * 3
		}
	}
	for op < (opstart+outlen)-4 {
		_u := bswap32(ip)
		stou32(op, e.lutXe[(_u>>8)&0xfff]<<16|e.lutXe[_u>>20])
		op += 4
		ip += 3
	}
	_l := uint32((ipstart + uintptr(inlen)) - ip)
	if _l == 3 {
		_u := uint32(*(*byte)(unsafe.Pointer(ip + 0)))<<24 | uint32(*(*byte)(unsafe.Pointer(ip + 1)))<<16 | uint32(*(*byte)(unsafe.Pointer(ip + 2)))<<8
		stou32(op, uint32(e.lutSe[(_u>>8)&0x3f])<<24|uint32(e.lutSe[(_u>>14)&0x3f])<<16|uint32(e.lutSe[(_u>>20)&0x3f])<<8|uint32(e.lutSe[(_u>>26)&0x3f]))
	} else if _l != 0 {
		*(*byte)(unsafe.Pointer(op)) = e.lutSe[(*(*byte)(unsafe.Pointer(ip + 0))>>2)&0x3f]
		op++
		if _l == 2 {
			*(*byte)(unsafe.Pointer(op)) = e.lutSe[(*(*byte)(unsafe.Pointer(ip + 0))&0x3)<<4|(*(*byte)(unsafe.Pointer(ip + 1))&0xf0)>>4]
			op++
			*(*byte)(unsafe.Pointer(op)) = e.lutSe[(*(*byte)(unsafe.Pointer(ip + 1))&0xf)<<2]
			op++
		} else {
			*(*byte)(unsafe.Pointer(op)) = e.lutSe[(*(*byte)(unsafe.Pointer(ip + 0))&0x3)<<4]
			op++
			if e.pad {
				*(*byte)(unsafe.Pointer(op)) = '='
				op++
			}
		}
		if e.pad {
			*(*byte)(unsafe.Pointer(op)) = '='
		}
	}
}
