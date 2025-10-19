package protocol

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
)

// Crypto abstracts E2E encryption with sender authentication for CSE sync protocol.
// Based on patent EP4512040A2: blind relay cannot authenticate E2E encrypted traffic,
// so encryption MUST include cryptographic authentication (AEAD/MAC) to prevent tampering.
//
// DESIGN DECISION: Combined encryption + authentication (not separate interfaces)
// WHY: Patent shows they're inseparable - every encrypted message MUST be authenticated.
// Separating them would allow unsafe usage (encrypt without auth).
// TRADE-OFF: Less flexible, but prevents security bugs.
// INSPIRED BY: TLS (encryption+MAC in one layer), NaCl secretbox (encrypt+auth primitive).
//
// This is Layer 3 (protocol layer) of the CSE architecture.
type Crypto interface {
	// Encrypt encrypts plaintext and adds sender authentication (MAC).
	// Returns ciphertext that only intended recipient can decrypt.
	//
	// Idempotent-safe: Encrypting same data twice yields different ciphertext via random nonce.
	// This enables safe retries on network failures without replay attack risk.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - plaintext: Data to encrypt (can be empty)
	//   - recipientID: Recipient identifier for key lookup (not embedded in ciphertext)
	//
	// Returns:
	//   - Ciphertext bytes (includes nonce + encrypted data + MAC)
	//   - CryptoEncryptionError if encryption fails
	//   - CryptoConfigError if recipientID is invalid
	Encrypt(ctx context.Context, plaintext []byte, recipientID string) ([]byte, error)

	// Decrypt decrypts ciphertext and verifies sender authentication.
	// Returns error if MAC verification fails or decryption fails.
	//
	// SECURITY: senderID MUST match the authenticated sender from MAC.
	// This prevents impersonation attacks where attacker replays valid
	// ciphertext but claims different sender.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - ciphertext: Encrypted data from Encrypt() (includes nonce + data + MAC)
	//   - senderID: Claimed sender from Transport layer (UNVERIFIED, must match MAC)
	//
	// Returns:
	//   - Plaintext bytes
	//   - CryptoDecryptionError if decryption fails
	//   - CryptoAuthenticationError if MAC verification fails (SECURITY CRITICAL)
	//   - CryptoConfigError if senderID is invalid
	Decrypt(ctx context.Context, ciphertext []byte, senderID string) ([]byte, error)
}

// CryptoEncryptionError indicates encryption operation failed.
// Common causes: key unavailable, RNG failure, context cancelled.
type CryptoEncryptionError struct{ Err error }

// CryptoDecryptionError indicates decryption operation failed.
// Common causes: corrupted ciphertext, wrong key, context cancelled.
type CryptoDecryptionError struct{ Err error }

// CryptoAuthenticationError indicates MAC verification failed.
// SECURITY CRITICAL: This means ciphertext was tampered or replayed.
// Always treat as potential attack, never ignore this error.
type CryptoAuthenticationError struct{ Err error }

// CryptoConfigError indicates configuration problem.
// Common causes: invalid recipientID/senderID, missing keys.
type CryptoConfigError struct{ Err error }

func (e CryptoEncryptionError) Error() string {
	return fmt.Sprintf("encryption failed: %v", e.Err)
}

func (e CryptoEncryptionError) Unwrap() error {
	return e.Err
}

func (e CryptoDecryptionError) Error() string {
	return fmt.Sprintf("decryption failed: %v", e.Err)
}

func (e CryptoDecryptionError) Unwrap() error {
	return e.Err
}

func (e CryptoAuthenticationError) Error() string {
	return fmt.Sprintf("authentication failed: %v", e.Err)
}

func (e CryptoAuthenticationError) Unwrap() error {
	return e.Err
}

func (e CryptoConfigError) Error() string {
	return fmt.Sprintf("crypto config error: %v", e.Err)
}

func (e CryptoConfigError) Unwrap() error {
	return e.Err
}

