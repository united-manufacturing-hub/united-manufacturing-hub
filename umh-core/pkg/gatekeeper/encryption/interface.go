package encryption

// Handler handles encryption/decryption for a specific protocol version.
type Handler interface {
	Decrypt(data []byte) ([]byte, error)
	Encrypt(data []byte) ([]byte, error)
}
