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
	pushCalls         int
	pushedBytes       int64
	nextError         int
	slowDelay         time.Duration
	// pathFaults holds faults that persist until cleared, keyed by request path.
	// SimulateServerError and SimulateSlowResponse are one-shot and shared by every
	// path, so neither can hold one endpoint slow while another stays healthy.
	pathFaults map[string]PathFault
	// faultedRequests counts requests per path, for StallEveryNth.
	faultedRequests map[string]int
	// closing is closed by Close, releasing any request parked by a Hang fault.
	// httptest.Server.Close waits for outstanding handlers, so a Hang that only
	// watched the request context would deadlock teardown.
	closing   chan struct{}
	closeOnce sync.Once
	mu        sync.Mutex
}

// PathFault describes what one endpoint does to every request until the fault is
// cleared. A zero PathFault is no fault.
type PathFault struct {
	// Delay is how long to hold the request before answering.
	Delay time.Duration
	// StatusCode, when non-zero, is returned instead of handling the request.
	// It is applied after Delay.
	StatusCode int
	// Hang answers nothing and holds the connection until the fault is cleared or
	// the client gives up. Delay and StatusCode are ignored when it is set.
	Hang bool
	// BytesPerSecond models a bandwidth limit rather than a latency: the request
	// is held for ContentLength/BytesPerSecond before being handled, so a bigger
	// body costs more time. Delay, which is fixed, cannot express that -- under a
	// fixed delay message size has no effect on anything, so an experiment that
	// varies size while holding Delay constant can only ever report "size does
	// not matter". Added with Delay, both apply.
	BytesPerSecond int
	// StallEveryNth is the period of the stall pattern: within each run of
	// StallEveryNth requests to this path, the first StallBurst of them hang for
	// StallFor and the rest are untouched. It models what a uniform Delay or
	// BytesPerSecond cannot: a link whose median request is fast and whose tail
	// is not.
	//
	// StallBurst exists because an isolated slow request leaves no trace in the
	// agent. A transient failure is absorbed without an action_failed, and a
	// child only degrades after three CONSECUTIVE errors, so scattered stalls
	// produce no observable event at all however many there are. Burst length is
	// therefore a separate dial from duty cycle, not a detail of it.
	//
	// It counts rather than randomises, so a run is reproducible. 0 disables it.
	StallEveryNth int
	// StallBurst is how many consecutive requests stall at the start of each
	// period. Defaults to 1 when StallEveryNth is set and this is not.
	StallBurst int
	// StallFor is how long a stalled request is held. Ignored unless
	// StallEveryNth is set.
	StallFor time.Duration
}

// NewMockRelayServer creates and starts a new mock relay server.
func NewMockRelayServer() *MockRelayServer {
	m := &MockRelayServer{
		pullQueue:         make([]*types.UMHMessage, 0),
		pushedMsgs:        make([]*types.UMHMessage, 0),
		connectionHeaders: make([]string, 0),
		jwtToken:          "mock-jwt-token-" + time.Now().Format("20060102150405"),
		// Bug #6 fix: Default backend UUID - different from any placeholder UUID
		backendUUID:     "backend-real-uuid-12345678",
		backendName:     "Mock Instance Name",
		pathFaults:      make(map[string]PathFault),
		faultedRequests: make(map[string]int),
		closing:         make(chan struct{}),
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

	if r.URL.Path == "/v2/instance/push" {
		m.pushCalls++
	}
	m.mu.Unlock()

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

	if fault, ok := m.pathFault(r.URL.Path); ok {
		if fault.Hang {
			// Hold until the client's own timeout fires or the server closes.
			select {
			case <-r.Context().Done():
			case <-m.closing:
			}

			return
		}

		wait := fault.Delay

		if fault.StallEveryNth > 0 && fault.StallFor > 0 {
			m.mu.Lock()
			m.faultedRequests[r.URL.Path]++
			nth := m.faultedRequests[r.URL.Path]
			m.mu.Unlock()

			burst := fault.StallBurst
			if burst <= 0 {
				burst = 1
			}

			if nth%fault.StallEveryNth < burst {
				wait += fault.StallFor
			}
		}

		if fault.BytesPerSecond > 0 && r.ContentLength > 0 {
			wait += time.Duration(float64(r.ContentLength) / float64(fault.BytesPerSecond) * float64(time.Second))
		}

		if wait > 0 {
			select {
			case <-time.After(wait):
			case <-r.Context().Done():
				return
			case <-m.closing:
				return
			}
		}

		if fault.StatusCode != 0 {
			w.WriteHeader(fault.StatusCode)

			return
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

	for _, msg := range payload.UMHMessages {
		if msg != nil {
			m.pushedBytes += int64(len(msg.Content))
		}
	}
	m.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

// URL returns the server's URL.
func (m *MockRelayServer) URL() string {
	return m.server.URL
}

// Close shuts down the mock server.
func (m *MockRelayServer) Close() {
	// Release parked Hang requests first. server.Close waits for outstanding
	// handlers, so closing in the other order deadlocks.
	m.closeOnce.Do(func() { close(m.closing) })

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

// SetPathFault makes every request to path behave as fault describes, until
// ClearPathFault or another SetPathFault replaces it. Login is not exempt, so a
// fault on /v2/instance/login also breaks authentication.
func (m *MockRelayServer) SetPathFault(path string, fault PathFault) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.pathFaults[path] = fault
}

// ClearPathFault removes the fault on path, if any.
func (m *MockRelayServer) ClearPathFault(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.pathFaults, path)
}

// pathFault reports the fault registered for path.
func (m *MockRelayServer) pathFault(path string) (PathFault, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	fault, ok := m.pathFaults[path]

	return fault, ok
}

// PushCallCount returns how many push requests reached the server, including any
// the faults above turned away. GetPushedMessages only counts requests that were
// handled, so the two differ whenever a fault is set.
func (m *MockRelayServer) PushCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.pushCalls
}

// PushedBytes returns the total Content bytes the server accepted on the push
// endpoint. Compared against what a producer offered, it shows whether the
// constraint under test actually bit.
func (m *MockRelayServer) PushedBytes() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.pushedBytes
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
