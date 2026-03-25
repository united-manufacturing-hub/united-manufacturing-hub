package key_encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/united-manufacturing-hub/ManagementConsole/cryptolib/conversion"
	"github.com/united-manufacturing-hub/ManagementConsole/cryptolib/settings"
	"golang.org/x/crypto/argon2"
)

// EncryptedData represents an encrypted Ed25519 private key
type EncryptedData struct {
	// Salt for Argon2id
	Salt []byte
	// Nonce for AES-GCM
	Nonce []byte
	// Encrypted key data
	Data []byte
	// Argon2id parameters
	Time    uint32
	Memory  uint32
	Threads uint8
}

// EncryptPrivateKey encrypts an Ed25519 private key using Argon2id password-based key derivation and AES-256-GCM.
func EncryptPrivateKey(key ed25519.PrivateKey, password string) (*EncryptedData, error) {
	// Generate a random salt
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, errors.Join(err, errors.New("failed to generate salt"))
	}

	// Derive key using Argon2id
	derivedKey := argon2.IDKey(
		[]byte(password),
		salt,
		settings.ARGON2ID_TIME,
		settings.ARGON2ID_MEMORY,
		settings.ARGON2ID_THREADS,
		settings.ARGON2ID_KEY_LENGTH,
	)

	// Create AES-256-GCM cipher
	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to create AES cipher"))
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to create GCM"))
	}

	// Generate nonce
	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, errors.Join(err, errors.New("failed to generate nonce"))
	}

	// Encrypt the private key
	ciphertext := aesgcm.Seal(nil, nonce, key, nil)

	return &EncryptedData{
		Salt:    salt,
		Nonce:   nonce,
		Data:    ciphertext,
		Time:    settings.ARGON2ID_TIME,
		Memory:  settings.ARGON2ID_MEMORY,
		Threads: settings.ARGON2ID_THREADS,
	}, nil
}

// DecryptPrivateKey decrypts an Ed25519 private key that was encrypted using EncryptPrivateKey.
func DecryptPrivateKey(enc *EncryptedData, password string) (ed25519.PrivateKey, error) {
	// Derive key using stored parameters
	derivedKey := argon2.IDKey(
		[]byte(password),
		enc.Salt,
		enc.Time,
		enc.Memory,
		enc.Threads,
		32,
	)

	// Create AES-256-GCM cipher
	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to create AES cipher"))
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to create GCM"))
	}

	// Decrypt the private key
	plaintext, err := aesgcm.Open(nil, enc.Nonce, enc.Data, nil)
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to decrypt private key"))
	}

	// Verify the decrypted key is a valid Ed25519 private key
	if len(plaintext) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid decrypted key size")
	}

	return ed25519.PrivateKey(plaintext), nil
}

// EncryptPrivateKeyWithPublicKey encrypts an Ed25519 private key using another Ed25519 public key.
// This is done by converting both keys to X25519 format, performing ECDH key exchange,
// and using the resulting shared secret to encrypt the private key with AES-GCM.
func EncryptPrivateKeyWithPublicKey(ourPrivateKey ed25519.PrivateKey, theirPublicKey ed25519.PublicKey, keyToEncrypt ed25519.PrivateKey) (*EncryptedData, error) {
	// Convert our Ed25519 private key to X25519 private key
	ourX25519PrivateKey, err := conversion.ConvertEd25519PrivateKeyToX25519PrivateKey(ourPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert our Ed25519 private key to X25519: %w", err)
	}

	// Convert their Ed25519 public key to X25519 public key
	theirX25519PublicKey, err := conversion.ConvertEd25519PublicKeyToX25519PublicKey(theirPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert their Ed25519 public key to X25519: %w", err)
	}

	// Create ECDH keys
	curve := ecdh.X25519()

	ourECDHPrivateKey, err := curve.NewPrivateKey(ourX25519PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create ECDH private key: %w", err)
	}

	theirECDHPublicKey, err := curve.NewPublicKey(theirX25519PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create ECDH public key: %w", err)
	}

	// Perform key exchange to get shared secret
	sharedSecret, err := ourECDHPrivateKey.ECDH(theirECDHPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to perform ECDH key exchange: %w", err)
	}

	// Create AES-GCM cipher using the shared secret
	// We'll use Argon2id to derive a suitable key from the shared secret
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// Argon2id parameters
	time := uint32(1)
	memory := uint32(64 * 1024)
	threads := uint8(4)
	keyLen := uint32(32) // 256-bit key for AES-256

	// Derive key using Argon2id
	key := argon2.IDKey(sharedSecret, salt, time, memory, threads, keyLen)

	// Create AES-GCM cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate nonce
	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the private key
	ciphertext := aesgcm.Seal(nil, nonce, keyToEncrypt, nil)

	// Return the encrypted key
	return &EncryptedData{
		Salt:    salt,
		Nonce:   nonce,
		Data:    ciphertext,
		Time:    time,
		Memory:  memory,
		Threads: threads,
	}, nil
}

