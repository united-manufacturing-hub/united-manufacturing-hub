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

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

// UMHMessage represents a message in the umh-core push/pull protocol
type UMHMessage struct {
	InstanceUUID string `json:"instanceUUID"`
	Content      string `json:"content"`
	Email        string `json:"email"`
}

// AuthRequest represents an authentication request
type AuthRequest struct {
	InstanceUUID string `json:"instanceUUID"`
	Email        string `json:"email"`
}

// AuthResponse represents an authentication response
type AuthResponse struct {
	Token string `json:"token"`
}

// PullPayload represents the response from /v2/instance/pull
type PullPayload struct {
	UMHMessages []*UMHMessage `json:"umhMessages"`
}

// PushPayload represents the request to /v2/instance/push
type PushPayload struct {
	UMHMessages []*UMHMessage `json:"umhMessages"`
}

// HTTPTransport implements HTTP-based communication using umh-core protocol
type HTTPTransport struct {
	apiURL     string
	token      atomic.Value // stores string
	httpClient *http.Client
}

// NewHTTPTransport creates a new HTTP transport
func NewHTTPTransport(apiURL string) *HTTPTransport {
	t := &HTTPTransport{
		apiURL: apiURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				IdleConnTimeout: 90 * time.Second,
			},
		},
	}
	t.token.Store("")
	return t
}

// Authenticate performs JWT authentication
func (t *HTTPTransport) Authenticate(ctx context.Context, req AuthRequest) (AuthResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return AuthResponse{}, fmt.Errorf("failed to marshal auth request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", t.apiURL+"/v2/auth", bytes.NewBuffer(body))
	if err != nil {
		return AuthResponse{}, fmt.Errorf("failed to create auth request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return AuthResponse{}, fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return AuthResponse{}, fmt.Errorf("auth failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var authResp AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return AuthResponse{}, fmt.Errorf("failed to decode auth response: %w", err)
	}

	// Store token for future requests
	t.token.Store(authResp.Token)

	return authResp, nil
}

// Pull retrieves messages from the backend (GET /v2/instance/pull)
func (t *HTTPTransport) Pull(ctx context.Context) ([]*UMHMessage, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", t.apiURL+"/v2/instance/pull", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}

	// Add JWT token as cookie
	token := t.token.Load().(string)
	if token != "" {
		httpReq.AddCookie(&http.Cookie{Name: "token", Value: token})
	}

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("pull request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("authentication failed: %d", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("pull failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var payload PullPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to decode pull response: %w", err)
	}

	return payload.UMHMessages, nil
}

// Push sends messages to the backend (POST /v2/instance/push)
func (t *HTTPTransport) Push(ctx context.Context, messages []*UMHMessage) error {
	if len(messages) == 0 {
		return nil
	}

	payload := PushPayload{
		UMHMessages: messages,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal push payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", t.apiURL+"/v2/instance/push", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create push request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Add JWT token as cookie
	token := t.token.Load().(string)
	if token != "" {
		httpReq.AddCookie(&http.Cookie{Name: "token", Value: token})
	}

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("push request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("authentication failed: %d", resp.StatusCode)
	}

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("push failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// UpdateToken updates the JWT token
func (t *HTTPTransport) UpdateToken(token string) {
	t.token.Store(token)
}

// ResetClient resets the HTTP client (closes idle connections)
func (t *HTTPTransport) ResetClient() {
	if transport, ok := t.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
}

// Close closes the transport
func (t *HTTPTransport) Close() error {
	t.ResetClient()
	return nil
}
