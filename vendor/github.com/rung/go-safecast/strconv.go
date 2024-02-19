package safecast

import "strconv"

// Atoi32 converts string to int32 in a safe way.
// You get error when the value is out of the 32-bit range.
//
// This is a wrapper function of strconv.ParseInt.
func Atoi32(s string) (int32, error) {
	i, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(i), nil
}

// Atoi16 converts string to int16 in a safe way.
// You get error when the value is out of the 16-bit range.
//
// This is a wrapper function of strconv.ParseInt.
func Atoi16(s string) (int16, error) {
	i, err := strconv.ParseInt(s, 10, 16)
	if err != nil {
		return 0, err
	}
	return int16(i), nil
}

// Atoi8 converts string to int8 in a safe way.
// You get error when the value is out of the 8-bit range.
//
// This is a wrapper function of strconv.ParseInt.
func Atoi8(s string) (int8, error) {
	i, err := strconv.ParseInt(s, 10, 8)
	if err != nil {
		return 0, err
	}
	return int8(i), nil
}