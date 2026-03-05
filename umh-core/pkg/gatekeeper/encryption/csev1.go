package encryption

import (
	"errors"

	"go.uber.org/zap"
)

// CseV1Handler handles client-side encryption v1. Not yet implemented.
type CseV1Handler struct {
	log *zap.SugaredLogger
}

// NewCseV1Handler creates a client-side encryption v1 handler (stub, not yet implemented).
func NewCseV1Handler(log *zap.SugaredLogger) Handler {
	return &CseV1Handler{log: log}
}

func (h *CseV1Handler) Decrypt(data []byte) ([]byte, error) {
	// TODO: Implement actual decryption
	return nil, errors.New("cseV1 decryption not yet implemented")
}

func (h *CseV1Handler) Encrypt(data []byte) ([]byte, error) {
	// TODO: Implement actual encryption
	return nil, errors.New("cseV1 encryption not yet implemented")
}
