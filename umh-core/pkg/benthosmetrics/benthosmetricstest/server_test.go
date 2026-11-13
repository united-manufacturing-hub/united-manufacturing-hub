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

package benthosmetricstest_test

import (
	"fmt"
	"io"
	"net/http"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/benthosmetrics/benthosmetricstest"
)

func TestBenthosmetricstest(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Benthosmetricstest Suite")
}

// get requests path from s and returns the status and the body.
func get(s *benthosmetricstest.Server, path string) (int, string) {
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d%s", s.Port(), path))
	if err != nil || resp == nil {
		Fail(fmt.Sprintf("GET %s: %v", path, err))

		return 0, ""
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())

	return resp.StatusCode, string(body)
}

var _ = Describe("Server", func() {
	var s *benthosmetricstest.Server

	BeforeEach(func() {
		s = benthosmetricstest.NewServer()
		DeferCleanup(s.Close)
	})

	It("adds the traffic and calls OnMetricsRead once per /metrics read", func() {
		reads := 0

		s.OnMetricsRead(func() { reads++ })
		s.SetTraffic(10, 4, 1)

		get(s, "/ping")
		get(s, "/metrics")
		_, body := get(s, "/metrics")

		Expect(reads).To(Equal(2), "only /metrics reads count")

		input, output, errors := s.Counters()
		Expect([]int64{input, output, errors}).To(Equal([]int64{20, 8, 2}))
		Expect(body).To(ContainSubstring(`input_received{label="",path="root.input"} 20`))
		Expect(body).To(ContainSubstring(`output_sent{label="",path="root.output.broker.1"} 4`))
	})

	It("answers a failing path with its status and a valid body, and adds no traffic", func() {
		reads := 0

		s.OnMetricsRead(func() { reads++ })
		s.SetTraffic(10, 0, 0)
		s.SetFailing("/metrics", http.StatusInternalServerError)
		s.SetFailing("/ping", http.StatusServiceUnavailable)

		status, body := get(s, "/ping")
		Expect(status).To(Equal(http.StatusServiceUnavailable))
		Expect(body).To(Equal("pong"), "only the status tells the failure")

		status, _ = get(s, "/metrics")
		Expect(status).To(Equal(http.StatusInternalServerError))
		Expect(reads).To(BeZero(), "a failing read is not a scrape Benthos answered")

		input, _, _ := s.Counters()
		Expect(input).To(BeZero())

		s.SetFailing("/metrics", 0)
		status, _ = get(s, "/metrics")
		Expect(status).To(Equal(http.StatusOK), "status 0 serves the path again")
	})

	It("lets SetHandler answer first and records every request", func() {
		s.SetHandler(func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path != "/version" {
				return false
			}

			_, _ = w.Write([]byte("<html>not benthos</html>"))

			return true
		})

		_, body := get(s, "/version")
		Expect(body).To(Equal("<html>not benthos</html>"))
		_, body = get(s, "/ready")
		Expect(body).To(Equal(`{"error":"","statuses":[]}`))
		Expect(s.Requests()).To(Equal([]string{"/version", "/ready"}))
	})

	It("can be closed twice", func() {
		s.Close()
		Expect(s.Close).NotTo(Panic())
	})
})
