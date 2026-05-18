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
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/hash"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

const (
	// LongPollingDuration is the HTTP timeout for long polling requests to the relay server.
	LongPollingDuration = 30 * time.Second
	// LongPollingBuffer is added to action timeout to prevent premature cancellation.
	LongPollingBuffer = 1 * time.Second
)

// isCloudflareChallenge detects Cloudflare challenge pages via headers and body content.
//
// Cloudflare challenge pages occur when Cloudflare's anti-bot protection activates.
// Unlike backend rate limiting (which returns JSON with Retry-After), Cloudflare
// challenges return HTML pages with JavaScript challenges.
//
// Detection criteria:
//   - HTTP 429 status code (required)
//   - "Server: cloudflare" header (optional but strong signal)
//   - Body content markers (at least one required):
//   - "Just a moment" - Cloudflare waiting page title text
//   - "challenge-form" - Cloudflare challenge form element ID
//   - "cf-browser-verification" - Cloudflare browser verification div ID
//
// These strings identify Cloudflare challenge pages because Cloudflare generates
// challenge pages server-side with consistent HTML markers that have remained
// stable across production versions. "Just a moment" appears in all challenge
// page variants, while "challenge-form" and "cf-browser-verification" are
// element IDs specific to Cloudflare-generated pages.
//
// Some Cloudflare configurations may not include the Server header, so body
// content alone is also checked as a fallback.
func isCloudflareChallenge(statusCode int, headers http.Header, body []byte) bool {
	if statusCode != http.StatusTooManyRequests {
		return false
	}

	server := headers.Get("Server")
	if strings.Contains(strings.ToLower(server), "cloudflare") {
		return bytes.Contains(body, []byte("Just a moment")) ||
			bytes.Contains(body, []byte("challenge-form")) ||
			bytes.Contains(body, []byte("cf-browser-verification"))
	}

	// Check body without server header (some Cloudflare configurations)
	return bytes.Contains(body, []byte("Just a moment")) ||
		bytes.Contains(body, []byte("challenge-form"))
}

// isProxyBlock detects proxy block pages (Zscaler, BlueCoat, etc.).
func isProxyBlock(body []byte) bool {
	signatures := [][]byte{
		[]byte("Zscaler"),
		[]byte("BlueCoat"),
		[]byte("Access Denied"),
		[]byte("This site has been blocked"),
		[]byte("web filter"),
		[]byte("Security Policy"),
		[]byte("Websense"),
		[]byte("FortiGuard"),
	}
	for _, sig := range signatures {
		if bytes.Contains(body, sig) {
			return true
		}
	}

	return false
}

// parseRetryAfter extracts Retry-After header value (RFC 7231).
// Returns 0 if header missing or invalid.
func parseRetryAfter(headers http.Header) time.Duration {
	value := headers.Get("Retry-After")
	if value == "" {
		return 0
	}

	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second
	}

	if t, err := http.ParseTime(value); err == nil {
		delay := time.Until(t)
		if delay > 0 {
			return delay
		}
	}

	return 0
}

// classifyError determines error type from HTTP response.
// Classification order: most specific first (Cloudflare), then general (429), then status codes.
func classifyError(statusCode int, body []byte, headers http.Header) types.ErrorType {
	// Cloudflare challenge (must check before generic 429)
	if isCloudflareChallenge(statusCode, headers, body) {
		return types.ErrorTypeCloudflareChallenge
	}
	// Proxy block (Zscaler, etc.)
	if isProxyBlock(body) {
		return types.ErrorTypeProxyBlock
	}
	// Backend rate limit
	if statusCode == http.StatusTooManyRequests {
		return types.ErrorTypeBackendRateLimit
	}
	// Auth errors
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		return types.ErrorTypeInvalidToken
	}
	// Instance deleted
	if statusCode == http.StatusNotFound {
		return types.ErrorTypeInstanceDeleted
	}
	// Server errors
	if statusCode >= 500 {
		return types.ErrorTypeServerError
	}

	return types.ErrorTypeUnknown
}

