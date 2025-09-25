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

package internal

import (
	"errors"
	"io"
	"net"
	"net/http"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/error_handler"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/safejson"
	"go.uber.org/zap"
)

// processCookies extracts cookies from the HTTP response and updates the provided cookie map.
// This function is used internally to maintain session state across requests.
// If the cookies parameter is nil, no processing is performed.
func processCookies(response *http.Response, cookies *map[string]string) {
	if cookies == nil {
		return
	}

	// Initialize the map if it's nil to prevent panic
	if *cookies == nil {
		*cookies = make(map[string]string)
	}

	cookieMap := *cookies
	// Process response cookies
	for _, cookie := range response.Cookies() {
		cookieMap[cookie.Name] = cookie.Value
	}

	*cookies = cookieMap
}

// processJSONResponse handles HTTP response processing including JSON deserialization and error handling.
// This function reads the response body, checks for HTTP errors, deserializes JSON data, processes cookies,
// and extracts the client's external IP address from response headers.
// Returns the deserialized data, HTTP status code, and any processing error.
// For 401 Unauthorized responses, returns a specific authentication error without reporting to Sentry.
// Other HTTP errors (4xx, 5xx) are reported to the error handler for monitoring.
func ProcessJSONResponse[R any](response *http.Response, cookies *map[string]string, endpoint string, method string, requestBody interface{}, logger *zap.SugaredLogger) (*R, int, error) {
	defer func() {
		if err := response.Body.Close(); err != nil {
			// Log error but don't return it since we're in a defer
			// The main error handling will be done by the caller
			logger.Warnf("Failed to close response body: %s", err)
		}
	}()

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, response.StatusCode, err
	}

	if response.StatusCode < 200 || response.StatusCode > 399 {
		// if the error is 401, we want to report it as a login error
		if response.StatusCode == http.StatusUnauthorized {
			// no need to report it to sentry
			return nil, response.StatusCode, errors.New("unauthorized: Authentication failed. Either your API key is invalid or your instance has been removed. Please verify your API key or recreate the instance")
		} else {
			error_handler.ReportHTTPErrors(
				errors.New("error response code: "+response.Status),
				response.StatusCode,
				endpoint,
				method,
				requestBody,
				bodyBytes,
			)
		}

		return nil, response.StatusCode, errors.New("error response code: " + response.Status)
	}

	if len(bodyBytes) == 0 {
		return nil, response.StatusCode, nil
	}

	var typedResult R
	if err := safejson.Unmarshal(bodyBytes, &typedResult); err != nil {
		return nil, response.StatusCode, err
	}

	processCookies(response, cookies)

	// Process client IP
	if ip := net.ParseIP(response.Header.Get("X-Client-Ip")); ip != nil {
		SetLatestExternalIp(ip)
	}

	return &typedResult, response.StatusCode, nil
}
