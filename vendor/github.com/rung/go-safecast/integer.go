package safecast

import "errors"

// Int32 converts int to int32 in a safe way.
// You get error when the value is out of the 32-bit range.
func Int32(i int) (int32, error) {
	if i > 2147483647 || i < -2147483648 {
		return 0, errors.New("int32 out of range")
	}
	return int32(i), nil
}

// Int16 converts int to int16 in a safe way.
// You get error when the value is out of the 16-bit range.
func Int16(i int) (int16, error) {
	if i > 32767 || i < -32768 {
		return 0, errors.New("int16 out of range")
	}
	return int16(i), nil
}

// Int8 converts int to int8 in a safe way.
// You get error when the value is out of the 8-bit range.
func Int8(i int) (int8, error) {
	if i > 127 || i < -128 {
		return 0, errors.New("int8 out of range")
	}
	return int8(i), nil
}
