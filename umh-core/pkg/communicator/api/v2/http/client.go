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

// GetClient returns a configured HTTP client with TLS and HTTP/2 settings.
// When insecureTLS is true, TLS certificate verification is disabled and minimum TLS version is set to 1.0 for compatibility.
// HTTP/2 is explicitly disabled to avoid issues with some middleware and legacy systems.
// The client includes a 30-second timeout and uses system proxy settings.
// You probably don't want to use this, but use PostRequest or GetRequest instead.
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
				// MinVersion: TLS 1.0, since middle boxes sometime struggle with newer TLS versions at our customers.
				MinVersion: tls.VersionTLS10,
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
