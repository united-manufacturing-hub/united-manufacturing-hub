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
	"errors"
	"io"
	"net"
	"net/http"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/error_handler"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/safejson"
	"go.uber.org/zap"
)

// processCookies handles cookie updates from response headers.
func processCookies(response *http.Response, cookies *map[string]string) {
	if cookies == nil {
		return
	}

	cookieMap := *cookies
	// Process response cookies
	for _, cookie := range response.Cookies() {
		cookieMap[cookie.Name] = cookie.Value
	}

	*cookies = cookieMap
}

// processJSONResponse processes the HTTP response and unmarshals the JSON body.
func processJSONResponse[R any](response *http.Response, cookies *map[string]string, endpoint Endpoint, method string, logger *zap.SugaredLogger) (*R, int, error) {
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
				string(endpoint),
				method,
				nil,
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
		LatestExternalIp = ip
	}

	return &typedResult, response.StatusCode, nil
}