// DecryptPrivateKeyWithPublicKey decrypts an Ed25519 private key that was encrypted using EncryptPrivateKeyWithPublicKey.
// It requires the public key of the party who encrypted the key and the private key of the party for whom it was encrypted.
// EncryptData encrypts arbitrary data using Argon2id key derivation and AES-256-GCM.
// This is a generalized version of EncryptPrivateKey that works with any byte data.
func EncryptData(data []byte, password string) (*EncryptedData, error) {
	// Generate a random salt
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, errors.Join(err, errors.New("failed to generate salt"))
	}

	// Derive key using Argon2id
	derivedKey := argon2.IDKey(
		[]byte(password),
		salt,
		settings.ARGON2ID_TIME,
		settings.ARGON2ID_MEMORY,
		settings.ARGON2ID_THREADS,
		settings.ARGON2ID_KEY_LENGTH,
	)

	// Create AES-256-GCM cipher
	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to create AES cipher"))
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to create GCM"))
	}

	// Generate nonce
	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, errors.Join(err, errors.New("failed to generate nonce"))
	}

	// Encrypt the data
	ciphertext := aesgcm.Seal(nil, nonce, data, nil)

	return &EncryptedData{
		Salt:    salt,
		Nonce:   nonce,
		Data:    ciphertext,
		Time:    settings.ARGON2ID_TIME,
		Memory:  settings.ARGON2ID_MEMORY,
		Threads: settings.ARGON2ID_THREADS,
	}, nil
}

// DecryptData decrypts data that was encrypted using EncryptData.
func DecryptData(enc *EncryptedData, password string) ([]byte, error) {
	// Derive key using stored parameters
	derivedKey := argon2.IDKey(
		[]byte(password),
		enc.Salt,
		enc.Time,
		enc.Memory,
		enc.Threads,
		32,
	)

	// Create AES-256-GCM cipher
	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to create AES cipher"))
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to create GCM"))
	}

	// Decrypt the data
	plaintext, err := aesgcm.Open(nil, enc.Nonce, enc.Data, nil)
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to decrypt data"))
	}

	return plaintext, nil
}

func DecryptPrivateKeyWithPrivateKey(enc *EncryptedData, ourPrivateKey ed25519.PrivateKey, ourPrivateKeyPassword string, theirPublicKey ed25519.PublicKey) (ed25519.PrivateKey, error) {
	// Convert our Ed25519 private key to X25519 private key
	ourX25519PrivateKey, err := conversion.ConvertEd25519PrivateKeyToX25519PrivateKey(ourPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert our Ed25519 private key to X25519: %w", err)
	}

	// Convert their Ed25519 public key to X25519 public key
	theirX25519PublicKey, err := conversion.ConvertEd25519PublicKeyToX25519PublicKey(theirPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert their Ed25519 public key to X25519: %w", err)
	}

	// Create ECDH keys
	curve := ecdh.X25519()

	ourECDHPrivateKey, err := curve.NewPrivateKey(ourX25519PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create ECDH private key: %w", err)
	}

	theirECDHPublicKey, err := curve.NewPublicKey(theirX25519PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create ECDH public key: %w", err)
	}

	// Perform key exchange to get shared secret
	sharedSecret, err := ourECDHPrivateKey.ECDH(theirECDHPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to perform ECDH key exchange: %w", err)
	}

	// Derive key using Argon2id with the stored parameters
	key := argon2.IDKey(sharedSecret, enc.Salt, enc.Time, enc.Memory, enc.Threads, 32)

	// Create AES-GCM cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt the private key
	plaintext, err := aesgcm.Open(nil, enc.Nonce, enc.Data, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt private key using private key: %w", err)
	}

	// Verify the decrypted key is a valid Ed25519 private key
	if len(plaintext) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid decrypted key size")
	}

	return ed25519.PrivateKey(plaintext), nil
}
