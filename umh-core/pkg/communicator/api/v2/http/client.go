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
	"crypto/tls"
	"net/http"
	"sync"
	"time"
)

var secureHTTPClient *http.Client
var insecureHTTPClient *http.Client
var initHTTPClientOnce sync.Once

func GetClient(insecureTLS bool) *http.Client {
	// Prevent init race
	initHTTPClientOnce.Do(func() {
		// Create a custom transport with HTTP/2 disabled
		secureTransport := &http.Transport{
			ForceAttemptHTTP2: false,
			TLSNextProto:      make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
			Proxy:             http.ProxyFromEnvironment,
		}

		secureHTTPClient = &http.Client{
			Transport: secureTransport,
			Timeout:   30 * time.Second,
		}

		// Create a custom transport with HTTP/2 disabled and insecure TLS
		insecureTransport := &http.Transport{
			ForceAttemptHTTP2: false,
			TLSNextProto:      make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
			Proxy:             http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionTLS10, // Allow older TLS versions
			},
		}

		// Create an HTTP client with the custom transport
		insecureHTTPClient = &http.Client{
			Transport: insecureTransport,
			Timeout:   30 * time.Second,
		}
	})

	if insecureTLS {
		return insecureHTTPClient
	}

	return secureHTTPClient
}
