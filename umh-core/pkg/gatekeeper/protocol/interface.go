package protocol

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/encryption"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// Detector detects protocol version and returns the appropriate encryption handler.
type Detector interface {
	Detect(msg *models.UMHMessage) encryption.Handler
}
