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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// MockRelayServer is a mock HTTP server that simulates the relay server for testing.
// It supports authentication, pull, and push operations with error injection capabilities.
type MockRelayServer struct {
	server   *httptest.Server
	jwtToken string
	// Bug #6 fix: Backend returns a specific UUID for the instance
	backendUUID       string
	backendName       string
	pullQueue         []*types.UMHMessage
	pushedMsgs        []*types.UMHMessage
	connectionHeaders []string
	authCalls         int
	nextError         int
	slowDelay         time.Duration
	// bandwidthBytesPerSecond stays in effect until changed, where nextError and
	// slowDelay above clear themselves after one request. Set by
	// SimulateBandwidthLimitation, which describes what it does.
	bandwidthBytesPerSecond int
	mu                      sync.Mutex
	// closing is closed once, at the start of Close, to release any request
	// parked in the bandwidth hold below. Without it, Close waits out the
	// full hold of every in-flight request, since nothing else cancels their
	// contexts.
	closing     chan struct{}
	closingOnce sync.Once
}

// NewMockRelayServer creates and starts a new mock relay server.
func NewMockRelayServer() *MockRelayServer {
	m := &MockRelayServer{
		pullQueue:         make([]*types.UMHMessage, 0),
		pushedMsgs:        make([]*types.UMHMessage, 0),
		connectionHeaders: make([]string, 0),
		jwtToken:          "mock-jwt-token-" + time.Now().Format("20060102150405"),
		// Bug #6 fix: Default backend UUID - different from any placeholder UUID
		backendUUID: "backend-real-uuid-12345678",
		backendName: "Mock Instance Name",
		closing:     make(chan struct{}),
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

	// Hold requests whose body has a declared length, to simulate a bandwidth-limited
	// uplink rather than a slow server: a bigger body waits longer. This has no path
	// exemption, unlike the error/slow-response faults below. It does not need one:
	// keying the hold on body size already confines its effect to push, since push is
	// the only endpoint whose request declares a length. login has no body and pull is
	// a GET with a nil body, so both report zero bytes, and zero bytes at any rate is
	// no delay.
	m.mu.Lock()
	bytesPerSecond := m.bandwidthBytesPerSecond
	m.mu.Unlock()

	if bytesPerSecond > 0 && r.ContentLength > 0 {
		holdDuration := time.Duration(r.ContentLength) * time.Second / time.Duration(bytesPerSecond)

		select {
		case <-time.After(holdDuration):
		case <-r.Context().Done():
			return
		case <-m.closing:
			return
		}
	}

	// Check for injected errors (except for login endpoint)
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
		var req types.AuthRequest

		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	m.mu.Lock()
	m.authCalls++
	token := m.jwtToken
	backendUUID := m.backendUUID
	backendName := m.backendName
	m.mu.Unlock()

	// Set JWT cookie (matching real backend behavior)
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
	})

	// Return uuid and name in response body (matches real backend behavior)
	resp := struct {
		UUID string `json:"uuid"`
		Name string `json:"name"`
	}{
		UUID: backendUUID,
		Name: backendName,
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
	messages := m.pullQueue
	m.pullQueue = make([]*types.UMHMessage, 0) // Clear queue after pull
	m.mu.Unlock()

	// Use uppercase "UMHMessages" to match real backend
	payload := struct {
		UMHMessages []*types.UMHMessage `json:"UMHMessages"`
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
		UMHMessages []*types.UMHMessage `json:"UMHMessages"`
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

// Close shuts down the mock server. Safe to call more than once: it releases
// any request parked in the bandwidth hold before waiting for handlers to
// return, so a closing server never blocks on its own held requests.
func (m *MockRelayServer) Close() {
	m.closingOnce.Do(func() {
		close(m.closing)
	})
	m.server.Close()
}

// QueuePullMessage adds a message to the pull queue.
func (m *MockRelayServer) QueuePullMessage(msg *types.UMHMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.pullQueue = append(m.pullQueue, msg)
}

// GetPushedMessages returns all messages that were pushed to the server.
func (m *MockRelayServer) GetPushedMessages() []*types.UMHMessage {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return a copy to avoid race conditions
	result := make([]*types.UMHMessage, len(m.pushedMsgs))
	copy(result, m.pushedMsgs)

	return result
}

// ClearPushedMessages clears all recorded pushed messages.
func (m *MockRelayServer) ClearPushedMessages() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.pushedMsgs = make([]*types.UMHMessage, 0)
}

// AuthCallCount returns the number of authentication calls made.
func (m *MockRelayServer) AuthCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.authCalls
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

// SimulateBandwidthLimitation models a slow uplink rather than a slow server: every
// request whose body has a declared length is held for ContentLength/maxBytesPerSecond
// before it is answered, so a bigger request takes proportionally longer. Unlike
// SimulateServerError and SimulateSlowResponse, it is not one-time — it stays in effect
// for every request until changed. Pass 0 to disable it.
func (m *MockRelayServer) SimulateBandwidthLimitation(maxBytesPerSecond int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.bandwidthBytesPerSecond = maxBytesPerSecond
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
