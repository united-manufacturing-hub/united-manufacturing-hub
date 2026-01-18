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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

// HTTPTransport implements HTTP-based communication using umh-core protocol.
type HTTPTransport struct {
	RelayURL   string
	httpClient *http.Client
}

// NewHTTPTransport creates a new HTTP transport.
// If timeout is 0, defaults to 30 seconds.
func NewHTTPTransport(relayURL string, timeout time.Duration) *HTTPTransport {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	t := &HTTPTransport{
		RelayURL: relayURL,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				DisableKeepAlives: true, // BUG #3 fix: no connection reuse, no stale connections
			},
		},
	}

	return t
}

// Authenticate performs JWT authentication.
func (t *HTTPTransport) Authenticate(ctx context.Context, req transport.AuthRequest) (transport.AuthResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return transport.AuthResponse{}, fmt.Errorf("failed to marshal auth request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.RelayURL+"/v2/instance/login", bytes.NewBuffer(body))
	if err != nil {
		return transport.AuthResponse{}, fmt.Errorf("failed to create auth request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return transport.AuthResponse{}, fmt.Errorf("auth request failed: %w", err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)

		return transport.AuthResponse{}, fmt.Errorf("auth failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var authResp transport.AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return transport.AuthResponse{}, fmt.Errorf("failed to decode auth response: %w", err)
	}

	return authResp, nil
}

// Pull retrieves messages from the backend (GET /v2/instance/pull).
func (t *HTTPTransport) Pull(ctx context.Context, jwtToken string) ([]*transport.UMHMessage, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, t.RelayURL+"/v2/instance/pull", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}

	// Add JWT token as cookie
	httpReq.AddCookie(&http.Cookie{Name: "token", Value: jwtToken})

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("pull request failed: %w", err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("authentication failed: %d", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)

		return nil, fmt.Errorf("pull failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var payload transport.PullPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to decode pull response: %w", err)
	}

	return payload.UMHMessages, nil
}

// Push sends messages to the backend (POST /v2/instance/push).
func (t *HTTPTransport) Push(ctx context.Context, jwtToken string, messages []*transport.UMHMessage) error {
	if len(messages) == 0 {
		return nil
	}

	payload := transport.PushPayload{
		UMHMessages: messages,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal push payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.RelayURL+"/v2/instance/push", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create push request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Add JWT token as cookie
	httpReq.AddCookie(&http.Cookie{Name: "token", Value: jwtToken})

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("push request failed: %w", err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("authentication failed: %d", resp.StatusCode)
	}

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)

		return fmt.Errorf("push failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// ResetClient resets the HTTP client (closes idle connections).
func (t *HTTPTransport) ResetClient() {
	if transport, ok := t.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
}

// Close closes the transport.
func (t *HTTPTransport) Close() {
	t.ResetClient()
}
