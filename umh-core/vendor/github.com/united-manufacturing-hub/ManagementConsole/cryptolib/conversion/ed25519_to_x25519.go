package conversion

import (
	"crypto/ed25519"
	"crypto/sha512"
	"fmt"

	"filippo.io/edwards25519"
)

/*
This package contains functions for converting Ed25519 keys to X25519 keys.

The Ed25519 curve is a twisted Edwards curve, while X25519 is a Montgomery curve.

The conversion is done by first converting the Ed25519 public key to an Edwards25519 point,
and then converting the Edwards25519 point to an X25519 point.

The private key conversion is done by first deriving the private key from the seed, and then clamping the private key.

This is similar to https://libsodium.gitbook.io/doc/advanced/ed25519-curve25519
See also: https://docs.rs/crate/ed25519_to_curve25519/latest/source/src/lib.rs
*/

// ConvertEd25519PublicKeyToX25519PublicKey converts an Ed25519 public key to a Curve25519 public key.
// This is useful for converting Ed25519 signing keys to X25519 key exchange keys.
func ConvertEd25519PublicKeyToX25519PublicKey(publicKey ed25519.PublicKey) ([]byte, error) {
	// Check if the public key has the correct length
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid Ed25519 public key length: %d", len(publicKey))
	}

	// Create an edwards25519 Point from the Ed25519 public key
	edPoint, err := edwards25519.NewIdentityPoint().SetBytes(publicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid Ed25519 public key: %w", err)
	}

	// Convert to X25519 (Montgomery form)
	x25519Bytes := edPoint.BytesMontgomery()

	return x25519Bytes, nil
}

// ConvertEd25519PrivateKeyToX25519PrivateKey converts an Ed25519 private key to an X25519 private key.
// This is useful for converting Ed25519 signing keys to X25519 key exchange keys.
func ConvertEd25519PrivateKeyToX25519PrivateKey(privateKey ed25519.PrivateKey) ([]byte, error) {
	// Check if the private key has the correct length
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid Ed25519 private key length: %d", len(privateKey))
	}

	// The Ed25519 private key is a 64-byte value where the first 32 bytes are the seed
	// and the last 32 bytes are the public key.
	seed := privateKey[:32]

	// The X25519 private key is derived from the Ed25519 seed by hashing it with SHA-512
	// and then clamping the first 32 bytes of the hash.
	h := sha512.Sum512(seed)

	// Clamp the private key according to the X25519 specification
	// See RFC 7748, Section 5
	h[0] &= 248
	h[31] &= 127
	h[31] |= 64

	// Return the first 32 bytes as the X25519 private key
	return h[:32], nil
}

// Ed25519ToX25519 is a convenience function that converts an Ed25519 public key to an X25519 public key.
// It accepts and returns byte slices for easier use.
func Ed25519ToX25519(ed25519PublicKey []byte) ([]byte, error) {
	// Check if the public key has the correct length
	if len(ed25519PublicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid Ed25519 public key length: %d", len(ed25519PublicKey))
	}

	return ConvertEd25519PublicKeyToX25519PublicKey(ed25519.PublicKey(ed25519PublicKey))
}

// Ed25519PrivateKeyToX25519 is a convenience function that converts an Ed25519 private key to an X25519 private key.
// It accepts and returns byte slices for easier use.
func Ed25519PrivateKeyToX25519(ed25519PrivateKey []byte) ([]byte, error) {
	// Check if the private key has the correct length
	if len(ed25519PrivateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid Ed25519 private key length: %d", len(ed25519PrivateKey))
	}

	return ConvertEd25519PrivateKeyToX25519PrivateKey(ed25519.PrivateKey(ed25519PrivateKey))
}

// ConvertEd25519SignatureToCurve25519Signature converts an Ed25519 signature to a Curve25519 signature.
// This is based on the Rust crate's ed25519_sign_to_curve25519 function.
// The conversion simply copies the signature and sets the sign bit from the public key.
func ConvertEd25519SignatureToCurve25519Signature(publicKey ed25519.PublicKey, signature []byte) ([]byte, error) {
	// Check if the public key has the correct length
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid Ed25519 public key length: %d", len(publicKey))
	}

	// Check if the signature has the correct length
	if len(signature) != ed25519.SignatureSize {
		return nil, fmt.Errorf("invalid Ed25519 signature length: %d", len(signature))
	}

	// Create a copy of the signature
	result := make([]byte, ed25519.SignatureSize)
	copy(result, signature)

	// Get the sign bit from the public key (highest bit of the last byte)
	signBit := publicKey[31] & 0x80

	// Set the sign bit in the signature
	result[63] = result[63] | signBit

	return result, nil
}

// Ed25519SignToCurve25519 is a convenience function that converts an Ed25519 signature to a Curve25519 signature.
// It accepts and returns byte slices for easier use.
func Ed25519SignToCurve25519(ed25519PublicKey, ed25519Signature []byte) ([]byte, error) {
	// Check if the public key has the correct length
	if len(ed25519PublicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid Ed25519 public key length: %d", len(ed25519PublicKey))
	}

	// Check if the signature has the correct length
	if len(ed25519Signature) != ed25519.SignatureSize {
		return nil, fmt.Errorf("invalid Ed25519 signature length: %d", len(ed25519Signature))
	}

	return ConvertEd25519SignatureToCurve25519Signature(ed25519.PublicKey(ed25519PublicKey), ed25519Signature)
}
