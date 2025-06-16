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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
)

var (
	// defaultTransport is a shared transport with connection pooling
	defaultTransport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   50 * time.Millisecond,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   50 * time.Millisecond,
		ExpectContinueTimeout: 100 * time.Millisecond,
		MaxIdleConnsPerHost:   10,   // Increase from default 2
		DisableCompression:    true, // For our metrics endpoint, compression is likely overkill
	}

	// sharedClient is a reusable client for quick local requests
	sharedClient = &http.Client{
		Transport: defaultTransport,
		Timeout:   1 * time.Second,
	}
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

	// Use the shared client for local/quick requests without deadline
	// This is much faster than creating a new client for each request
	_, hasDeadline := ctx.Deadline()
	if !hasDeadline && isLocalRequest(req.URL.Host) {
		return sharedClient.Do(req)
	}

	// Get client configured for this specific context
	client, err := c.createClientFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Execute the request
	return client.Do(req)
}

// isLocalRequest checks if the host is a localhost or loopback address
func isLocalRequest(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "[::1]" || host == ""
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

	// Create a new transport with context-specific timeouts
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   timeout / 2,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       timeout / 4,
		TLSHandshakeTimeout:   timeout / 4,
		ExpectContinueTimeout: timeout / 4,
		ResponseHeaderTimeout: timeout / 2,
		MaxIdleConnsPerHost:   10,
		DisableCompression:    true,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
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
	if resp == nil {
		return nil, nil, fmt.Errorf("received nil response for %s", url)
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			sentry.ReportIssuef(sentry.IssueTypeError, c.logger, "failed to close response body: %v", err)
		}
	}()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, fmt.Errorf("failed to read response body for %s: %w", url, err)
	}

	return resp, body, nil
}