// MockCrypto provides a test implementation using XOR cipher with checksum authentication.
// NOT SECURE - for testing only. Real implementation will use AES-GCM or ChaCha20-Poly1305.
//
// Implements idempotent encryption via random nonce and simple checksum-based MAC.
// Supports error simulation for testing failure scenarios.
type MockCrypto struct {
	mu          sync.RWMutex
	failEncrypt bool
	failDecrypt bool
	failAuth    bool
	simulateErr error
}

// NewMockCrypto creates a new MockCrypto instance for testing.
// Starts with no simulated errors - use SimulateXXXFailure() to inject failures.
func NewMockCrypto() *MockCrypto {
	return &MockCrypto{}
}

// SimulateEncryptFailure makes the next Encrypt() call fail with err.
// Use ClearSimulatedErrors() to reset.
func (m *MockCrypto) SimulateEncryptFailure(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.failEncrypt = true
	m.simulateErr = err
}

// SimulateDecryptFailure makes the next Decrypt() call fail with err.
// Use ClearSimulatedErrors() to reset.
func (m *MockCrypto) SimulateDecryptFailure(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.failDecrypt = true
	m.simulateErr = err
}

// SimulateAuthFailure makes the next Decrypt() call fail MAC verification.
// Returns CryptoAuthenticationError. Use ClearSimulatedErrors() to reset.
func (m *MockCrypto) SimulateAuthFailure() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.failAuth = true
}

// ClearSimulatedErrors resets all simulated error states.
// Useful in test cleanup (AfterEach) to prevent test contamination.
func (m *MockCrypto) ClearSimulatedErrors() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.failEncrypt = false
	m.failDecrypt = false
	m.failAuth = false
	m.simulateErr = nil
}

func (m *MockCrypto) Encrypt(ctx context.Context, plaintext []byte, recipientID string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.failEncrypt {
		return nil, CryptoEncryptionError{Err: m.simulateErr}
	}

	select {
	case <-ctx.Done():
		return nil, CryptoEncryptionError{Err: ctx.Err()}
	default:
	}

	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return nil, CryptoEncryptionError{Err: fmt.Errorf("failed to generate nonce: %w", err)}
	}

	encrypted := make([]byte, len(plaintext))

	key := byte(0x42)
	for i, b := range plaintext {
		encrypted[i] = b ^ key
	}

	checksum := byte(0)
	for _, b := range encrypted {
		checksum ^= b
	}

	ciphertext := make([]byte, 0, len("MOCK:")+len(nonce)+1+len(encrypted))
	ciphertext = append(ciphertext, []byte("MOCK:")...)
	ciphertext = append(ciphertext, nonce...)
	ciphertext = append(ciphertext, checksum)
	ciphertext = append(ciphertext, encrypted...)

	return ciphertext, nil
}

func (m *MockCrypto) Decrypt(ctx context.Context, ciphertext []byte, senderID string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.failDecrypt {
		return nil, CryptoDecryptionError{Err: m.simulateErr}
	}

	if m.failAuth {
		return nil, CryptoAuthenticationError{Err: errors.New("MAC verification failed")}
	}

	select {
	case <-ctx.Done():
		return nil, CryptoDecryptionError{Err: ctx.Err()}
	default:
	}

	if len(ciphertext) < len("MOCK:")+12+1 {
		return nil, CryptoAuthenticationError{Err: errors.New("invalid ciphertext format")}
	}

	prefix := string(ciphertext[:5])
	if prefix != "MOCK:" {
		return nil, CryptoAuthenticationError{Err: errors.New("invalid MAC prefix")}
	}

	storedChecksum := ciphertext[5+12]
	encryptedData := ciphertext[5+12+1:]

	actualChecksum := byte(0)
	for _, b := range encryptedData {
		actualChecksum ^= b
	}

	if actualChecksum != storedChecksum {
		return nil, CryptoAuthenticationError{Err: errors.New("checksum verification failed")}
	}

	plaintext := make([]byte, len(encryptedData))

	key := byte(0x42)
	for i, b := range encryptedData {
		plaintext[i] = b ^ key
	}

	return plaintext, nil
}
