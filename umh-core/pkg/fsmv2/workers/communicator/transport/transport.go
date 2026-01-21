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
// Note: InstanceUUID uses json tag "umhInstance" to match backend API (models.UMHMessage).
type UMHMessage struct {
	InstanceUUID string `json:"umhInstance"`
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
	Token        string `json:"token"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"`
	InstanceUUID string `json:"uuid,omitempty"` // Instance UUID returned by backend
	InstanceName string `json:"name,omitempty"` // Instance name returned by backend
}

// PullPayload represents the response from /v2/instance/pull.
type PullPayload struct {
	UMHMessages []*UMHMessage `json:"UMHMessages"`
}

// PushPayload represents the request to /v2/instance/push.
type PushPayload struct {
	UMHMessages []*UMHMessage `json:"UMHMessages"`
}

// Transport defines the interface for communicating with the relay server.
//
// Transport implementations handle authentication and bidirectional messaging
// between edge instances and the backend relay server.
type Transport interface {
	// Authenticate obtains a JWT token from the relay server.
	// Exchanges credentials (instanceUUID, email) for a JWT used in Push/Pull.
	// Idempotent: safe to call multiple times; each returns a NEW token.
	// Returns error for network failures, auth failures (401), or server errors.
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

	// Reset recreates the underlying client to establish fresh connections.
	//
	// This method is useful when the transport is in a degraded state and
	// retrying with the same client isn't helping. It closes existing idle
	// connections and creates a new HTTP client with the same configuration.
	//
	// Use case: When DegradedState detects persistent failures (e.g., 5 consecutive
	// errors), it triggers Reset() to potentially resolve connection-level issues
	// like stale TCP connections, DNS caching problems, or corrupted connection state.
	//
	// Idempotency:
	//   - Safe to call multiple times
	//   - Does not affect in-flight requests (they will complete or timeout)
	//   - Subsequent Push/Pull/Authenticate calls use the new client
	//
	// Does not return an error (reset operations are best-effort).
	Reset()
}