// newTransportError creates a classified transport error from an HTTP response.
func newTransportError(statusCode int, body []byte, headers http.Header, baseErr error) *types.TransportError {
	errType := classifyError(statusCode, body, headers)
	retryAfter := parseRetryAfter(headers)

	msg := fmt.Sprintf("HTTP %d: %s", statusCode, errType.String())
	if len(body) > 0 && len(body) < 200 {
		msg = fmt.Sprintf("HTTP %d (%s): %s", statusCode, errType.String(), string(body))
	}

	return &types.TransportError{
		Type:       errType,
		StatusCode: statusCode,
		Message:    msg,
		RetryAfter: retryAfter,
		Err:        baseErr,
	}
}

// HTTPTransport implements HTTP-based communication using umh-core protocol.
//
// HTTPTransport publishes httpClient via atomic.Pointer so Push, Pull, and Authenticate
// can read it concurrently while Reset replaces it without a mutex. Readers Load() a
// local *http.Client and complete the in-flight call against that captured pointer;
// subsequent calls observe the pointer from the most recent Store call.
// CloseIdleConnections on the old client's transport acquires http.Transport's internal
// idleMu, which is independent of any synchronization on HTTPTransport itself, so there
// is no deadlock between Reset's cleanup and concurrent Do() calls. Reset publishes the
// new client before closing idle connections on the old one.
//
// HTTPTransport must be created via NewHTTPTransport; a zero-value HTTPTransport is invalid.
type HTTPTransport struct {
	// Field order set by betteralign-fix.
	httpClient atomic.Pointer[http.Client]
	RelayURL   string
}

// NewHTTPTransport creates a new HTTP transport.
// If timeout is 0, defaults to LongPollingDuration (30 seconds).
// Transport settings match legacy communicator to avoid Cloudflare issues.
// Connection pooling replaces the legacy DisableKeepAlives setting.
func NewHTTPTransport(relayURL string, timeout time.Duration) *HTTPTransport {
	if timeout == 0 {
		timeout = LongPollingDuration
	}

	t := &HTTPTransport{
		RelayURL: relayURL,
	}
	t.httpClient.Store(buildHTTPClient(timeout))

	return t
}

// buildHTTPClient constructs the *http.Client used by NewHTTPTransport and
// Reset. Both call paths produce a client with identical settings, so this
// function centralizes the configuration.
func buildHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
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
	}
}

