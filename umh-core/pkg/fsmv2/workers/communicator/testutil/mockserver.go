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

// Package testutil provides testing utilities for the communicator package.
package testutil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

// MockRelayServer is a mock HTTP server that simulates the relay server for testing.
// It supports authentication, pull, and push operations with error injection capabilities.
type MockRelayServer struct {
	server   *httptest.Server
	jwtToken string
	// Bug #6 fix: Backend returns a specific UUID for the instance
	backendUUID       string
	backendName       string
	pullQueue         []*transport.UMHMessage
	pushedMsgs        []*transport.UMHMessage
	connectionHeaders []string
	jwtExpiresAt      int64
	authCalls         int
	pullCalls         int
	nextError         int
	persistentError   int
	slowDelay             time.Duration
	persistentPushDelay   time.Duration
	persistentPushError   int
	mu                    sync.Mutex
}

// NewMockRelayServer creates and starts a new mock relay server.
func NewMockRelayServer() *MockRelayServer {
	m := &MockRelayServer{
		pullQueue:         make([]*transport.UMHMessage, 0),
		pushedMsgs:        make([]*transport.UMHMessage, 0),
		connectionHeaders: make([]string, 0),
		jwtToken:          "mock-jwt-token-" + time.Now().Format("20060102150405"),
		// Bug #6 fix: Default backend UUID - different from any placeholder UUID
		backendUUID: "backend-real-uuid-12345678",
		backendName: "Mock Instance Name",
	}

	m.server = httptest.NewServer(http.HandlerFunc(m.handler))

	return m
}

// SetBackendUUID sets the UUID that will be returned in login responses.
func (m *MockRelayServer) SetBackendUUID(uuid, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.backendUUID = uuid
	m.backendName = name
}

// handler routes requests to the appropriate handler based on the path.
func (m *MockRelayServer) handler(w http.ResponseWriter, r *http.Request) {
	// Track Connection header for Bug #3 validation
	m.mu.Lock()
	m.connectionHeaders = append(m.connectionHeaders, r.Header.Get("Connection"))
	m.mu.Unlock()

	// Check for injected errors (except for login endpoint, unless persistent auth error)
	m.mu.Lock()
	persistentErr := m.persistentError
	m.mu.Unlock()

	if r.URL.Path == "/v2/instance/login" && persistentErr != 0 {
		m.mu.Lock()
		m.authCalls++
		m.mu.Unlock()
		w.WriteHeader(persistentErr)

		return
	}

	if r.URL.Path != "/v2/instance/login" {
		m.mu.Lock()
		errCode := m.nextError
		slowDelay := m.slowDelay

		if errCode != 0 {
			m.nextError = 0 // One-time error
		}

		if slowDelay > 0 {
			m.slowDelay = 0 // One-time slow response
		}

		m.mu.Unlock()

		if errCode != 0 {
			w.WriteHeader(errCode)

			return
		}

		if slowDelay > 0 {
			time.Sleep(slowDelay)
		}
	}

	// Apply persistent push error (separate from one-shot nextError)
	if r.URL.Path == "/v2/instance/push" {
		m.mu.Lock()
		pushErr := m.persistentPushError
		m.mu.Unlock()

		if pushErr != 0 {
			w.WriteHeader(pushErr)

			return
		}
	}

	// Apply persistent push delay (separate from one-shot slowDelay)
	if r.URL.Path == "/v2/instance/push" {
		m.mu.Lock()
		pushDelay := m.persistentPushDelay
		m.mu.Unlock()

		if pushDelay > 0 {
			time.Sleep(pushDelay)
		}
	}

	switch r.URL.Path {
	case "/v2/instance/login":
		m.handleLogin(w, r)
	case "/v2/instance/pull":
		m.handlePull(w, r)
	case "/v2/instance/push":
		m.handlePush(w, r)
	default:
		http.NotFound(w, r)
	}
}

