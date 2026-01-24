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
	InstanceUUID string `json:"uuid,omitempty"` // Instance UUID returned by backend
	InstanceName string `json:"name,omitempty"` // Instance name returned by backend
	ExpiresAt    int64  `json:"expiresAt,omitempty"`
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
type Transport interface {
	// Authenticate obtains a JWT token from the relay server.
	Authenticate(ctx context.Context, req AuthRequest) (AuthResponse, error)

	// Pull retrieves pending messages from the relay server (NOT idempotent: removes messages from queue).
	Pull(ctx context.Context, jwtToken string) ([]*UMHMessage, error)

	// Push sends messages to the relay server (retrying may cause duplicates).
	Push(ctx context.Context, jwtToken string, messages []*UMHMessage) error

	// Close releases all resources held by the transport. Safe to call multiple times.
	Close()

	// Reset recreates the underlying client to establish fresh connections when retries aren't helping.
	Reset()
}
