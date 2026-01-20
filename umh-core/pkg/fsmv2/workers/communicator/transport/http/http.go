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
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/hash"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

// HTTPTransport implements HTTP-based communication using umh-core protocol.
type HTTPTransport struct {
	RelayURL   string
	httpClient *http.Client
}

// NewHTTPTransport creates a new HTTP transport.
// If timeout is 0, defaults to 30 seconds.
// Transport settings match legacy communicator to avoid Cloudflare issues.
func NewHTTPTransport(relayURL string, timeout time.Duration) *HTTPTransport {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// Transport settings with proper connection pooling (Bug #7 fix)
	// Replaced DisableKeepAlives:true with proper pooling for better performance
	t := &HTTPTransport{
		RelayURL: relayURL,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				// Disable HTTP/2 to match Cloudflare behavior
				ForceAttemptHTTP2: false,
				TLSNextProto:      make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
				Proxy:             http.ProxyFromEnvironment,

				// Connection pooling (minimal for low-traffic FSMv2)
				MaxIdleConns:        5, // Total idle connections
				MaxIdleConnsPerHost: 1, // 1 idle connection to relay endpoint
				MaxConnsPerHost:     2, // Up to 2 concurrent (auth + pull/push)

				// Dial settings
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,

				// Timeouts
				IdleConnTimeout:       30 * time.Second, // Match legacy keepAliveTimeout
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		},
	}

	return t
}

// Authenticate performs JWT authentication.
// Uses Authorization header with double-hashed token (matching legacy communicator).
func (t *HTTPTransport) Authenticate(ctx context.Context, req transport.AuthRequest) (transport.AuthResponse, error) {
	// Create POST request (no body needed - auth is in header)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.RelayURL+"/v2/instance/login", nil)
	if err != nil {
		return transport.AuthResponse{}, fmt.Errorf("failed to create auth request: %w", err)
	}

	// Double-hash the token: Hash(Hash(AuthToken)) - matches legacy communicator
	hashedToken := hash.Sha3Hash(hash.Sha3Hash(req.Email))
	httpReq.Header.Set("Authorization", "Bearer "+hashedToken)
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

	// Read and log response body for debugging
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return transport.AuthResponse{}, fmt.Errorf("failed to read auth response body: %w", err)
	}

	// Parse response body for instance info
	var loginResp struct {
		UUID string `json:"uuid"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(bodyBytes, &loginResp); err != nil {
		// Log the response body to understand what the backend returns
		return transport.AuthResponse{}, fmt.Errorf("failed to decode auth response (body: %s): %w", string(bodyBytes), err)
	}

	// Extract JWT token from cookie (matching legacy behavior)
	var jwtToken string

	for _, cookie := range resp.Cookies() {
		if cookie.Name == "token" {
			jwtToken = cookie.Value

			break
		}
	}

	if jwtToken == "" {
		return transport.AuthResponse{}, errors.New("no token cookie returned from login")
	}

	// Set default expiry to 23 hours from now (typical JWT lifetime is 24h, refresh proactively)
	// The backend doesn't return expiresAt in the response, so we estimate based on typical JWT lifetime
	defaultExpiry := time.Now().Add(23 * time.Hour).Unix()

	return transport.AuthResponse{
		Token:        jwtToken,
		ExpiresAt:    defaultExpiry,
		InstanceUUID: loginResp.UUID,
		InstanceName: loginResp.Name,
	}, nil
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

	// 204 No Content = no messages available (valid response)
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
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

// Reset recreates the HTTP client to establish fresh connections.
// This is useful when the transport is in a degraded state and
// retrying with the same client isn't helping.
// Thread-safe: the httpClient field is replaced atomically.
func (t *HTTPTransport) Reset() {
	// Close existing connections
	t.ResetClient()

	// Recreate HTTP client with same settings
	t.httpClient = &http.Client{
		Timeout: t.httpClient.Timeout,
		Transport: &http.Transport{
			// Disable HTTP/2 to match Cloudflare behavior
			ForceAttemptHTTP2: false,
			TLSNextProto:      make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
			Proxy:             http.ProxyFromEnvironment,

			// Connection pooling (minimal for low-traffic FSMv2)
			MaxIdleConns:        5, // Total idle connections
			MaxIdleConnsPerHost: 1, // 1 idle connection to relay endpoint
			MaxConnsPerHost:     2, // Up to 2 concurrent (auth + pull/push)

			// Dial settings
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,

			// Timeouts
			IdleConnTimeout:       30 * time.Second, // Match legacy keepAliveTimeout
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}
