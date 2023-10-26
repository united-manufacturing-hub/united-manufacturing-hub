package base64

import "unsafe"

//go:nosplit
func (e *Encoding) decode(dst []byte, src []byte) int {
	dstlen := uintptr(len(dst))
	srclen := uintptr(len(src))
	if srclen == 0 || (e.pad && (srclen&3) != 0) {
		return 0
	}
	ip := (*sliceHeader)(unsafe.Pointer(&src)).data
	ipstart := ip
	op := (*sliceHeader)(unsafe.Pointer(&dst)).data
	opstart := op
	var cu uint32
	if srclen >= 8+4 {
		ux := ctou32(ip)
		vx := ctou32(ip + 4)
		for ip < (ipstart+srclen)-(128+4) {
			{
				_u := ux
				ux = ctou32(ip + 8 + 0*8)
				_u = (e.lutXd0[byte(_u)] | e.lutXd1[byte(_u>>8)] | e.lutXd2[byte(_u>>16)] | e.lutXd3[_u>>24])
				cu |= _u
				stou32(op+0*6, _u)
				_v := vx
				vx = ctou32(ip + 8 + 0*8 + 4)
				_v = (e.lutXd0[byte(_v)] | e.lutXd1[byte(_v>>8)] | e.lutXd2[byte(_v>>16)] | e.lutXd3[_v>>24])
				cu |= _v
				stou32(op+0*6+3, _v)
			}
			{
				_u := ux
				ux = ctou32(ip + 8 + 1*8)
				_u = (e.lutXd0[byte(_u)] | e.lutXd1[byte(_u>>8)] | e.lutXd2[byte(_u>>16)] | e.lutXd3[_u>>24])
				cu |= _u
				stou32(op+1*6, _u)
				_v := vx
				vx = ctou32(ip + 8 + 1*8 + 4)
				_v = (e.lutXd0[byte(_v)] | e.lutXd1[byte(_v>>8)] | e.lutXd2[byte(_v>>16)] | e.lutXd3[_v>>24])
				cu |= _v
				stou32(op+1*6+3, _v)
			}
			{
				_u := ux
				ux = ctou32(ip + 8 + 2*8)
				_u = (e.lutXd0[byte(_u)] | e.lutXd1[byte(_u>>8)] | e.lutXd2[byte(_u>>16)] | e.lutXd3[_u>>24])
				cu |= _u
				stou32(op+2*6, _u)
				_v := vx
				vx = ctou32(ip + 8 + 2*8 + 4)
				_v = (e.lutXd0[byte(_v)] | e.lutXd1[byte(_v>>8)] | e.lutXd2[byte(_v>>16)] | e.lutXd3[_v>>24])
				cu |= _v
				stou32(op+2*6+3, _v)
			}
			{
				_u := ux
				ux = ctou32(ip + 8 + 3*8)
				_u = (e.lutXd0[byte(_u)] | e.lutXd1[byte(_u>>8)] | e.lutXd2[byte(_u>>16)] | e.lutXd3[_u>>24])
				cu |= _u
				stou32(op+3*6, _u)
				_v := vx
				vx = ctou32(ip + 8 + 3*8 + 4)
				_v = (e.lutXd0[byte(_v)] | e.lutXd1[byte(_v>>8)] | e.lutXd2[byte(_v>>16)] | e.lutXd3[_v>>24])
				cu |= _v
				stou32(op+3*6+3, _v)
			}
			{
				_u := ux
				ux = ctou32(ip + 8 + 4*8)
				_u = (e.lutXd0[byte(_u)] | e.lutXd1[byte(_u>>8)] | e.lutXd2[byte(_u>>16)] | e.lutXd3[_u>>24])
				cu |= _u
				stou32(op+4*6, _u)
				_v := vx
				vx = ctou32(ip + 8 + 4*8 + 4)
				_v = (e.lutXd0[byte(_v)] | e.lutXd1[byte(_v>>8)] | e.lutXd2[byte(_v>>16)] | e.lutXd3[_v>>24])
				cu |= _v
				stou32(op+4*6+3, _v)
			}
			{
				_u := ux
				ux = ctou32(ip + 8 + 5*8)
				_u = (e.lutXd0[byte(_u)] | e.lutXd1[byte(_u>>8)] | e.lutXd2[byte(_u>>16)] | e.lutXd3[_u>>24])
				cu |= _u
				stou32(op+5*6, _u)
				_v := vx
				vx = ctou32(ip + 8 + 5*8 + 4)
				_v = (e.lutXd0[byte(_v)] | e.lutXd1[byte(_v>>8)] | e.lutXd2[byte(_v>>16)] | e.lutXd3[_v>>24])
				cu |= _v
				stou32(op+5*6+3, _v)
			}
			{
				_u := ux
				ux = ctou32(ip + 8 + 6*8)
				_u = (e.lutXd0[byte(_u)] | e.lutXd1[byte(_u>>8)] | e.lutXd2[byte(_u>>16)] | e.lutXd3[_u>>24])
				cu |= _u
				stou32(op+6*6, _u)
				_v := vx
				vx = ctou32(ip + 8 + 6*8 + 4)
				_v = (e.lutXd0[byte(_v)] | e.lutXd1[byte(_v>>8)] | e.lutXd2[byte(_v>>16)] | e.lutXd3[_v>>24])
				cu |= _v
				stou32(op+6*6+3, _v)
			}
			{
				_u := ux
				ux = ctou32(ip + 8 + 7*8)
				_u = (e.lutXd0[byte(_u)] | e.lutXd1[byte(_u>>8)] | e.lutXd2[byte(_u>>16)] | e.lutXd3[_u>>24])
				cu |= _u
				stou32(op+7*6, _u)
				_v := vx
				vx = ctou32(ip + 8 + 7*8 + 4)
				_v = (e.lutXd0[byte(_v)] | e.lutXd1[byte(_v>>8)] | e.lutXd2[byte(_v>>16)] | e.lutXd3[_v>>24])
				cu |= _v
				stou32(op+7*6+3, _v)
			}

			{
				_u := ux
				ux = ctou32(ip + 8 + 8*8)
				_u = (e.lutXd0[byte(_u)] | e.lutXd1[byte(_u>>8)] | e.lutXd2[byte(_u>>16)] | e.lutXd3[_u>>24])
				cu |= _u
				stou32(op+8*6, _u)
				_v := vx
				vx = ctou32(ip + 8 + 8*8 + 4)
				_v = (e.lutXd0[byte(_v)] | e.lutXd1[byte(_v>>8)] | e.lutXd2[byte(_v>>16)] | e.lutXd3[_v>>24])
				cu |= _v
				stou32(op+8*6+3, _v)
			}
			{
				_u := ux
				ux = ctou32(ip + 8 + 9*8)
				_u = (e.lutXd0[byte(_u)] | e.lutXd1[byte(_u>>8)] | e.lutXd2[byte(_u>>16)] | e.lutXd3[_u>>24])
				cu |= _u
				stou32(op+9*6, _u)
				_v := vx
				vx = ctou32(ip + 8 + 9*8 + 4)
				_v = (e.lutXd0[byte(_v)] | e.lutXd1[byte(_v>>8)] | e.lutXd2[byte(_v>>16)] | e.lutXd3[_v>>24])
				cu |= _v
				stou32(op+9*6+3, _v)
			}
			{
				_u := ux
				ux = ctou32(ip + 8 + 10*8)
				_u = (e.lutXd0[byte(_u)] | e.lutXd1[byte(_u>>8)] | e.lutXd2[byte(_u>>16)] | e.lutXd3[_u>>24])
				cu |= _u
				stou32(op+10*6, _u)
				_v := vx
				vx = ctou32(ip + 8 + 10*8 + 4)
				_v = (e.lutXd0[byte(_v)] | e.lutXd1[byte(_v>>8)] | e.lutXd2[byte(_v>>16)] | e.lutXd3[_v>>24])
				cu |= _v
				stou32(op+10*6+3, _v)
			}
			{
				_u := ux
				ux = ctou32(ip + 8 + 11*8)
				_u = (e.lutXd0[byte(_u)] | e.lutXd1[byte(_u>>8)] | e.lutXd2[byte(_u>>16)] | e.lutXd3[_u>>24])
				cu |= _u
				stou32(op+11*6, _u)
				_v := vx
				vx = ctou32(ip + 8 + 11*8 + 4)
				_v = (e.lutXd0[byte(_v)] | e.lutXd1[byte(_v>>8)] | e.lutXd2[byte(_v>>16)] | e.lutXd3[_v>>24])
				cu |= _v
				stou32(op+11*6+3, _v)
			}
			{
				_u := ux
				ux = ctou32(ip + 8 + 12*8)
				_u = (e.lutXd0[byte(_u)] | e.lutXd1[byte(_u>>8)] | e.lutXd2[byte(_u>>16)] | e.lutXd3[_u>>24])
				cu |= _u
				stou32(op+12*6, _u)
				_v := vx
				vx = ctou32(ip + 8 + 12*8 + 4)
				_v = (e.lutXd0[byte(_v)] | e.lutXd1[byte(_v>>8)] | e.lutXd2[byte(_v>>16)] | e.lutXd3[_v>>24])
				cu |= _v
				stou32(op+12*6+3, _v)
			}
			{
				_u := ux
				ux = ctou32(ip + 8 + 13*8)
				_u = (e.lutXd0[byte(_u)] | e.lutXd1[byte(_u>>8)] | e.lutXd2[byte(_u>>16)] | e.lutXd3[_u>>24])
				cu |= _u
				stou32(op+13*6, _u)
				_v := vx
				vx = ctou32(ip + 8 + 13*8 + 4)
				_v = (e.lutXd0[byte(_v)] | e.lutXd1[byte(_v>>8)] | e.lutXd2[byte(_v>>16)] | e.lutXd3[_v>>24])
				cu |= _v
				stou32(op+13*6+3, _v)
			}
			{
				_u := ux
				ux = ctou32(ip + 8 + 14*8)
				_u = (e.lutXd0[byte(_u)] | e.lutXd1[byte(_u>>8)] | e.lutXd2[byte(_u>>16)] | e.lutXd3[_u>>24])
				cu |= _u
				stou32(op+14*6, _u)
				_v := vx
				vx = ctou32(ip + 8 + 14*8 + 4)
				_v = (e.lutXd0[byte(_v)] | e.lutXd1[byte(_v>>8)] | e.lutXd2[byte(_v>>16)] | e.lutXd3[_v>>24])
				cu |= _v
				stou32(op+14*6+3, _v)
			}
			{
				_u := ux
				ux = ctou32(ip + 8 + 15*8)
				_u = (e.lutXd0[byte(_u)] | e.lutXd1[byte(_u>>8)] | e.lutXd2[byte(_u>>16)] | e.lutXd3[_u>>24])
				cu |= _u
				stou32(op+15*6, _u)
				_v := vx
				vx = ctou32(ip + 8 + 15*8 + 4)
				_v = (e.lutXd0[byte(_v)] | e.lutXd1[byte(_v>>8)] | e.lutXd2[byte(_v>>16)] | e.lutXd3[_v>>24])
				cu |= _v
				stou32(op+15*6+3, _v)
			}
			ip += 128
			op += (128 / 4) * 3
		}
		for ip < (ipstart+srclen)-(16+4) {
			{
				_u := ux
				ux = ctou32(ip + 8 + 0*8)
				_u = (e.lutXd0[byte(_u)] | e.lutXd1[byte(_u>>8)] | e.lutXd2[byte(_u>>16)] | e.lutXd3[_u>>24])
				cu |= _u
				stou32(op+0*6, _u)
				_v := vx
				vx = ctou32(ip + 8 + 0*8 + 4)
				_v = (e.lutXd0[byte(_v)] | e.lutXd1[byte(_v>>8)] | e.lutXd2[byte(_v>>16)] | e.lutXd3[_v>>24])
				cu |= _v
				stou32(op+0*6+3, _v)
			}
			{
				_u := ux
				ux = ctou32(ip + 8 + 1*8)
				_u = (e.lutXd0[byte(_u)] | e.lutXd1[byte(_u>>8)] | e.lutXd2[byte(_u>>16)] | e.lutXd3[_u>>24])
				cu |= _u
				stou32(op+1*6, _u)
				_v := vx
				vx = ctou32(ip + 8 + 1*8 + 4)
				_v = (e.lutXd0[byte(_v)] | e.lutXd1[byte(_v>>8)] | e.lutXd2[byte(_v>>16)] | e.lutXd3[_v>>24])
				cu |= _v
				stou32(op+1*6+3, _v)
			}
			ip += 16
			op += (16 / 4) * 3
		}
		if ip < (ipstart+srclen)-(8+4) {
			_u := ux
			_u = (e.lutXd0[byte(_u)] | e.lutXd1[byte(_u>>8)] | e.lutXd2[byte(_u>>16)] | e.lutXd3[_u>>24])
			cu |= _u
			stou32(op+0*6, _u)
			_v := vx
			_v = (e.lutXd0[byte(_v)] | e.lutXd1[byte(_v>>8)] | e.lutXd2[byte(_v>>16)] | e.lutXd3[_v>>24])
			cu |= _v
			stou32(op+0*6+3, _v)
			ip += 8
			op += (8 / 4) * 3
		}
	}
	for ip < (ipstart+srclen)-4 {
		u := ctou32(ip)
		u = (e.lutXd0[byte(u)] | e.lutXd1[byte(u>>8)] | e.lutXd2[byte(u>>16)] | e.lutXd3[u>>24])
		stou32(op, u)
		cu |= u
		ip += 4
		op += 3
	}
	var u uint32
	l := (ipstart + srclen) - ip
	if e.pad && l == 4 {
		if *(*byte)(unsafe.Pointer(ip + 3)) == '=' {
			l = 3
			if *(*byte)(unsafe.Pointer(ip + 2)) == '=' {
				l = 2
			}
		}
	}
	up := (*[4]byte)(unsafe.Pointer(&u))
	switch l {
	case 4:
		if !e.pad && op-opstart+3 > dstlen {
			return 0
		}
		u = ctou32(ip)
		u = (e.lutXd0[byte(u)] | e.lutXd1[byte(u>>8)] | e.lutXd2[byte(u>>16)] | e.lutXd3[u>>24])
		putTail(op, up, 3)
		op += 3
		cu |= u
		break
	case 3:
		if !e.pad && op-opstart+2 > dstlen {
			return 0
		}
		u = e.lutXd0[*(*byte)(unsafe.Pointer(ip + 0))] | e.lutXd1[*(*byte)(unsafe.Pointer(ip + 1))] | e.lutXd2[*(*byte)(unsafe.Pointer(ip + 2))]
		putTail(op, up, 2)
		op += 2
		cu |= u
		break
	case 2:
		if !e.pad && op-opstart >= dstlen {
			return 0
		}
		u = e.lutXd0[*(*byte)(unsafe.Pointer(ip + 0))] | e.lutXd1[*(*byte)(unsafe.Pointer(ip + 1))]
		putTail(op, up, 1)
		op++
		cu |= u
		break
	case 1:
		return 0
	}
	if cu == 0xffffffff {
		return 0
	}
	return int(op - opstart)
}
