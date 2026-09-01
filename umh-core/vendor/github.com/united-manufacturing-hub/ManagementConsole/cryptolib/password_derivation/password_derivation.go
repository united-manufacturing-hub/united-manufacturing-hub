package password_derivation

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/united-manufacturing-hub/ManagementConsole/cryptolib/settings"
	"golang.org/x/crypto/argon2"
)

// deriveSaltFromEmail creates a deterministic salt from an email address
// by combining it with a server-side pepper and hashing the result
func deriveSaltFromEmail(email string) []byte {
	hash := sha256.New()
	hash.Write([]byte(email))
	hash.Write([]byte(settings.ARGON2ID_STATIC_PEPPER))
	return hash.Sum(nil)
}

// DerivePrivateKeyEncryptionKeyFromPassword generates a hex-encoded 32-byte encryption key
// from a password and email combination using Argon2id.
func DerivePrivateKeyEncryptionKeyFromPassword(password, email string) string {

	salt := deriveSaltFromEmail(email)
	key := argon2.IDKey(
		[]byte(password),
		salt,
		settings.ARGON2ID_TIME,
		settings.ARGON2ID_MEMORY,
		settings.ARGON2ID_THREADS,
		settings.ARGON2ID_KEY_LENGTH,
	)
	return hex.EncodeToString(key)
}

// DeriveCAEncryptionKeyFromPassword generates a hex-encoded 32-byte encryption key
// by applying the derivation twice with different salt modifications
func DeriveCAEncryptionKeyFromPassword(password, email string) string {
	// First derivation
	firstKey := DerivePrivateKeyEncryptionKeyFromPassword(password, email)

	// Second derivation with modified salt
	salt := deriveSaltFromEmail(email + "_CA")
	key := argon2.IDKey(
		[]byte(firstKey),
		salt,
		settings.ARGON2ID_TIME,
		settings.ARGON2ID_MEMORY,
		settings.ARGON2ID_THREADS,
		settings.ARGON2ID_KEY_LENGTH,
	)
	return hex.EncodeToString(key)
}