// Authenticate performs JWT authentication using double-hashed token in Authorization header.
func (t *HTTPTransport) Authenticate(ctx context.Context, req types.AuthRequest) (types.AuthResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.RelayURL+"/v2/instance/login", nil)
	if err != nil {
		return types.AuthResponse{}, &types.TransportError{
			Type:    types.ErrorTypeNetwork,
			Message: fmt.Sprintf("failed to create auth request: %v", err),
			Err:     err,
		}
	}

	// Double-hash token: Hash(Hash(AuthToken)) matches legacy communicator behavior
	hashedToken := hash.Sha3Hash(hash.Sha3Hash(req.Email))
	httpReq.Header.Set("Authorization", "Bearer "+hashedToken)
	httpReq.Header.Set("Content-Type", "application/json")

	client := t.httpClient.Load()
	resp, err := client.Do(httpReq)
	if err != nil {
		return types.AuthResponse{}, &types.TransportError{
			Type:    types.ErrorTypeNetwork,
			Message: fmt.Sprintf("auth request failed: %v", err),
			Err:     err,
		}
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, readErr := io.ReadAll(resp.Body)

		return types.AuthResponse{}, newTransportError(resp.StatusCode, bodyBytes, resp.Header, readErr)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.AuthResponse{}, &types.TransportError{
			Type:    types.ErrorTypeNetwork,
			Message: fmt.Sprintf("failed to read auth response body: %v", err),
			Err:     err,
		}
	}

	var loginResp struct {
		UUID string `json:"uuid"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(bodyBytes, &loginResp); err != nil {
		bodyPreview := string(bodyBytes)
		if len(bodyPreview) > 256 {
			bodyPreview = bodyPreview[:256] + "..."
		}

		return types.AuthResponse{}, &types.TransportError{
			Type:    types.ErrorTypeUnknown,
			Message: fmt.Sprintf("failed to decode auth response: %v (body preview: %s)", err, bodyPreview),
			Err:     err,
		}
	}

	var jwtToken string

	for _, cookie := range resp.Cookies() {
		if cookie.Name == "token" {
			jwtToken = cookie.Value

			break
		}
	}

	if jwtToken == "" {
		return types.AuthResponse{}, &types.TransportError{
			Type:    types.ErrorTypeInvalidToken,
			Message: "no token cookie returned from login",
		}
	}

	// Backend doesn't return expiresAt; estimate 23h (refresh before typical 24h JWT expiry)
	defaultExpiry := time.Now().Add(23 * time.Hour).Unix()

	return types.AuthResponse{
		Token:        jwtToken,
		ExpiresAt:    defaultExpiry,
		InstanceUUID: loginResp.UUID,
		InstanceName: loginResp.Name,
	}, nil
}

// Pull retrieves messages from the backend (GET /v2/instance/pull).
func (t *HTTPTransport) Pull(ctx context.Context, jwtToken string) ([]*types.UMHMessage, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, t.RelayURL+"/v2/instance/pull", nil)
	if err != nil {
		return nil, &types.TransportError{
			Type:    types.ErrorTypeNetwork,
			Message: fmt.Sprintf("failed to create pull request: %v", err),
			Err:     err,
		}
	}

	httpReq.AddCookie(&http.Cookie{Name: "token", Value: jwtToken})

	client := t.httpClient.Load()
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, &types.TransportError{
			Type:    types.ErrorTypeNetwork,
			Message: fmt.Sprintf("pull request failed: %v", err),
			Err:     err,
		}
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, readErr := io.ReadAll(resp.Body)

		return nil, newTransportError(resp.StatusCode, bodyBytes, resp.Header, readErr)
	}

	var payload types.PullPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, &types.TransportError{
			Type:    types.ErrorTypeUnknown,
			Message: fmt.Sprintf("failed to decode pull response: %v", err),
			Err:     err,
		}
	}

	return payload.UMHMessages, nil
}

// Push sends messages to the backend (POST /v2/instance/push).
func (t *HTTPTransport) Push(ctx context.Context, jwtToken string, messages []*types.UMHMessage) error {
	if len(messages) == 0 {
		return nil
	}

	payload := types.PushPayload{
		UMHMessages: messages,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return &types.TransportError{
			Type:    types.ErrorTypeUnknown,
			Message: fmt.Sprintf("failed to marshal push payload: %v", err),
			Err:     err,
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.RelayURL+"/v2/instance/push", bytes.NewBuffer(body))
	if err != nil {
		return &types.TransportError{
			Type:    types.ErrorTypeNetwork,
			Message: fmt.Sprintf("failed to create push request: %v", err),
			Err:     err,
		}
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.AddCookie(&http.Cookie{Name: "token", Value: jwtToken})

	client := t.httpClient.Load()
	resp, err := client.Do(httpReq)
	if err != nil {
		return &types.TransportError{
			Type:    types.ErrorTypeNetwork,
			Message: fmt.Sprintf("push request failed: %v", err),
			Err:     err,
		}
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		bodyBytes, readErr := io.ReadAll(resp.Body)

		return newTransportError(resp.StatusCode, bodyBytes, resp.Header, readErr)
	}

	return nil
}

// ResetClient closes idle connections on the current HTTP client without
// replacing it. Safe to call concurrently with Push, Pull, and Authenticate.
// http.Transport.CloseIdleConnections acquires the transport's internal
// idleMu, which is independent of any synchronization on HTTPTransport.
func (t *HTTPTransport) ResetClient() {
	if c := t.httpClient.Load(); c != nil {
		// If Transport is not *http.Transport, idle-connection draining is skipped.
		if tr, ok := c.Transport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
	}
}

// Close drains idle connections but does not prevent new requests; callers must
// cancel the context to stop in-flight and future calls.
func (t *HTTPTransport) Close() {
	t.ResetClient()
}

// Reset recreates the HTTP client to establish fresh connections when retries
// aren't helping. The new client is published atomically; concurrent Push,
// Pull, and Authenticate either observe the old client (and finish their
// in-flight call against it) or the new one. Reset is non-blocking. The
// atomic Store does not wait for in-flight reads.
// Reset must be called from a single goroutine; the FSMv2 reconciler is the only intended caller.
func (t *HTTPTransport) Reset() {
	old := t.httpClient.Load()

	timeout := LongPollingDuration
	if old != nil {
		timeout = old.Timeout
	}

	t.httpClient.Store(buildHTTPClient(timeout))

	if old != nil {
		// If Transport is not *http.Transport, idle-connection draining is skipped.
		if tr, ok := old.Transport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
	}
}
