package internal

import (
	"encoding/binary"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"
)

// AsXXHash returns the XXHash128 of the given data.
// This hash is extremely fast and reasonable for use as a key in a cache.
// https://cyan4973.github.io/xxHash/
func AsXXHash(inputs ...[]byte) []byte {
	h := xxh3.New()
	for _, input := range inputs {
		_, err := h.Write(input)
		if err != nil {
			zap.S().Errorf("Unable to write to hash: %v", err)
		}
	}

	return Uint128ToBytes(h.Sum128())
}

// Uint128ToBytes converts a uint128 to a byte array
func Uint128ToBytes(a xxh3.Uint128) (b []byte) {
	b = make([]byte, 16)
	binary.LittleEndian.PutUint64(b[0:8], a.Lo)
	binary.LittleEndian.PutUint64(b[8:16], a.Hi)
	return
}
