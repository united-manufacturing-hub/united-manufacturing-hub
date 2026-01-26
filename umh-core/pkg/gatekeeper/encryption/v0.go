package encryption

import "go.uber.org/zap"

type V0Handler struct {
	log *zap.SugaredLogger
}

func NewV0Handler(log *zap.SugaredLogger) Handler {
	return &V0Handler{log: log}
}

func (h *V0Handler) Decrypt(data []byte) ([]byte, error) {
	return data, nil
}

func (h *V0Handler) Encrypt(data []byte) ([]byte, error) {
	return data, nil
}
