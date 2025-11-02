package transport

import "context"

// UMHMessage represents a message in the umh-core push/pull protocol.
type UMHMessage struct {
	InstanceUUID string `json:"instanceUUID"`
	Content      string `json:"content"`
	Email        string `json:"email"`
}

// AuthRequest represents an authentication request.
type AuthRequest struct {
	InstanceUUID string `json:"instanceUUID"`
	Email        string `json:"email"`
}

// AuthResponse represents an authentication response.
type AuthResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expiresAt,omitempty"`
}

// PullPayload represents the response from /v2/instance/pull.
type PullPayload struct {
	UMHMessages []*UMHMessage `json:"umhMessages"`
}

// PushPayload represents the request to /v2/instance/push.
type PushPayload struct {
	UMHMessages []*UMHMessage `json:"umhMessages"`
}

type Transport interface {
	Authenticate(ctx context.Context, req AuthRequest) (AuthResponse, error)
	Pull(ctx context.Context, jwtToken string) ([]*UMHMessage, error)
	Push(ctx context.Context, jwtToken string, messages []*UMHMessage) error
	Close()
}
