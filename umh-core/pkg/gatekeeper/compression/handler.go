package compression

import (
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

type handler struct {
	log *zap.SugaredLogger
}

func NewHandler(log *zap.SugaredLogger) Handler {
	return &handler{log: log}
}

func (h *handler) Decode(content string) (models.UMHMessageContent, error) {
	return encoding.DecodeMessageFromUserToUMHInstance(content)
}

func (h *handler) Encode(content models.UMHMessageContent) (string, error) {
	return encoding.EncodeMessageFromUMHInstanceToUser(content)
}
