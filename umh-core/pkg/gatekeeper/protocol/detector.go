package protocol

import (
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/encryption"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

type detector struct {
	v0Handler    encryption.Handler
	cseV1Handler encryption.Handler
	log          *zap.SugaredLogger
}

func NewDetector(log *zap.SugaredLogger) Detector {
	return &detector{
		v0Handler:    encryption.NewV0Handler(log),
		cseV1Handler: encryption.NewCseV1Handler(log),
		log:          log,
	}
}

func (d *detector) Detect(msg *models.UMHMessage) encryption.Handler {
	switch msg.ProtocolVersion {
	case models.CseV1:
		return d.cseV1Handler
	default:
		return d.v0Handler
	}
}
