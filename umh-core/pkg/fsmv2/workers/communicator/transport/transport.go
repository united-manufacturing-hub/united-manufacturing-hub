// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

// Transport defines the interface for communicating with the relay server.
//
// Transport implementations handle authentication and bidirectional messaging
// between edge instances and the backend relay server.
type Transport interface {
	// Authenticate obtains a JWT token from the relay server.
	//
	// This method exchanges pre-shared credentials (instanceUUID, email) for
	// a JWT token used in subsequent Push/Pull operations.
	//
	// Context handling:
	//   - Respects ctx.Done() for cancellation
	//   - No automatic timeout (caller should use context.WithTimeout)
	//
	// Idempotency:
	//   - Safe to call multiple times with same credentials
	//   - Each call returns a NEW token (previous tokens remain valid until expiry)
	//
	// Returns:
	//   - AuthResponse with JWT token and expiry on success
	//   - Error for network failures, auth failures (401), or server errors (5xx)
	//
	// Example:
	//   resp, err := transport.Authenticate(ctx, AuthRequest{
	//       InstanceUUID: "uuid-123",
	//       Email: "user@example.com",
	//   })
	//   if err != nil { return err }
	//   // Use resp.Token for Push/Pull
	Authenticate(ctx context.Context, req AuthRequest) (AuthResponse, error)

	// Pull retrieves pending messages from the relay server.
	//
	// This method fetches all available messages queued for this instance.
	// Messages are removed from the relay server's queue upon successful retrieval.
	//
	// Context handling:
	//   - Respects ctx.Done() for cancellation
	//   - No automatic timeout (caller should use context.WithTimeout)
	//
	// Idempotency:
	//   - NOT idempotent: each call REMOVES messages from server queue
	//   - Failed calls may leave messages on server (safe to retry)
	//
	// Returns:
	//   - Slice of UMHMessage (may be empty if no messages pending)
	//   - Error for network failures, auth failures (401), or server errors (5xx)
	//
	// Example:
	//   messages, err := transport.Pull(ctx, jwtToken)
	//   if err != nil { return err }
	//   for _, msg := range messages {
	//       // Process msg
	//   }
	Pull(ctx context.Context, jwtToken string) ([]*UMHMessage, error)

	// Push sends messages to the relay server.
	//
	// This method transmits messages from edge to backend (status updates,
	// responses to actions, etc.).
	//
	// Context handling:
	//   - Respects ctx.Done() for cancellation
	//   - No automatic timeout (caller should use context.WithTimeout)
	//
	// Idempotency:
	//   - NOT fully idempotent: retrying may cause duplicate messages on server
	//   - Safe to retry on network failure (backend should handle duplicates)
	//
	// Returns:
	//   - nil on success
	//   - Error for network failures, auth failures (401), or server errors (5xx)
	//
	// Example:
	//   err := transport.Push(ctx, jwtToken, []*UMHMessage{
	//       {InstanceUUID: "uuid-123", Email: "user@example.com", Content: "status"},
	//   })
	//   if err != nil { return err }
	Push(ctx context.Context, jwtToken string, messages []*UMHMessage) error

	// Close releases all resources held by the transport.
	//
	// This method should be called when the transport is no longer needed to
	// prevent resource leaks (HTTP clients, connection pools, etc.).
	//
	// Idempotency:
	//   - Safe to call multiple times (subsequent calls are no-ops)
	//   - Calling Push/Pull/Authenticate after Close results in undefined behavior
	//
	// Does not return an error (cleanup operations are best-effort).
	Close()
}
