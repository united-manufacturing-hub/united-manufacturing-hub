package encoder

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/united-manufacturing-hub/ManagementConsole/cryptolib/key_encryption"
)

func EncodeX509Certificate(cert *x509.Certificate) string {
	return base64.StdEncoding.EncodeToString(cert.Raw)
}

func DecodeX509Certificate(encoded string) (*x509.Certificate, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to decode base64"))
	}
	return x509.ParseCertificate(decoded)
}

func EncodeEd25519PrivateKey(key ed25519.PrivateKey, password string) (string, error) {
	// Encrypt the key first
	encrypted, err := key_encryption.EncryptPrivateKey(key, password)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encrypt key"))
	}

	// Encode the encrypted key data as JSON
	jsonData, err := json.Marshal(encrypted)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encode encrypted key"))
	}

	// Base64 encode the JSON data
	return base64.StdEncoding.EncodeToString(jsonData), nil
}

func DecodeEd25519PrivateKey(encoded string, password string) (ed25519.PrivateKey, error) {
	// Decode base64
	jsonData, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to decode base64"))
	}

	// Unmarshal JSON into EncryptedData
	var encrypted key_encryption.EncryptedData
	if err := json.Unmarshal(jsonData, &encrypted); err != nil {
		return nil, errors.Join(err, errors.New("failed to unmarshal JSON"))
	}

	// Decrypt the key
	decrypted, err := key_encryption.DecryptPrivateKey(&encrypted, password)
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to decrypt key"))
	}

	return decrypted, nil
}

func ReEncodeEd25519PrivateKey(encoded string, oldPassword string, newPassword string) (string, error) {
	decoded, err := DecodeEd25519PrivateKey(encoded, oldPassword)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to decode key"))
	}

	// Encode the key with the new password
	return EncodeEd25519PrivateKey(decoded, newPassword)
}

func EncodeEd25519PrivateKeyWithPublicKey(ourPrivateKey ed25519.PrivateKey, theirPublicKey ed25519.PublicKey, keyToEncrypt ed25519.PrivateKey) (string, error) {
	epk, err := key_encryption.EncryptPrivateKeyWithPublicKey(ourPrivateKey, theirPublicKey, keyToEncrypt)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encrypt private key"))
	}

	// Encode the encrypted key data as JSON
	jsonData, err := json.Marshal(epk)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encode encrypted key"))
	}

	// Base64 encode the JSON data
	return base64.StdEncoding.EncodeToString(jsonData), nil
}

func DecodeEd25519PrivateKeyWithPrivateKey(encoded string, ourPrivateKey ed25519.PrivateKey, ourPrivateKeyPassword string, theirPublicKey ed25519.PublicKey) (ed25519.PrivateKey, error) {
	// Decode base64
	jsonData, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to decode base64"))
	}

	// Unmarshal JSON into EncryptedData
	var epk key_encryption.EncryptedData
	if err := json.Unmarshal(jsonData, &epk); err != nil {
		return nil, errors.Join(err, errors.New("failed to unmarshal JSON"))
	}

	// Decrypt the key
	decrypted, err := key_encryption.DecryptPrivateKeyWithPrivateKey(&epk, ourPrivateKey, ourPrivateKeyPassword, theirPublicKey)
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to decrypt private key"))
	}

	return decrypted, nil
}

// EncryptCertificate encrypts a certificate string using Argon2id + AES-256-GCM.
// The derivedPassword should already be derived using DerivePrivateKeyEncryptionKeyFromPassword.
func EncryptCertificate(certString string, derivedPassword string) (string, error) {
	// Encrypt the certificate data
	encrypted, err := key_encryption.EncryptData([]byte(certString), derivedPassword)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encrypt certificate"))
	}

	// Encode the encrypted data as JSON
	jsonData, err := json.Marshal(encrypted)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encode encrypted certificate"))
	}

	// Base64 encode the JSON data
	return base64.StdEncoding.EncodeToString(jsonData), nil
}

// DecryptCertificate decrypts an encrypted certificate string.
// The derivedPassword should already be derived using DerivePrivateKeyEncryptionKeyFromPassword.
func DecryptCertificate(encryptedCert string, derivedPassword string) (string, error) {
	// Decode base64
	jsonData, err := base64.StdEncoding.DecodeString(encryptedCert)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to decode base64-encoded certificate"))
	}

	// Unmarshal JSON into key_encryption.EncryptedData (reusing the struct for encrypted data)
	var encrypted key_encryption.EncryptedData
	if err := json.Unmarshal(jsonData, &encrypted); err != nil {
		return "", errors.Join(err, errors.New("failed to unmarshal JSON"))
	}

	// Decrypt the certificate
	decrypted, err := key_encryption.DecryptData(&encrypted, derivedPassword)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to decrypt certificate"))
	}

	return string(decrypted), nil
}
