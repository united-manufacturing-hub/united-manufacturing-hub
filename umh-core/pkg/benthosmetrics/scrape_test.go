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

// What one scrape returns: the HTTP calls it makes, the rates it computes,
// and IsActive across a counter reset.

package benthosmetrics

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/benthosmetrics/benthosmetricstest"
)

// newServer starts a fake Benthos whose /metrics serves input and output.
func newServer(input, output int64) *benthosmetricstest.Server {
	s := benthosmetricstest.NewServer()
	s.SetCounters(input, output, 0)

	return s
}

var _ = Describe("Scrape", func() {
	It("fetches all endpoints", func() {
		s := newServer(10, 5)
		defer s.Close()

		t0 := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return t0 })

		probes, sample, err := tr.Scrape(context.Background(), int(s.Port()))
		Expect(err).NotTo(HaveOccurred())

		Expect(s.Requests()).To(ConsistOf("/ping", "/ready", "/version", "/metrics"))
		Expect(probes.PingAlive).To(BeTrue())
		Expect(probes.Ready).To(BeTrue())
		Expect(probes.Version).To(Equal(benthosmetricstest.Version))
		Expect(probes.At).To(Equal(t0), "Probes.At is when the scrape began")
		Expect(sample.At).To(Equal(t0), "Sample.At is when /metrics was read")
		Expect(sample.Metrics.Inputs).To(HaveKey("root.input"))
		Expect(sample.Metrics.Inputs["root.input"].Received).To(Equal(int64(10)))
		Expect(sample.Metrics.Outputs["root.output.broker.0"].Sent).To(Equal(int64(2)))
		Expect(sample.Metrics.Outputs["root.output.broker.1"].Sent).To(Equal(int64(3)))
	})

	It("rejects a 500 /version whose body is valid JSON", func() {
		s := newServer(10, 5)
		defer s.Close()

		s.SetFailing("/version", http.StatusInternalServerError)

		tr := NewTracker(time.Now)
		probes, sample, err := tr.Scrape(context.Background(), int(s.Port()))
		Expect(err).To(HaveOccurred(),
			"a Benthos answering 500 is not healthy; the body decodes, so only the status check fails it")
		Expect(strings.Count(err.Error(), "/version")).To(Equal(1),
			"the error names the URL once")

		By("keeping the probe answers and returning no measurement")
		Expect(probes.PingAlive).To(BeTrue())
		Expect(probes.Ready).To(BeTrue())
		Expect(probes.Version).To(BeEmpty())
		Expect(sample).To(Equal(Sample{}))
	})

	It("clears only PingAlive when /ping answers 500", func() {
		s := newServer(10, 5)
		defer s.Close()

		s.SetFailing("/ping", http.StatusInternalServerError)

		tr := NewTracker(time.Now)
		probes, _, err := tr.Scrape(context.Background(), int(s.Port()))
		Expect(err).NotTo(HaveOccurred())
		Expect(probes.PingAlive).To(BeFalse(),
			"the /ping body answers pong, so only the status check clears PingAlive")
		Expect(probes.Ready).To(BeTrue())
	})

	It("computes the throughput", func() {
		s := newServer(10, 5)
		defer s.Close()

		now := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return now })

		_, sample, err := tr.Scrape(context.Background(), int(s.Port()))
		Expect(err).NotTo(HaveOccurred())
		Expect(sample.InputMessagesPerSecond).To(Equal(unmeasured),
			"the first scrape holds one sample")
		Expect(sample.IsActive).To(BeFalse())

		s.SetCounters(12, 6, 0)

		now = now.Add(time.Second)
		_, sample, err = tr.Scrape(context.Background(), int(s.Port()))
		Expect(err).NotTo(HaveOccurred())
		Expect(perSecond(sample.InputMessagesPerSecond)).To(BeNumerically("~", 2, 0.5),
			"10 -> 12 over 1s, so the scrape feeds the window")
		Expect(sample.IsActive).To(BeTrue())
	})

	It("reads a counter reset as the post-restart rate", func() {
		s := newServer(100, 100)
		defer s.Close()

		now := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return now })

		_, _, err := tr.Scrape(context.Background(), int(s.Port()))
		Expect(err).NotTo(HaveOccurred())

		// A restart drops both counters. The drop is folded into the
		// offsets, so the series the engine sees never goes down and the
		// tick reads the messages the restart itself processed, never a
		// negative rate.
		s.SetCounters(5, 5, 0)

		now = now.Add(time.Second)
		_, sample, err := tr.Scrape(context.Background(), int(s.Port()))
		Expect(err).NotTo(HaveOccurred())
		Expect(sample.IsActive).To(BeTrue())
		Expect(perSecond(sample.InputMessagesPerSecond)).To(BeNumerically("~", 5, 0.5),
			"the reset tick reads 100 -> 5 as 5 messages in 1s, not as -95/s")
		Expect(sample.Metrics.Inputs["root.input"].Received).To(Equal(int64(5)),
			"Metrics still reports the post-restart value")
	})

	It("stamps the counters when /metrics is read, not when the scrape began", func() {
		// The counter grows 1 per virtual ms; the first /ping costs 350
		// virtual ms. The rate must be 1000/s, so the counters must be
		// stamped when /metrics is read.
		var mu sync.Mutex

		vt := time.Unix(1_000_000, 0)
		slowPing := true
		clock := func() time.Time {
			mu.Lock()
			defer mu.Unlock()

			return vt
		}

		s := newServer(0, 0)
		defer s.Close()

		s.SetHandler(func(w http.ResponseWriter, r *http.Request) bool {
			mu.Lock()
			defer mu.Unlock()

			switch r.URL.Path {
			case "/ping":
				if slowPing {
					vt = vt.Add(350 * time.Millisecond)
					slowPing = false
				}

				_, _ = w.Write([]byte("pong"))
			case "/metrics":
				n := vt.Sub(time.Unix(1_000_000, 0)).Milliseconds()
				_, _ = fmt.Fprintf(w, "input_received{path=\"root.input\"} %d\n", n)
			default:
				return false // /ready and /version: the default handler answers
			}

			return true
		})

		tr := NewTracker(clock)
		_, _, err := tr.Scrape(context.Background(), int(s.Port()))
		Expect(err).NotTo(HaveOccurred())
		mu.Lock()
		vt = vt.Add(100 * time.Millisecond)
		mu.Unlock()

		_, sample, err := tr.Scrape(context.Background(), int(s.Port()))
		Expect(err).NotTo(HaveOccurred())
		Expect(perSecond(sample.InputMessagesPerSecond)).To(BeNumerically("~", 1000, 10),
			"the three probes before /metrics take a varying time")
	})

	It("counts the instance live only when /ping answers pong", func() {
		s := newServer(1, 1)
		defer s.Close()

		s.SetHandler(func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path == "/ping" {
				_, _ = w.Write([]byte("<html>not benthos</html>"))

				return true
			}

			return false
		})

		probes, _, err := NewTracker(time.Now).Scrape(context.Background(), int(s.Port()))
		Expect(err).NotTo(HaveOccurred())
		Expect(probes.PingAlive).To(BeFalse(),
			"/ping answered, but not with pong")
	})

	It("reads Ready false on a 503 /ready whose body is valid JSON", func() {
		s := newServer(1, 1)
		defer s.Close()

		s.SetFailing("/ready", http.StatusServiceUnavailable)

		probes, _, err := NewTracker(time.Now).Scrape(context.Background(), int(s.Port()))
		Expect(err).NotTo(HaveOccurred())
		Expect(probes.Ready).To(BeFalse(),
			"the body reports no error, so only the status check clears Ready")
	})

	It("reads Ready false when /ready names an error", func() {
		s := newServer(1, 1)
		defer s.Close()

		s.SetHandler(func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path == "/ready" {
				_, _ = w.Write([]byte(`{"error":"output not connected"}`))

				return true
			}

			return false
		})

		probes, _, err := NewTracker(time.Now).Scrape(context.Background(), int(s.Port()))
		Expect(err).NotTo(HaveOccurred())
		Expect(probes.Ready).To(BeFalse())
	})

	It("treats a new port as a new counter series", func() {
		a := newServer(100, 100)
		defer a.Close()

		b := newServer(150, 150)
		defer b.Close()

		now := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return now })

		_, _, err := tr.Scrape(context.Background(), int(a.Port()))
		Expect(err).NotTo(HaveOccurred())

		now = now.Add(time.Second)
		_, sample, err := tr.Scrape(context.Background(), int(b.Port()))
		Expect(err).NotTo(HaveOccurred())
		Expect(sample.InputMessagesPerSecond).To(Equal(unmeasured),
			"a new port is a new counter series, so the window rebuilds")
	})

	It("rejects a 3xx /metrics", func() {
		s := newServer(1, 1)
		defer s.Close()

		s.SetFailing("/metrics", http.StatusFound)

		_, _, err := NewTracker(time.Now).Scrape(context.Background(), int(s.Port()))
		Expect(err).To(HaveOccurred())
	})

	It("does not follow a redirect away from /metrics", func() {
		// The redirect target answers with a valid metrics body, so this
		// spec fails only when the client follows the Location header.
		s := newServer(1, 1)
		defer s.Close()

		s.SetHandler(func(w http.ResponseWriter, r *http.Request) bool {
			switch r.URL.Path {
			case "/metrics":
				http.Redirect(w, r, "/elsewhere", http.StatusFound)

				return true
			case "/elsewhere":
				_, _ = fmt.Fprint(w, "input_received{label=\"\",path=\"root.input\"} 1\n")

				return true
			}

			return false
		})

		_, _, err := NewTracker(time.Now).Scrape(context.Background(), int(s.Port()))
		Expect(err).To(HaveOccurred(),
			"a 3xx answer is an error, not a hop: the scrape never leaves this Benthos' endpoints")
	})

	It("fails the scrape when /metrics does not parse", func() {
		s := newServer(1, 1)
		defer s.Close()

		s.SetHandler(func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path == "/metrics" {
				_, _ = w.Write([]byte("input_received{label=\"\",path=\"root.input\"}\n"))

				return true
			}

			return false
		})

		_, _, err := NewTracker(time.Now).Scrape(context.Background(), int(s.Port()))
		Expect(err).To(MatchError(ContainSubstring("parse /metrics")))
	})

	It("fails the scrape when /version is not JSON", func() {
		s := newServer(1, 1)
		defer s.Close()

		s.SetHandler(func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path == "/version" {
				_, _ = w.Write([]byte("<html>not benthos</html>"))

				return true
			}

			return false
		})

		_, _, err := NewTracker(time.Now).Scrape(context.Background(), int(s.Port()))
		Expect(err).To(MatchError(ContainSubstring("decode /version")))
	})
})
