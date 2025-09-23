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

package http

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/safejson"
	"go.uber.org/zap"
)

type Endpoint string

var (
	LoginEndpoint Endpoint = "/v2/instance/login"
	PushEndpoint  Endpoint = "/v2/instance/push"
	PullEndpoint  Endpoint = "/v2/instance/pull"
)

// DoHTTPRequestWithRetry performs HTTP requests with automatic retries for connection errors.
// This is the main function that external callers should use for HTTP requests.
func DoHTTPRequestWithRetry[T any](ctx context.Context, method, url string, data *T, header map[string]string, cookies *map[string]string, insecureTLS bool, enableLongPoll bool, logger *zap.SugaredLogger) (*http.Response, error) {
	retryClient := retryablehttp.NewClient()
	retryClient.HTTPClient = GetClient(insecureTLS)
	retryClient.RetryMax = 10
	retryClient.RetryWaitMin = 1 * time.Second
	retryClient.RetryWaitMax = 5 * time.Second

	// Use a logger that wraps zap
	retryClient.Logger = &zapRetryLogger{logger: logger}

	// Custom retry policy that only retries on connection errors, not HTTP status codes
	retryClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		// Don't retry if context was cancelled
		if ctx.Err() != nil {
			return false, ctx.Err()
		}

		// Only retry on connection errors (when err != nil), not on HTTP status errors
		if err != nil {
			// Check if it's a retryable connection error
			errStr := err.Error()
			isRetryable := strings.Contains(errStr, "EOF") ||
				strings.Contains(errStr, "connection reset") ||
				strings.Contains(errStr, "connection refused") ||
				strings.Contains(errStr, "timeout") ||
				strings.Contains(errStr, "deadline exceeded") ||
				strings.Contains(errStr, "no such host") ||
				strings.Contains(errStr, "network is unreachable")

			if isRetryable {
				logger.Debugf("Retrying due to connection error: %v", err)

				return true, nil
			}
		}

		// Don't retry on HTTP status errors (4xx, 5xx)
		return false, nil
	}

	var bodyReader io.Reader

	if method == http.MethodPost && data != nil {
		body, err := safejson.Marshal(data)
		if err != nil {
			return nil, err
		}

		bodyReader = bytes.NewReader(body)
	}

	req, err := retryablehttp.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	// Set method-specific headers
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
		req.ContentLength = -1
	}

	// Set headers
	for k, v := range header {
		req.Header.Set(k, v)
	}

	// Set cookies
	if cookies != nil {
		for k, v := range *cookies {
			req.AddCookie(&http.Cookie{Name: k, Value: v})
		}
	}

	// Set long poll headers conditionally
	if enableLongPoll {
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("Keep-Alive", "timeout=30, max=1000")
		req.Header.Set("X-Features", "longpoll;")
	}

	return retryClient.Do(req)
}

// zapRetryLogger adapts zap.SugaredLogger to retryablehttp.LeveledLogger interface.
type zapRetryLogger struct {
	logger *zap.SugaredLogger
}

func (z *zapRetryLogger) Error(msg string, keysAndValues ...interface{}) {
	z.logger.Errorw(msg, keysAndValues...)
}

func (z *zapRetryLogger) Info(msg string, keysAndValues ...interface{}) {
	z.logger.Infow(msg, keysAndValues...)
}

func (z *zapRetryLogger) Debug(msg string, keysAndValues ...interface{}) {
	z.logger.Debugw(msg, keysAndValues...)
}

func (z *zapRetryLogger) Warn(msg string, keysAndValues ...interface{}) {
	z.logger.Warnw(msg, keysAndValues...)
}
