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

package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	httptransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

func TestHTTPTransport(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HTTP Transport Suite")
}

var _ = Describe("HTTP Transport", func() {
	Describe("Bug #7: Connection Pooling Fix", func() {
		It("uses connection pooling with keep-alive", func() {
			// Track Connection headers from requests
			// With connection pooling enabled, sequential requests should NOT send "Connection: close"
			var connectionHeaders []string
			var mu sync.Mutex

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				connectionHeaders = append(connectionHeaders, r.Header.Get("Connection"))
				mu.Unlock()
				_ = json.NewEncoder(w).Encode(map[string]any{"umhMessages": []any{}})
			}))
			defer server.Close()

			// Note: Constructor accepts relayURL and timeout
			transport := httptransport.NewHTTPTransport(server.URL, 30*time.Second)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Make two requests
			_, err := transport.Pull(ctx, "jwt-token-1")
			Expect(err).NotTo(HaveOccurred())

			_, err = transport.Pull(ctx, "jwt-token-2")
			Expect(err).NotTo(HaveOccurred())

			// Assert: With connection pooling enabled, Connection header should NOT be "close"
			// This verifies Bug #7 fix: replaced DisableKeepAlives with proper pooling
			mu.Lock()
			defer mu.Unlock()
			Expect(connectionHeaders).To(HaveLen(2))
			for _, header := range connectionHeaders {
				// Connection header should be empty (keep-alive is default)
				// or "keep-alive" - but NOT "close"
				Expect(header).NotTo(Equal("close"))
			}
		})
	})

	Describe("Bug #4: Context Timeout Fix", func() {
		It("respects context cancellation and returns error", func() {
			// Server that blocks for 5 seconds
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(5 * time.Second)
				_ = json.NewEncoder(w).Encode(map[string]any{"UMHMessages": []any{}})
			}))
			defer server.Close()

			transport := httptransport.NewHTTPTransport(server.URL, 30*time.Second)

			// Context with 100ms timeout - MUCH shorter than server delay
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			// Pull should return quickly with context deadline exceeded
			_, err := transport.Pull(ctx, "jwt-token")

			// Assert: Error occurred due to context timeout
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context deadline exceeded"))
		})

		It("completes within context timeout when server responds quickly", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"UMHMessages": []any{}})
			}))
			defer server.Close()

			transport := httptransport.NewHTTPTransport(server.URL, 30*time.Second)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Should complete quickly
			_, err := transport.Pull(ctx, "jwt-token")
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
