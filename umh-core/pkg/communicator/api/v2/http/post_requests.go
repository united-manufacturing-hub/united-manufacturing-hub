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
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/http/internal"
	"go.uber.org/zap"
)

// PostRequest performs an HTTP POST request to the specified endpoint with automatic retry logic.
// The generic type parameter R specifies the expected response type for JSON deserialization.
// The request body is JSON-encoded from the provided data parameter.
// Returns the deserialized response, HTTP status code, and any error that occurred.
//
// Type Parameters:
//   - R: The expected response type that the JSON response will be unmarshaled into
//
// Parameters:
//   - ctx: Context for request cancellation and timeout (defaults to 30s if nil)
//   - endpoint: API endpoint path to request
//   - data: Request body data to be JSON-encoded (can be any serializable type)
//   - header: Optional HTTP headers to include
//   - cookies: Optional HTTP cookies (will be updated with response cookies if not nil)
//   - insecureTLS: Whether to disable TLS certificate verification
//   - apiURL: Base API URL to prepend to endpoint
//   - logger: Logger instance for request logging
//
// Returns:
//   - result: Pointer to deserialized response data of type R (nil if error or empty response)
//   - statusCode: HTTP status code from the response
//   - responseErr: Any error that occurred during the request or response processing
func PostRequest[R any](ctx context.Context, endpoint Endpoint, data any, header map[string]string, cookies *map[string]string, insecureTLS bool, apiURL string, logger *zap.SugaredLogger) (result *R, statusCode int, responseErr error) {
	// Set up context with default 30 second timeout if none provided
	if ctx == nil {
		var cancel context.CancelFunc

		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
	}

	requestURL, err := url.JoinPath(apiURL, string(endpoint))
	if err != nil {
		return nil, 0, err
	}

	response, err := internal.DoHTTPRequestWithRetry(ctx, http.MethodPost, requestURL, data, header, cookies, insecureTLS, false, logger, GetClient(insecureTLS))
	if err != nil {
		if response != nil {
			return nil, response.StatusCode, err
		}

		return nil, 0, err
	}

	result, statusCode, responseErr = internal.ProcessJSONResponse[R](response, cookies, string(endpoint), http.MethodPost, logger)

	return
}
