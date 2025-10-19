package protocol

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
)

type Crypto interface {
	Encrypt(ctx context.Context, plaintext []byte, recipientID string) ([]byte, error)

	Decrypt(ctx context.Context, ciphertext []byte, senderID string) ([]byte, error)
}

type CryptoEncryptionError struct{ Err error }
type CryptoDecryptionError struct{ Err error }
type CryptoAuthenticationError struct{ Err error }
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

type MockCrypto struct {
	mu          sync.RWMutex
	failEncrypt bool
	failDecrypt bool
	failAuth    bool
	simulateErr error
}

func NewMockCrypto() *MockCrypto {
	return &MockCrypto{}
}

func (m *MockCrypto) SimulateEncryptFailure(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.failEncrypt = true
	m.simulateErr = err
}

func (m *MockCrypto) SimulateDecryptFailure(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.failDecrypt = true
	m.simulateErr = err
}

func (m *MockCrypto) SimulateAuthFailure() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.failAuth = true
}

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
