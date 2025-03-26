package hash

import (
	"fmt"

	"golang.org/x/crypto/sha3"
)

func Sha3Hash(input string) string {

	// Create a new hash & write input string
	hash := sha3.New256()
	_, _ = hash.Write([]byte(input))

	// Get the resulting encoded byte slice
	sha3 := hash.Sum(nil)

	// Convert the encoded byte slice to a string
	return fmt.Sprintf("%x", sha3)
}
