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

// Package types defines the transport protocol contract: the Transport interface,
// message types, and error classification types used by the transport worker and
// its consumers.
package types

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	depspkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// AuthSession groups the auth fields that travel together from parent transport status
// to push/pull child config: the JWT bearer token, its expiry, and the backend-confirmed
// instance UUID. Atomic token+expiry update is provided by the SetJWT setter on
// TransportDependencies, not enforced by the struct. Children read the session from
// their own snapshot instead of reaching into parent dependencies.
//
// Defined here (a leaf package imported by snapshot, push, and pull) to avoid an
// import cycle: RenderChildren (in the snapshot package) stamps it and push/pull
// parse it, but snapshot cannot import push/pull.
type AuthSession struct {
	Expiry       time.Time `json:"expiry,omitempty"       yaml:"expiry,omitempty"`
	Token        string    `json:"token,omitempty"        yaml:"token,omitempty"`
	InstanceUUID string    `json:"instanceUUID,omitempty" yaml:"instanceUUID,omitempty"`
}

// IsUsable reports whether the session has a token that will still be valid after
// the given safety buffer. Pass 1*time.Minute for child-worker COS checks or
// 10*time.Minute for the parent's proactive refresh trigger.
func (a AuthSession) IsUsable(buffer time.Duration) bool {
	if a.Token == "" || a.Expiry.IsZero() {
		return false
	}

	return !time.Now().Add(buffer).After(a.Expiry)
}

// ChildAuthUserSpec is the YAML carrier for the auth session that transport's
// RenderChildren stamps onto its push/pull children. push/pull embed it in their
// UserSpec so the marshal-then-parse round trip is structurally identical on both sides.
type ChildAuthUserSpec struct {
	AuthSession AuthSession `json:"authSession,omitempty" yaml:"authSession,omitempty"`
}

// UMHMessage represents a message in the umh-core push/pull protocol.
// Note: InstanceUUID uses json tag "umhInstance" to match backend API (models.UMHMessage).
type UMHMessage struct {
	InstanceUUID string `json:"umhInstance"`
	Content      string `json:"content"`
	Email        string `json:"email"`
	TraceID      string `json:"traceId,omitempty"`
}

// AuthRequest represents an authentication request.
type AuthRequest struct {
	InstanceUUID string `json:"instanceUUID"`
	Email        string `json:"email"`
}

