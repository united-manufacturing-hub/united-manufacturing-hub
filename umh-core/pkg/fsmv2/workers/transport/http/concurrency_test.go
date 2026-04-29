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

// This spec fails under "go test -race" because Push reads t.httpClient
// and Reset writes t.httpClient with no synchronization. Without -race
// the data race is not visible and the spec passes.
// The race is resolved when httpClient is switched to atomic.Pointer[http.Client].

package transport_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"

	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

var _ = Describe("HTTPTransport concurrency", func() {
	It("does not race when Push and Reset run concurrently (RED until atomic.Pointer[http.Client] fix)", func() {
		// The server always returns 200 OK. The test only checks that the race
		// detector reports a data race, not that individual requests succeed.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		transport := httpTransport.NewHTTPTransport(server.URL, 5*time.Second)
		defer transport.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// 10,000 iterations is enough to make the race detector trip reliably; reduce on slow CI if needed.
		const iterations = 10000
		// msgs is read-only and shared across both goroutines.
		msgs := []*types.UMHMessage{{InstanceUUID: "race-test", Email: "race@example.com", Content: "{}"}}

		var wg sync.WaitGroup
		wg.Add(2)

		// Goroutine A calls Push repeatedly; Push reads t.httpClient on every call.
		go func() {
			defer wg.Done()
			for range iterations {
				_ = transport.Push(ctx, "race-token", msgs)
			}
		}()

		// Goroutine B calls Reset repeatedly; Reset writes t.httpClient with no synchronization.
		go func() {
			defer wg.Done()
			for range iterations {
				transport.Reset()
			}
		}()

		wg.Wait()
	})
})
