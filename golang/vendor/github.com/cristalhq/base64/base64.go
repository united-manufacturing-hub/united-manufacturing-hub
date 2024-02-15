// based on https://github.com/powturbo/Turbo-Base64
package base64

import (
	"errors"
	"unsafe"
)

var ErrWrongData = errors.New("wrong base64 data")

type Encoding struct {
	lutSe  [64]byte
	lutXe  [4096]uint32
	lutXd0 [256]uint32
	lutXd1 [256]uint32
	lutXd2 [256]uint32
	lutXd3 [256]uint32
	pad    bool
}

func (e *Encoding) EncodedLen(n int) int {
	if !e.pad {
		return (n*8 + 5) / 6
	}
	return (n + 2) / 3 * 4
}

func (e *Encoding) Encode(dst []byte, src []byte) {
	if len(src) == 0 {
		return
	}
	e.encode(dst, src, uintptr(e.EncodedLen(len(src))))
}

func (e *Encoding) EncodeToBytes(src []byte) []byte {
	if len(src) == 0 {
		return []byte{}
	}
	length := e.EncodedLen(len(src))
	result := make([]byte, length)
	e.encode(result, src, uintptr(length))
	return result
}

func (e *Encoding) EncodeToString(src []byte) string {
	return b2s(e.EncodeToBytes(src))
}

func (e *Encoding) EncodeString(src string) []byte {
	return e.EncodeToBytes(s2b(src))
}

func (e *Encoding) EncodeStringToString(src string) string {
	return b2s(e.EncodeToBytes(s2b(src)))
}

func (e *Encoding) DecodedLen(n int) int {
	sf := 0
	if n > 4 {
		sf++
	}
	if !e.pad {
		return n*6/8 + sf
	}
	return n/4*3 + sf
}

func (e *Encoding) Decode(dst []byte, src []byte) (int, error) {
	if len(src) == 0 {
		return 0, nil
	}
	n := e.decode(dst, src)
	if n == 0 {
		return 0, ErrWrongData
	}
	return n, nil
}

func (e *Encoding) DecodeToBytes(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return []byte{}, nil
	}
	length := e.DecodedLen(len(src))
	if length == 0 {
		return nil, ErrWrongData
	}
	result := make([]byte, length)
	n := e.decode(result, src)
	if n == 0 {
		return nil, ErrWrongData
	}
	(*sliceHeader)(unsafe.Pointer(&result)).len = n
	return result, nil
}

func (e *Encoding) DecodeToString(src []byte) (string, error) {
	result, err := e.DecodeToBytes(src)
	if err != nil {
		return "", err
	}
	return b2s(result), nil
}

func (e *Encoding) DecodeString(src string) ([]byte, error) {
	return e.DecodeToBytes(s2b(src))
}

func (e *Encoding) DecodeStringToString(src string) (string, error) {
	result, err := e.DecodeToBytes(s2b(src))
	if err != nil {
		return "", err
	}
	return b2s(result), nil
}

func NewEncoding(lutSe [64]byte, pad bool) *Encoding {
	lutXe, lutXd0, lutXd1, lutXd2, lutXd3 := makeLuts(lutSe)
	return &Encoding{
		lutSe:  lutSe,
		lutXe:  lutXe,
		lutXd0: lutXd0,
		lutXd1: lutXd1,
		lutXd2: lutXd2,
		lutXd3: lutXd3,
		pad:    pad,
	}
}

var (
	StdEncoding    = NewEncoding(stdLutSe, true)
	RawStdEncoding = NewEncoding(stdLutSe, false)
	URLEncoding    = NewEncoding(urlLutSe, true)
	RawURLEncoding = NewEncoding(urlLutSe, false)
)