// handleLogin handles authentication requests.
// Matches real backend behavior: Authorization header with Bearer token, returns uuid/name in response.
func (m *MockRelayServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

		return
	}

	// Accept Authorization header or JSON body for backward compatibility with tests
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		// Fallback: try to read from body (legacy test behavior)
		// No auth header and no valid body - that's OK for mock, just continue
		var req transport.AuthRequest

		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	m.mu.Lock()
	m.authCalls++
	token := m.jwtToken
	backendUUID := m.backendUUID
	backendName := m.backendName
	expiresAt := m.jwtExpiresAt
	m.mu.Unlock()

	// Set JWT cookie (matching real backend behavior)
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
	})

	// Return uuid, name, and optional expiresAt in response body
	resp := struct {
		UUID      string `json:"uuid"`
		Name      string `json:"name"`
		ExpiresAt int64  `json:"expiresAt,omitempty"`
	}{
		UUID:      backendUUID,
		Name:      backendName,
		ExpiresAt: expiresAt,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handlePull handles pull requests.
func (m *MockRelayServer) handlePull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

		return
	}

	m.mu.Lock()
	m.pullCalls++
	messages := m.pullQueue
	m.pullQueue = make([]*transport.UMHMessage, 0) // Clear queue after pull
	m.mu.Unlock()

	// Use uppercase "UMHMessages" to match real backend
	payload := struct {
		UMHMessages []*transport.UMHMessage `json:"UMHMessages"`
	}{
		UMHMessages: messages,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

// handlePush handles push requests.
func (m *MockRelayServer) handlePush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

		return
	}

	// Use uppercase "UMHMessages" to match real backend
	var payload struct {
		UMHMessages []*transport.UMHMessage `json:"UMHMessages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)

		return
	}

	m.mu.Lock()
	m.pushedMsgs = append(m.pushedMsgs, payload.UMHMessages...)
	m.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

// URL returns the server's URL.
func (m *MockRelayServer) URL() string {
	return m.server.URL
}

// Close shuts down the mock server.
func (m *MockRelayServer) Close() {
	m.server.Close()
}

// QueuePullMessage adds a message to the pull queue.
func (m *MockRelayServer) QueuePullMessage(msg *transport.UMHMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.pullQueue = append(m.pullQueue, msg)
}

// GetPushedMessages returns all messages that were pushed to the server.
func (m *MockRelayServer) GetPushedMessages() []*transport.UMHMessage {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return a copy to avoid race conditions
	result := make([]*transport.UMHMessage, len(m.pushedMsgs))
	copy(result, m.pushedMsgs)

	return result
}

// ClearPushedMessages clears all recorded pushed messages.
func (m *MockRelayServer) ClearPushedMessages() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.pushedMsgs = make([]*transport.UMHMessage, 0)
}

// AuthCallCount returns the number of authentication calls made.
func (m *MockRelayServer) AuthCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.authCalls
}

// PullCallCount returns the number of pull HTTP requests made to the server.
func (m *MockRelayServer) PullCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.pullCalls
}

// SimulateAuthExpiry sets the next request to return 401 Unauthorized.
func (m *MockRelayServer) SimulateAuthExpiry() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextError = http.StatusUnauthorized
}

// SimulateServerError sets the next request to return the specified HTTP status code.
func (m *MockRelayServer) SimulateServerError(statusCode int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextError = statusCode
}

// SimulateSlowResponse makes the next request delay for the specified duration.
func (m *MockRelayServer) SimulateSlowResponse(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.slowDelay = delay
}

// SetJWTExpiry sets the expiresAt value returned in login responses.
// When non-zero, the expiresAt field is included in the JSON response body,
// allowing tests to control when token expiry triggers.
func (m *MockRelayServer) SetJWTExpiry(expiresAt int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.jwtExpiresAt = expiresAt
}

// SimulatePersistentAuthError makes ALL subsequent login requests return the given status code.
// Unlike SimulateServerError (one-shot), this persists until ClearPersistentAuthError is called.
func (m *MockRelayServer) SimulatePersistentAuthError(statusCode int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.persistentError = statusCode
}

// ClearPersistentAuthError stops rejecting login requests.
func (m *MockRelayServer) ClearPersistentAuthError() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.persistentError = 0
}

// SimulatePersistentSlowPush makes ALL subsequent push requests delay for the specified duration.
// Unlike SimulateSlowResponse (one-shot for any non-login endpoint), this persists until ClearPersistentSlowPush is called.
func (m *MockRelayServer) SimulatePersistentSlowPush(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.persistentPushDelay = delay
}

// ClearPersistentSlowPush stops delaying push requests.
func (m *MockRelayServer) ClearPersistentSlowPush() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.persistentPushDelay = 0
}

// SimulatePersistentPushError makes ALL subsequent push requests return the given status code.
// Pull and login endpoints are unaffected. Persists until ClearPersistentPushError is called.
func (m *MockRelayServer) SimulatePersistentPushError(statusCode int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.persistentPushError = statusCode
}

// ClearPersistentPushError stops rejecting push requests.
func (m *MockRelayServer) ClearPersistentPushError() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.persistentPushError = 0
}

// PushCallCount returns the number of messages pushed to the server.
func (m *MockRelayServer) PushCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.pushedMsgs)
}

// GetReceivedConnectionHeaders returns all Connection headers received from requests.
func (m *MockRelayServer) GetReceivedConnectionHeaders() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return a copy to avoid race conditions
	result := make([]string, len(m.connectionHeaders))
	copy(result, m.connectionHeaders)

	return result
}
