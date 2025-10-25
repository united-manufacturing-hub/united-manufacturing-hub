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
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/protocol"
)

type HTTPTransport struct {
	relayURL   string
	jwtToken   string
	httpClient *http.Client
	mu         sync.RWMutex
	closed     bool
	cancelFn   context.CancelFunc
}

func NewHTTPTransport(relayURL, jwtToken string) *HTTPTransport {
	return &HTTPTransport{
		relayURL: relayURL,
		jwtToken: jwtToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		closed: false,
	}
}

func (h *HTTPTransport) Send(ctx context.Context, to string, payload []byte) error {
	h.mu.RLock()
	if h.closed {
		h.mu.RUnlock()
		return errors.New("transport closed")
	}
	h.mu.RUnlock()

	msg := map[string]interface{}{
		"to":      to,
		"payload": string(payload),
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return protocol.TransportConfigError{Err: fmt.Errorf("failed to marshal message: %w", err)}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", h.relayURL+"/send", bytes.NewBuffer(body))
	if err != nil {
		return protocol.TransportConfigError{Err: fmt.Errorf("failed to create request: %w", err)}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.jwtToken)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return protocol.TransportTimeoutError{Err: err}
		}
		return protocol.TransportNetworkError{Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return protocol.TransportAuthError{Err: fmt.Errorf("authentication failed: %d - %s", resp.StatusCode, string(bodyBytes))}
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return protocol.TransportNetworkError{Err: fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(bodyBytes))}
	}

	return nil
}

func (h *HTTPTransport) Receive(ctx context.Context) (<-chan protocol.RawMessage, error) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil, errors.New("transport closed")
	}

	msgChan := make(chan protocol.RawMessage, 100)

	pollCtx, cancelFn := context.WithCancel(ctx)
	h.cancelFn = cancelFn
	h.mu.Unlock()

	go h.pollLoop(pollCtx, msgChan)

	return msgChan, nil
}

func (h *HTTPTransport) pollLoop(ctx context.Context, msgChan chan<- protocol.RawMessage) {
	defer close(msgChan)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			msg, err := h.poll(ctx)
			if err != nil {
				continue
			}

			if msg != nil {
				select {
				case msgChan <- *msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

func (h *HTTPTransport) poll(ctx context.Context) (*protocol.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", h.relayURL+"/receive", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create poll request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+h.jwtToken)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("poll request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, protocol.TransportAuthError{Err: errors.New("authentication failed")}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var msg protocol.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&msg); err != nil {
		return nil, fmt.Errorf("failed to decode message: %w", err)
	}

	return &msg, nil
}

func (h *HTTPTransport) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return nil
	}

	h.closed = true

	if h.cancelFn != nil {
		h.cancelFn()
	}

	return nil
}
