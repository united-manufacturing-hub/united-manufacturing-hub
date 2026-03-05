// Package protocol provides message protocol version detection.
package protocol

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/encryption"
)

// Detector detects protocol version and returns the appropriate encryption handler.
type Detector interface {
	Detect(msg *transport.UMHMessage) encryption.Handler
}
