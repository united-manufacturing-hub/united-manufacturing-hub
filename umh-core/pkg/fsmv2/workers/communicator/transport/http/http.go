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
	"strconv"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/hash"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

// ErrorType classifies HTTP errors for intelligent backoff strategies.
type ErrorType int

const (
	// ErrorTypeUnknown represents an unclassified error.
	ErrorTypeUnknown ErrorType = iota
	// ErrorTypeCloudflareChallenge represents Cloudflare challenge page (429 + HTML "Just a moment").
	ErrorTypeCloudflareChallenge
	// ErrorTypeBackendRateLimit represents backend rate limiting (429 + JSON + Retry-After).
	ErrorTypeBackendRateLimit
	// ErrorTypeInvalidToken represents authentication failure (401/403).
	ErrorTypeInvalidToken
	// ErrorTypeInstanceDeleted represents instance not found (404).
	ErrorTypeInstanceDeleted
	// ErrorTypeServerError represents server-side errors (5xx).
	ErrorTypeServerError
	// ErrorTypeProxyBlock represents proxy block pages (Zscaler, BlueCoat, etc.).
	ErrorTypeProxyBlock
	// ErrorTypeNetwork represents network/connection errors.
	ErrorTypeNetwork
)

// String returns a human-readable name for the error type.
func (e ErrorType) String() string {
	switch e {
	case ErrorTypeCloudflareChallenge:
		return "cloudflare_challenge"
	case ErrorTypeBackendRateLimit:
		return "backend_rate_limit"
	case ErrorTypeInvalidToken:
		return "invalid_token"
	case ErrorTypeInstanceDeleted:
		return "instance_deleted"
	case ErrorTypeServerError:
		return "server_error"
	case ErrorTypeProxyBlock:
		return "proxy_block"
	case ErrorTypeNetwork:
		return "network"
	default:
		return "unknown"
	}
}

// TransportError represents a classified HTTP transport error.
// It embeds error type information for intelligent backoff strategies.
type TransportError struct {
	Err        error
	Message    string
	Type       ErrorType
	StatusCode int
	RetryAfter time.Duration
}

// Error implements the error interface.
func (e *TransportError) Error() string {
	return e.Message
}

// Unwrap returns the underlying error.
func (e *TransportError) Unwrap() error {
	return e.Err
}

// Is implements errors.Is() for type-based error comparison (Go 1.13+ idiom).
func (e *TransportError) Is(target error) bool {
	t, ok := target.(*TransportError)
	if !ok {
		return false
	}

	return e.Type == t.Type
}

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
// Why these strings?
//   - Cloudflare's challenge pages are generated server-side and contain
//     consistent HTML markers. These strings have been stable across
//     Cloudflare versions observed in production.
//   - "Just a moment" appears in all challenge page variants
//   - "challenge-form" and "cf-browser-verification" are specific element IDs
//     that only appear in Cloudflare-generated pages
//
// Note: Some Cloudflare configurations may not include the Server header,
// so we also check body content alone as a fallback.
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
func classifyError(statusCode int, body []byte, headers http.Header) ErrorType {
	// Cloudflare challenge (must check before generic 429)
	if isCloudflareChallenge(statusCode, headers, body) {
		return ErrorTypeCloudflareChallenge
	}
	// Proxy block (Zscaler, etc.)
	if isProxyBlock(body) {
		return ErrorTypeProxyBlock
	}
	// Backend rate limit
	if statusCode == http.StatusTooManyRequests {
		return ErrorTypeBackendRateLimit
	}
	// Auth errors
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		return ErrorTypeInvalidToken
	}
	// Instance deleted
	if statusCode == http.StatusNotFound {
		return ErrorTypeInstanceDeleted
	}
	// Server errors
	if statusCode >= 500 {
		return ErrorTypeServerError
	}

	return ErrorTypeUnknown
}

