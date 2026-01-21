package encryption

import (
	"errors"

	"go.uber.org/zap"
)

type CseV1Handler struct {
	log *zap.SugaredLogger
}

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
