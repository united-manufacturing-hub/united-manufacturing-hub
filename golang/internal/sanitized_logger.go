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
	"fmt"
	"strings"
	"unicode"
)

func SanitizeStringArray(unsafestringarray []string) (safestrings []string) {
	safestrings = make([]string, len(unsafestringarray))
	for i := 0; i < len(unsafestringarray); i++ {
		safestrings[i] = SanitizeString(unsafestringarray[i])
	}
	return safestrings
}

// SanitizeByteArray formats the byte array and passes it through SanitizeString
func SanitizeByteArray(unsafebytearray []byte) (safestring string) {
	return SanitizeString(fmt.Sprintf("%08b", unsafebytearray))
}

// SanitizeString removes any rune that is not graphic and printable.
func SanitizeString(unsafestring string) (safestring string) {

	safestring = strings.Map(func(r rune) rune {
		if unicode.IsGraphic(r) && unicode.IsPrint(r) {
			return r
		}
		return -1
	}, unsafestring)
	return safestring
}
