// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