// newTransportError creates a classified transport error from an HTTP response.
func newTransportError(statusCode int, body []byte, headers http.Header, baseErr error) *TransportError {
	errType := classifyError(statusCode, body, headers)
	retryAfter := parseRetryAfter(headers)

	msg := fmt.Sprintf("HTTP %d: %s", statusCode, errType.String())
	if len(body) > 0 && len(body) < 200 {
		msg = fmt.Sprintf("HTTP %d (%s): %s", statusCode, errType.String(), string(body))
	}

	return &TransportError{
		Type:       errType,
		StatusCode: statusCode,
		Message:    msg,
		RetryAfter: retryAfter,
		Err:        baseErr,
	}
}

// HTTPTransport implements HTTP-based communication using umh-core protocol.
type HTTPTransport struct {
	httpClient *http.Client
	RelayURL   string
}

// NewHTTPTransport creates a new HTTP transport.
// If timeout is 0, defaults to 30 seconds.
// Transport settings match legacy communicator to avoid Cloudflare issues.
func NewHTTPTransport(relayURL string, timeout time.Duration) *HTTPTransport {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// Connection pooling replaces DisableKeepAlives for better performance (Bug #7 fix)
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

// Authenticate performs JWT authentication using double-hashed token in Authorization header.
func (t *HTTPTransport) Authenticate(ctx context.Context, req transport.AuthRequest) (transport.AuthResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.RelayURL+"/v2/instance/login", nil)
	if err != nil {
		return transport.AuthResponse{}, &TransportError{
			Type:    ErrorTypeNetwork,
			Message: fmt.Sprintf("failed to create auth request: %v", err),
			Err:     err,
		}
	}

	// Double-hash token: Hash(Hash(AuthToken)) matches legacy communicator behavior
	hashedToken := hash.Sha3Hash(hash.Sha3Hash(req.Email))
	httpReq.Header.Set("Authorization", "Bearer "+hashedToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return transport.AuthResponse{}, &TransportError{
			Type:    ErrorTypeNetwork,
			Message: fmt.Sprintf("auth request failed: %v", err),
			Err:     err,
		}
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)

		return transport.AuthResponse{}, newTransportError(resp.StatusCode, bodyBytes, resp.Header, nil)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return transport.AuthResponse{}, &TransportError{
			Type:    ErrorTypeNetwork,
			Message: fmt.Sprintf("failed to read auth response body: %v", err),
			Err:     err,
		}
	}

	var loginResp struct {
		UUID string `json:"uuid"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(bodyBytes, &loginResp); err != nil {
		return transport.AuthResponse{}, fmt.Errorf("failed to decode auth response: %w", err)
	}

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

	// Backend doesn't return expiresAt; estimate 23h (refresh before typical 24h JWT expiry)
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
		return nil, &TransportError{
			Type:    ErrorTypeNetwork,
			Message: fmt.Sprintf("failed to create pull request: %v", err),
			Err:     err,
		}
	}

	httpReq.AddCookie(&http.Cookie{Name: "token", Value: jwtToken})

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return nil, &TransportError{
			Type:    ErrorTypeNetwork,
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
		bodyBytes, _ := io.ReadAll(resp.Body)

		return nil, newTransportError(resp.StatusCode, bodyBytes, resp.Header, nil)
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
		return &TransportError{
			Type:    ErrorTypeNetwork,
			Message: fmt.Sprintf("failed to create push request: %v", err),
			Err:     err,
		}
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.AddCookie(&http.Cookie{Name: "token", Value: jwtToken})

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return &TransportError{
			Type:    ErrorTypeNetwork,
			Message: fmt.Sprintf("push request failed: %v", err),
			Err:     err,
		}
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)

		return newTransportError(resp.StatusCode, bodyBytes, resp.Header, nil)
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

// Reset recreates the HTTP client to establish fresh connections when retries aren't helping.
func (t *HTTPTransport) Reset() {
	// Close existing connections
	t.ResetClient()

	t.httpClient = &http.Client{
		Timeout: t.httpClient.Timeout,
		Transport: &http.Transport{
			ForceAttemptHTTP2: false,
			TLSNextProto:      make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
			Proxy:             http.ProxyFromEnvironment,

			MaxIdleConns:        5,
			MaxIdleConnsPerHost: 1,
			MaxConnsPerHost:     2,

			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,

			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}
