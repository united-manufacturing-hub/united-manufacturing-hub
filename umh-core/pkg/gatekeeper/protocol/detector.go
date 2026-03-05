package protocol

import (
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/encryption"
)

type detector struct {
	v0Handler    encryption.Handler
	cseV1Handler encryption.Handler
	log          *zap.SugaredLogger
}

// NewDetector creates a Detector that routes messages to the appropriate encryption handler.
func NewDetector(log *zap.SugaredLogger) Detector {
	return &detector{
		v0Handler:    encryption.NewV0Handler(log),
		cseV1Handler: encryption.NewCseV1Handler(log),
		log:          log,
	}
}

func (d *detector) Detect(msg *transport.UMHMessage) encryption.Handler {
	switch msg.ProtocolVersion {
	case transport.CseV1:
		return d.cseV1Handler
	default:
		return d.v0Handler
	}
}
