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

// R6 RED: the secure-rejection half of the TLS matrix is re-homed onto the
// FSMv2 transport client. The legacy communicator requester (deleted in R6)
// carried this test; it must now run against HTTPTransport. A self-signed /
// untrusted certificate MUST be rejected by default and MUST be accepted only
// when the insecure override is requested.
package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	httptransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/http"
)

var _ = Describe("TLS secure rejection on the FSMv2 transport client", func() {
	// httptest.NewTLSServer presents a self-signed certificate that no system
	// trust store recognizes - a stand-in for the badssl self-signed /
	// untrusted-root sites the legacy requester tested against, without the
	// network dependency.
	var server *httptest.Server

	BeforeEach(func() {
		server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"UMHMessages": []any{}})
		}))
	})

	AfterEach(func() {
		server.Close()
	})

	It("rejects the self-signed certificate by default (secure mode)", func() {
		transport := httptransport.NewHTTPTransport(server.URL, 30*time.Second, false)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := transport.Pull(ctx, "jwt-token")

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("certificate"))
	})

	It("accepts the self-signed certificate only with the insecure override", func() {
		transport := httptransport.NewHTTPTransport(server.URL, 30*time.Second, true)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		messages, err := transport.Pull(ctx, "jwt-token")

		Expect(err).ToNot(HaveOccurred())
		Expect(messages).To(BeEmpty())
	})
})
