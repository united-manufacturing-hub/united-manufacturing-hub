package compression

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"

// Handler handles encoding/decoding of message content (base64 + compression + JSON).
type Handler interface {
	Decode(content string) (models.UMHMessageContent, error)
	Encode(content models.UMHMessageContent) (string, error)
}