// AuthResponse represents an authentication response.
//
// InstanceUUID and InstanceName are returned by the backend on successful
// authentication and may be empty for clients that do not advertise them.
type AuthResponse struct {
	Token        string `json:"token"`
	InstanceUUID string `json:"uuid,omitempty"`
	InstanceName string `json:"name,omitempty"`
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

// ErrorType classifies transport-layer HTTP errors into categories that
// determine how the system responds. Each type maps to a Prometheus counter
// (via [CounterForErrorType]) and a transient/persistent classification
// (via [IsTransient]) that controls whether the error propagates to the FSM
// or is suppressed at the action layer (metrics are still recorded).
//
// ErrorTypeChannelFull shares the network counter pending a dedicated
// counter for inbound-channel backpressure.
type ErrorType int

const (
	// ErrorTypeUnknown represents an unclassified error.
	ErrorTypeUnknown ErrorType = iota
	// ErrorTypeCloudflareChallenge represents Cloudflare challenge page (429 + HTML "Just a moment").
	// Persistent: requires network path change or Cloudflare allowlisting.
	ErrorTypeCloudflareChallenge
	// ErrorTypeBackendRateLimit represents backend rate limiting (429 + JSON + Retry-After).
	// Transient: self-resolves after the rate limit window expires.
	ErrorTypeBackendRateLimit
	// ErrorTypeInvalidToken represents authentication failure (401/403).
	// Persistent: requires re-authentication by the parent worker.
	ErrorTypeInvalidToken
	// ErrorTypeInstanceDeleted represents instance not found (404).
	// Persistent: requires instance re-registration or human intervention.
	ErrorTypeInstanceDeleted
	// ErrorTypeServerError represents server-side errors (5xx).
	// Transient: backend recovers on its own.
	ErrorTypeServerError
	// ErrorTypeProxyBlock represents proxy block pages (Zscaler, BlueCoat, etc.).
	// Persistent: requires proxy configuration change.
	ErrorTypeProxyBlock
	// ErrorTypeNetwork represents network/connection errors (DNS, TCP, TLS).
	// Transient: network path recovers on its own.
	ErrorTypeNetwork
	// ErrorTypeChannelFull represents inbound channel capacity exceeded.
	// Transient: resolves as the consumer drains the channel.
	// Classified here for backoff purposes; not a network-layer transport error.
	ErrorTypeChannelFull

	// ErrorTypeMax is a helper value marking the end of the ErrorType iota range.
	// It is used in tests
	// DO NOT ADD NEW CONSTANTS AFTER THIS.
	ErrorTypeMax
)

// IsTransientTypes maps every known ErrorType to its transient classification.
// Adding a new ErrorType constant without a corresponding entry here causes the
// exhaustiveness test in is_transient_test.go to fail at test time.
//
// Transient: Network, ServerError, ChannelFull, BackendRateLimit.
// Persistent: everything else (InvalidToken, InstanceDeleted, ProxyBlock,
// CloudflareChallenge, Unknown).
var IsTransientTypes = map[ErrorType]bool{
	ErrorTypeUnknown:             false,
	ErrorTypeCloudflareChallenge: false,
	ErrorTypeBackendRateLimit:    true,
	ErrorTypeInvalidToken:        false,
	ErrorTypeInstanceDeleted:     false,
	ErrorTypeServerError:         true,
	ErrorTypeProxyBlock:          false,
	ErrorTypeNetwork:             true,
	ErrorTypeChannelFull:         true,
}

// IsTransient reports whether the error type represents a condition that
// typically self-resolves without human intervention.
//
// The classification controls error propagation in push and pull actions:
//   - Transient errors are suppressed (action returns nil). Metrics and
//     DegradedState are still updated. The failurerate.Tracker monitors
//     the rolling failure rate; if transient errors dominate the window,
//     it fires a one-shot SentryWarn escalation.
//   - Persistent errors propagate to the FSM as errors, triggering state
//     transitions (recovering, re-authentication) and firing SentryError.
func (e ErrorType) IsTransient() bool {
	return IsTransientTypes[e]
}

// ErrorTypeNames maps every known ErrorType to its human-readable name.
// Adding a new ErrorType constant without a corresponding entry here causes the
// exhaustiveness test in is_transient_test.go to fail at test time.
var ErrorTypeNames = map[ErrorType]string{
	ErrorTypeUnknown:             "unknown",
	ErrorTypeCloudflareChallenge: "cloudflare_challenge",
	ErrorTypeBackendRateLimit:    "backend_rate_limit",
	ErrorTypeInvalidToken:        "invalid_token",
	ErrorTypeInstanceDeleted:     "instance_deleted",
	ErrorTypeServerError:         "server_error",
	ErrorTypeProxyBlock:          "proxy_block",
	ErrorTypeNetwork:             "network",
	ErrorTypeChannelFull:         "channel_full",
}

// String returns a human-readable name for the error type.
func (e ErrorType) String() string {
	if s, ok := ErrorTypeNames[e]; ok {
		return s
	}

	return "unknown"
}

// TransportError represents a classified HTTP transport error.
// It embeds error type information for error-type-based backoff.
type TransportError struct {
	Err        error
	Message    string
	Type       ErrorType
	StatusCode int
	RetryAfter time.Duration
}

// Error implements the error interface. If Message is empty but a wrapped
// error is present, it falls back to the wrapped error's message so callers
// always see a non-empty string.
func (e *TransportError) Error() string {
	if e.Message == "" && e.Err != nil {
		return e.Err.Error()
	}

	return e.Message
}

// Unwrap returns the underlying error.
func (e *TransportError) Unwrap() error {
	return e.Err
}

// Is implements errors.Is() for type-based error comparison (Go 1.13+ idiom).
func (e *TransportError) Is(target error) bool {
	if e == nil {
		return false
	}

	t, ok := target.(*TransportError)
	if !ok {
		return false
	}

	return e.Type == t.Type
}

// ExtractErrorType unwraps a *TransportError from err and returns its ErrorType
// and RetryAfter duration. If err does not wrap a *TransportError, it returns
// ErrorTypeUnknown, a persistent type that propagates to the FSM and fires
// SentryError, ensuring unclassified errors are never silently suppressed.
//
// It is a thin shim over ExtractErrorDetails so the nil and typed-nil-wrapped
// handling lives in one place.
func ExtractErrorType(err error) (ErrorType, time.Duration) {
	t, ra, _, _ := ExtractErrorDetails(err)

	return t, ra
}

// ExtractErrorDetails unwraps a *TransportError from err and returns its
// ErrorType, RetryAfter duration, HTTP status code, and a sanitized detail
// string derived from the error's message. If err does not wrap a
// *TransportError, it returns ErrorTypeUnknown, zero RetryAfter, a zero status
// code, and a sanitized version of err's message. A nil err yields zero values
// and an empty detail string.
//
// The detail string is sanitized for control characters only; it does NOT
// redact credentials. Callers must not assume secrets are scrubbed from the
// output (ENG-5289 tracks adding redaction).
func ExtractErrorDetails(err error) (ErrorType, time.Duration, int, string) {
	if err == nil {
		return ErrorTypeUnknown, 0, 0, ""
	}

	var te *TransportError
	if errors.As(err, &te) {
		if te == nil {
			return ErrorTypeUnknown, 0, 0, ""
		}

		return te.Type, te.RetryAfter, te.StatusCode, sanitizeErrorDetail(te.Error())
	}

	return ErrorTypeUnknown, 0, 0, sanitizeErrorDetail(err.Error())
}

// ErrorTypeCounters maps every known ErrorType to its Prometheus counter.
// Adding a new ErrorType constant without a corresponding entry here causes the
// exhaustiveness test in is_transient_test.go to fail at test time.
//
// ErrorTypeUnknown and ErrorTypeChannelFull share CounterNetworkErrorsTotal:
// Unknown because it is unclassified, ChannelFull because it is internal
// backpressure pending a dedicated counter.
var ErrorTypeCounters = map[ErrorType]depspkg.CounterName{
	ErrorTypeUnknown:             depspkg.CounterNetworkErrorsTotal,
	ErrorTypeCloudflareChallenge: depspkg.CounterCloudflareErrorsTotal,
	ErrorTypeBackendRateLimit:    depspkg.CounterBackendRateLimitErrorsTotal,
	ErrorTypeInvalidToken:        depspkg.CounterAuthFailuresTotal,
	ErrorTypeInstanceDeleted:     depspkg.CounterInstanceDeletedTotal,
	ErrorTypeServerError:         depspkg.CounterServerErrorsTotal,
	ErrorTypeProxyBlock:          depspkg.CounterProxyBlockErrorsTotal,
	ErrorTypeNetwork:             depspkg.CounterNetworkErrorsTotal,
	ErrorTypeChannelFull:         depspkg.CounterNetworkErrorsTotal,
}

// CounterForErrorType maps an ErrorType to its corresponding Prometheus counter.
// Unknown and unrecognized types default to CounterNetworkErrorsTotal.
func CounterForErrorType(t ErrorType) depspkg.CounterName {
	if c, ok := ErrorTypeCounters[t]; ok {
		return c
	}

	return depspkg.CounterNetworkErrorsTotal
}

// sanitizeErrorDetail strips control characters to spaces, collapses runs of
// whitespace to a single space, trims surrounding whitespace, and caps the
// result to 256 bytes on a UTF-8 rune boundary so the output is always
// valid UTF-8.
//
// It does not redact credentials. Callers must not assume secrets are
// scrubbed from the output (ENG-5289 tracks adding redaction).
func sanitizeErrorDetail(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}

		return r
	}, s)

	s = strings.Join(strings.Fields(s), " ")

	if len(s) <= 256 {
		return s
	}

	end := 256

	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}

	return strings.TrimSpace(s[:end])
}
