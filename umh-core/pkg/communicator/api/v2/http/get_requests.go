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

	"go.uber.org/zap"
)

// GetRequest does a GET request to the given endpoint, with optional header and cookies.
func GetRequest[R any](ctx context.Context, endpoint Endpoint, header map[string]string, cookies *map[string]string, insecureTLS bool, apiURL string, logger *zap.SugaredLogger) (result *R, statusCode int, responseErr error) {
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

	response, err := doHTTPRequestWithRetry[any](ctx, http.MethodGet, requestURL, nil, header, cookies, insecureTLS, true, logger)
	if err != nil {
		if response != nil {
			return nil, response.StatusCode, err
		}

		return nil, 0, err
	}

	result, statusCode, responseErr = processJSONResponse[R](response, cookies, endpoint, http.MethodGet, logger)

	return
}
