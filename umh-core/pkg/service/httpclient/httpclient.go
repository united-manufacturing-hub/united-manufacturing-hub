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

package httpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/ctxutil"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"go.uber.org/zap"
)

// HTTPClient interface for making HTTP requests
type HTTPClient interface {
	// Do executes an HTTP request and returns the response
	Do(req *http.Request) (*http.Response, error)

	// GetWithBody performs a GET request and returns the response with body bytes
	// This is a convenience method that combines request creation, execution,
	// and body reading in a single call
	GetWithBody(ctx context.Context, url string) (*http.Response, []byte, error)
}

// DefaultHTTPClient is the default implementation of HTTPClient
type DefaultHTTPClient struct {
	logger *zap.SugaredLogger
}

// NewDefaultHTTPClient creates a new DefaultHTTPClient
func NewDefaultHTTPClient() *DefaultHTTPClient {
	return &DefaultHTTPClient{
		logger: logger.For("HTTPClient"),
	}
}

// Do performs the HTTP request, creating a context-optimized client
func (c *DefaultHTTPClient) Do(req *http.Request) (*http.Response, error) {
	// Extract context from request
	ctx := req.Context()

	// Get client configured for this specific context
	client, err := c.createClientFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Execute the request
	return client.Do(req)
}

// createClientFromContext creates an HTTP client with timeouts based on context deadline
func (c *DefaultHTTPClient) createClientFromContext(ctx context.Context) (*http.Client, error) {
	// Verify context has deadline and sufficient time
	remaining, _, err := ctxutil.HasSufficientTime(ctx, time.Millisecond) // Just need a minimal required time
	if err != nil {
		if errors.Is(err, ctxutil.ErrNoDeadline) {
			return nil, fmt.Errorf("no deadline set in context")
		}
		// For other errors, we still want to create a client with whatever time remains
		c.logger.Warnf("Creating HTTP client with limited time: %v", err)
	}

	// Use the available time for timeouts
	timeout := remaining

	// Create transport with timeouts scaled from context deadline
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: timeout / 2, // Use half the available time for connection
		}).DialContext,
		ResponseHeaderTimeout: timeout / 2, // And half for response
		ExpectContinueTimeout: timeout / 4,
		IdleConnTimeout:       timeout / 4,
		TLSHandshakeTimeout:   timeout / 4,
	}

	// Create a new client for this specific request
	return &http.Client{
		Transport: transport,
	}, nil
}

// GetWithBody performs a GET request and returns the response with body
func (c *DefaultHTTPClient) GetWithBody(ctx context.Context, url string) (*http.Response, []byte, error) {
	// Create request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request for %s: %w", url, err)
	}

	// Execute request
	resp, err := c.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to execute request for %s: %w", url, err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, fmt.Errorf("failed to read response body for %s: %w", url, err)
	}

	return resp, body, nil
}
