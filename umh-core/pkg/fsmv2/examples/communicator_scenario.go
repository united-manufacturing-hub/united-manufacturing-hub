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

package examples

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

// CommunicatorScenario tests the FSMv2 communicator with mock server.
//
// # Purpose
//
// This scenario provides end-to-end testing of the communicator worker,
// verifying the complete lifecycle:
//   - Authentication with relay server
//   - Message pulling (backend -> edge)
//   - Message pushing (edge -> backend)
//   - Error handling and degraded state
//   - Re-authentication on token expiry
//
// # Usage
//
// Create a scenario with a mock server URL:
//
//	mockServer := testutil.NewMockRelayServer()
//	scenario := examples.NewCommunicatorScenario(mockServer.URL(), "auth-token")
//	err := scenario.Run(ctx, 5*time.Second)
//
// # State Tracking
//
// The scenario tracks:
//   - receivedMessages: Messages pulled from backend
//   - outboundQueue: Messages to be pushed to backend
//   - consecutiveErrors: Error count for health monitoring
type CommunicatorScenario struct {
	apiURL    string
	authToken string
	logger    *zap.SugaredLogger

	// State tracking
	mu                sync.Mutex
	receivedMessages  []*transport.UMHMessage
	outboundQueue     []*transport.UMHMessage
	errorThreshold    int
	consecutiveErrors int

	// HTTP client for requests
	httpClient *http.Client
	jwtToken   string
}

// NewCommunicatorScenario creates a new scenario with mock server URL.
//
// Parameters:
//   - mockServerURL: URL of the mock relay server
//   - authToken: Authentication token for the relay server
//
// Returns a scenario ready to be run.
func NewCommunicatorScenario(mockServerURL, authToken string) *CommunicatorScenario {
	logger, _ := zap.NewDevelopment()

	return &CommunicatorScenario{
		apiURL:           mockServerURL,
		authToken:        authToken,
		logger:           logger.Sugar(),
		receivedMessages: make([]*transport.UMHMessage, 0),
		outboundQueue:    make([]*transport.UMHMessage, 0),
		errorThreshold:   3, // Default threshold
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Run executes the scenario for the specified duration.
//
// This method:
//  1. Authenticates with the mock server
//  2. Performs sync operations (push/pull)
//  3. Tracks messages and errors
//
// Parameters:
//   - ctx: Context for cancellation
//   - duration: How long to run the scenario
//
// Returns error if critical operations fail.
func (s *CommunicatorScenario) Run(ctx context.Context, duration time.Duration) error {
	// Create a context with timeout
	runCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	// Authenticate first
	if err := s.authenticate(runCtx); err != nil {
		s.mu.Lock()
		s.consecutiveErrors++
		s.mu.Unlock()

		return err
	}

	// Run sync loop
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-runCtx.Done():
			return nil
		case <-ticker.C:
			// Pull messages
			if err := s.pullMessages(runCtx); err != nil {
				s.mu.Lock()
				s.consecutiveErrors++
				shouldReauth := s.consecutiveErrors >= s.errorThreshold
				s.mu.Unlock()

				// Check if we need to re-authenticate (after threshold errors)
				// Note: authenticate() needs its own lock, so we release ours first
				if shouldReauth {
					// Try re-authentication
					if authErr := s.authenticate(runCtx); authErr == nil {
						s.mu.Lock()
						s.consecutiveErrors = 0
						s.mu.Unlock()
					}
				}

				continue
			}

			// Push outbound messages
			if err := s.pushMessages(runCtx); err != nil {
				s.mu.Lock()
				s.consecutiveErrors++
				s.mu.Unlock()

				continue
			}
		}
	}
}

// authenticate performs login with the mock relay server.
func (s *CommunicatorScenario) authenticate(ctx context.Context) error {
	authReq := transport.AuthRequest{
		InstanceUUID: "test-instance-uuid",
		Email:        s.authToken,
	}

	body, err := json.Marshal(authReq)
	if err != nil {
		return fmt.Errorf("failed to marshal auth request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL+"/v2/instance/login", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create auth request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("auth request failed: %w", err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("auth failed with status %d", resp.StatusCode)
	}

	var authResp transport.AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return fmt.Errorf("failed to decode auth response: %w", err)
	}

	s.mu.Lock()
	s.jwtToken = authResp.Token
	s.mu.Unlock()

	return nil
}

