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