// pullMessages fetches messages from the backend.
func (s *CommunicatorScenario) pullMessages(ctx context.Context) error {
	s.mu.Lock()
	token := s.jwtToken
	s.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.apiURL+"/v2/instance/pull", nil)
	if err != nil {
		return fmt.Errorf("failed to create pull request: %w", err)
	}

	req.AddCookie(&http.Cookie{Name: "token", Value: token})

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("pull request failed: %w", err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("authentication failed: %d", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pull failed with status %d", resp.StatusCode)
	}

	// Use uppercase "UMHMessages" to match mock server response
	var payload struct {
		UMHMessages []*transport.UMHMessage `json:"UMHMessages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("failed to decode pull response: %w", err)
	}

	// Store received messages
	if len(payload.UMHMessages) > 0 {
		s.mu.Lock()
		s.receivedMessages = append(s.receivedMessages, payload.UMHMessages...)
		s.consecutiveErrors = 0 // Reset on success
		s.mu.Unlock()
	} else {
		// Even empty response is success
		s.mu.Lock()
		s.consecutiveErrors = 0
		s.mu.Unlock()
	}

	return nil
}

// pushMessages sends queued messages to the backend.
func (s *CommunicatorScenario) pushMessages(ctx context.Context) error {
	s.mu.Lock()
	token := s.jwtToken
	outbound := make([]*transport.UMHMessage, len(s.outboundQueue))
	copy(outbound, s.outboundQueue)
	s.outboundQueue = s.outboundQueue[:0] // Clear queue
	s.mu.Unlock()

	if len(outbound) == 0 {
		return nil
	}

	// Use uppercase "UMHMessages" to match mock server expected format
	payload := struct {
		UMHMessages []*transport.UMHMessage `json:"UMHMessages"`
	}{
		UMHMessages: outbound,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		// Put messages back in queue on failure
		s.mu.Lock()
		s.outboundQueue = append(s.outboundQueue, outbound...)
		s.mu.Unlock()

		return fmt.Errorf("failed to marshal push payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL+"/v2/instance/push", bytes.NewBuffer(body))
	if err != nil {
		s.mu.Lock()
		s.outboundQueue = append(s.outboundQueue, outbound...)
		s.mu.Unlock()

		return fmt.Errorf("failed to create push request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "token", Value: token})

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.mu.Lock()
		s.outboundQueue = append(s.outboundQueue, outbound...)
		s.mu.Unlock()

		return fmt.Errorf("push request failed: %w", err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		s.mu.Lock()
		s.outboundQueue = append(s.outboundQueue, outbound...)
		s.mu.Unlock()

		return fmt.Errorf("push failed with status %d", resp.StatusCode)
	}

	s.mu.Lock()
	s.consecutiveErrors = 0 // Reset on success
	s.mu.Unlock()

	return nil
}

// GetReceivedMessages returns messages pulled from the backend.
//
// Returns a copy of the received messages slice to avoid race conditions.
func (s *CommunicatorScenario) GetReceivedMessages() []*transport.UMHMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]*transport.UMHMessage, len(s.receivedMessages))
	copy(result, s.receivedMessages)

	return result
}

// QueueOutboundMessage queues a message to be pushed to the backend.
//
// Messages are pushed during the next sync cycle.
func (s *CommunicatorScenario) QueueOutboundMessage(msg *transport.UMHMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.outboundQueue = append(s.outboundQueue, msg)
}

// GetConsecutiveErrors returns the current consecutive error count.
//
// This is used by tests to verify error tracking behavior.
func (s *CommunicatorScenario) GetConsecutiveErrors() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.consecutiveErrors
}

// SetErrorThreshold sets the error threshold for degraded state.
//
// After this many consecutive errors, the scenario attempts re-authentication.
func (s *CommunicatorScenario) SetErrorThreshold(threshold int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.errorThreshold = threshold
}
