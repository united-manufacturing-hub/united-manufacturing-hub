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
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// benthosServer answers like a Benthos instance: the four endpoints the
// scrape reads. The counters are read under mu, because the handler runs on
// the server's goroutine.
type benthosServer struct {
	mu      sync.Mutex
	server  *httptest.Server
	input   int64
	output  int64
	failOn  map[string]int
	request []string

	// override, when set, answers before the default handler and before
	// failOn; it returns true when it answered.
	override func(w http.ResponseWriter, r *http.Request) bool
}

func newBenthosServer(input, output int64, failOn map[string]int, override func(w http.ResponseWriter, r *http.Request) bool) *benthosServer {
	s := &benthosServer{input: input, output: output, failOn: failOn, override: override}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.request = append(s.request, r.URL.Path)
		s.mu.Unlock()

		if s.override != nil && s.override(w, r) {
			return
		}

		s.mu.Lock()
		input, output := s.input, s.output
		code, fail := s.failOn[r.URL.Path]
		s.mu.Unlock()

		if fail {
			// The error body stays valid for its endpoint, so the specs
			// below fail only when the status-code check in get is gone:
			// a body that also fails to decode would mask a missing
			// check.
			w.WriteHeader(code)
			switch r.URL.Path {
			case "/ready":
				_, _ = w.Write([]byte(`{"error":""}`))
			case "/version":
				_, _ = w.Write([]byte(`{"version":"1.2.3"}`))
			case "/metrics":
				fmt.Fprintf(w, "input_received{label=\"\",path=\"root.input\"} %d\n", input)
			default: // /ping: a body that answers pong, so only the status tells the failure
				_, _ = w.Write([]byte("pong"))
			}
			return
		}

		switch r.URL.Path {
		case "/ping":
			_, _ = w.Write([]byte("pong"))
		case "/ready":
			_, _ = w.Write([]byte(`{"error":""}`))
		case "/version":
			_, _ = w.Write([]byte(`{"version":"1.2.3"}`))
		case "/metrics":
			fmt.Fprintf(w, "input_received{label=\"\",path=\"root.input\"} %d\n", input)
			fmt.Fprintf(w, "output_sent{label=\"\",path=\"root.output\"} %d\n", output)
		default:
			http.NotFound(w, r)
		}
	}))

	return s
}

func (s *benthosServer) port() int {
	return s.server.Listener.Addr().(*net.TCPAddr).Port
}

func (s *benthosServer) setCounters(input, output int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.input, s.output = input, output
}

func (s *benthosServer) requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.request...)
}

func (s *benthosServer) close() {
	s.server.Close()
}

// inlineServer answers like benthosServer, but lets one path be overridden:
// h answers before the defaults and returns true when it did.
func inlineServer(h func(w http.ResponseWriter, r *http.Request) bool) (*httptest.Server, int) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h(w, r) {
			return
		}
		switch r.URL.Path {
		case "/ping":
			_, _ = w.Write([]byte("pong"))
		case "/ready":
			_, _ = w.Write([]byte(`{"error":""}`))
		case "/version":
			_, _ = w.Write([]byte(`{"version":"1.2.3"}`))
		case "/metrics":
			_, _ = w.Write([]byte("input_received{label=\"\",path=\"root.input\"} 1\n"))
		}
	}))

	return s, s.Listener.Addr().(*net.TCPAddr).Port
}

var _ = Describe("Scrape", func() {
	It("fetches all endpoints", func() {
		s := newBenthosServer(10, 5, nil, nil)
		defer s.close()

		t0 := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return t0 })

		sample, err := tr.Scrape(context.Background(), s.port())
		Expect(err).NotTo(HaveOccurred())

		Expect(s.requests()).To(ConsistOf("/ping", "/ready", "/version", "/metrics"))
		Expect(sample.PingAlive).To(BeTrue())
		Expect(sample.Ready).To(BeTrue())
		Expect(sample.Version).To(Equal("1.2.3"))
		Expect(sample.At).To(Equal(t0), "At is when the scrape began")
		Expect(sample.Metrics.Inputs).To(HaveKey("root.input"))
		Expect(sample.Metrics.Inputs["root.input"].Received).To(Equal(int64(10)))
		Expect(sample.Metrics.Outputs).To(HaveKey("root.output"))
		Expect(sample.Metrics.Outputs["root.output"].Sent).To(Equal(int64(5)))
	})

	It("rejects a Tracker that was not built with NewTracker", func() {
		// A zero-value Tracker with only a client set has no engine and no
		// clock; the scrape must report that instead of panicking.
		tr := &Tracker{client: &http.Client{}}

		_, err := tr.Scrape(context.Background(), testPort)
		Expect(err).To(MatchError(ContainSubstring("NewTracker")))
	})

	It("rejects a 500 /version whose body is valid JSON", func() {
		s := newBenthosServer(10, 5, map[string]int{
			"/version": http.StatusInternalServerError,
		}, nil)
		defer s.close()

		tr := NewTracker(func() time.Time { return time.Now() })
		_, err := tr.Scrape(context.Background(), s.port())
		Expect(err).To(HaveOccurred(),
			"a Benthos answering 500 is not healthy; the body decodes, so only the status check fails it")
	})

	It("clears only PingAlive when /ping answers 500", func() {
		s := newBenthosServer(10, 5, map[string]int{
			"/ping": http.StatusInternalServerError,
		}, nil)
		defer s.close()

		tr := NewTracker(func() time.Time { return time.Now() })
		sample, err := tr.Scrape(context.Background(), s.port())
		Expect(err).NotTo(HaveOccurred())
		Expect(sample.PingAlive).To(BeFalse(),
			"the /ping body answers pong, so only the status check clears PingAlive")
		Expect(sample.Ready).To(BeTrue())
	})

	It("computes the throughput", func() {
		s := newBenthosServer(10, 5, nil, nil)
		defer s.close()

		now := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return now })

		sample, err := tr.Scrape(context.Background(), s.port())
		Expect(err).NotTo(HaveOccurred())
		Expect(sample.Input.MessagesPerSecond).To(BeZero(),
			"the first scrape holds one sample")
		Expect(sample.IsActive).To(BeFalse())

		s.setCounters(12, 6)
		now = now.Add(time.Second)
		sample, err = tr.Scrape(context.Background(), s.port())
		Expect(err).NotTo(HaveOccurred())
		Expect(sample.Input.MessagesPerSecond).To(BeNumerically("~", 2, 0.5),
			"10 -> 12 over 1s, so the scrape feeds the window")
		Expect(sample.IsActive).To(BeTrue())
	})

	It("reads a counter reset as the post-restart rate", func() {
		s := newBenthosServer(100, 100, nil, nil)
		defer s.close()

		now := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return now })

		_, err := tr.Scrape(context.Background(), s.port())
		Expect(err).NotTo(HaveOccurred())

		// A restart drops both counters. The drop is folded into the
		// offsets, so the series the engine sees never goes down and the
		// tick reads the messages the restart itself processed, never a
		// negative rate.
		s.setCounters(5, 5)
		now = now.Add(time.Second)
		sample, err := tr.Scrape(context.Background(), s.port())
		Expect(err).NotTo(HaveOccurred())
		Expect(sample.IsActive).To(BeTrue())
		Expect(sample.Input.MessagesPerSecond).To(BeNumerically("~", 5, 0.5),
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
		clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return vt }
		s := newBenthosServer(0, 0, nil, func(w http.ResponseWriter, r *http.Request) bool {
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
				fmt.Fprintf(w, "input_received{path=\"root.input\"} %d\n", n)
			default:
				return false // /ready and /version: the default handler answers
			}
			return true
		})
		defer s.close()

		tr := NewTracker(clock)
		_, err := tr.Scrape(context.Background(), s.port())
		Expect(err).NotTo(HaveOccurred())
		mu.Lock()
		vt = vt.Add(100 * time.Millisecond)
		mu.Unlock()
		sample, err := tr.Scrape(context.Background(), s.port())
		Expect(err).NotTo(HaveOccurred())
		Expect(sample.Input.MessagesPerSecond).To(BeNumerically("~", 1000, 10),
			"the three probes before /metrics take a varying time")
	})

	It("counts the instance live only when /ping answers pong", func() {
		s := newBenthosServer(1, 1, nil, func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path == "/ping" {
				_, _ = w.Write([]byte("<html>not benthos</html>"))
				return true
			}
			return false
		})
		defer s.close()

		sample, err := NewTracker(nil).Scrape(context.Background(), s.port())
		Expect(err).NotTo(HaveOccurred())
		Expect(sample.PingAlive).To(BeFalse(),
			"/ping answered, but not with pong")
	})

	It("reads Ready false on a 503 /ready whose body is valid JSON", func() {
		s := newBenthosServer(1, 1, map[string]int{"/ready": http.StatusServiceUnavailable}, nil)
		defer s.close()

		sample, err := NewTracker(nil).Scrape(context.Background(), s.port())
		Expect(err).NotTo(HaveOccurred())
		Expect(sample.Ready).To(BeFalse(),
			"the body reports no error, so only the status check clears Ready")
	})

	It("reads Ready false when /ready names an error", func() {
		s := newBenthosServer(1, 1, nil, func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path == "/ready" {
				_, _ = w.Write([]byte(`{"error":"output not connected"}`))
				return true
			}
			return false
		})
		defer s.close()

		sample, err := NewTracker(nil).Scrape(context.Background(), s.port())
		Expect(err).NotTo(HaveOccurred())
		Expect(sample.Ready).To(BeFalse())
	})

	It("treats a new port as a new counter series", func() {
		a := newBenthosServer(100, 100, nil, nil)
		defer a.close()
		b := newBenthosServer(150, 150, nil, nil)
		defer b.close()

		now := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return now })

		_, err := tr.Scrape(context.Background(), a.port())
		Expect(err).NotTo(HaveOccurred())

		now = now.Add(time.Second)
		sample, err := tr.Scrape(context.Background(), b.port())
		Expect(err).NotTo(HaveOccurred())
		Expect(sample.Input.MessagesPerSecond).To(BeZero(),
			"a new port is a new counter series, so the window rebuilds")
	})

	It("rejects a 3xx /metrics", func() {
		s := newBenthosServer(1, 1, map[string]int{"/metrics": http.StatusFound}, nil)
		defer s.close()

		_, err := NewTracker(nil).Scrape(context.Background(), s.port())
		Expect(err).To(HaveOccurred())
	})

	It("does not follow a redirect away from /metrics", func() {
		// The redirect target answers with a valid metrics body, so this
		// spec fails only when the client follows the Location header.
		s := newBenthosServer(1, 1, nil, func(w http.ResponseWriter, r *http.Request) bool {
			switch r.URL.Path {
			case "/metrics":
				http.Redirect(w, r, "/elsewhere", http.StatusFound)
				return true
			case "/elsewhere":
				fmt.Fprint(w, "input_received{label=\"\",path=\"root.input\"} 1\n")
				return true
			}
			return false
		})
		defer s.close()

		_, err := NewTracker(nil).Scrape(context.Background(), s.port())
		Expect(err).To(HaveOccurred(),
			"a 3xx answer is an error, not a hop: the scrape never leaves this Benthos' endpoints")
	})

	It("fails the scrape when /metrics does not parse", func() {
		s := newBenthosServer(1, 1, nil, func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path == "/metrics" {
				_, _ = w.Write([]byte("input_received{label=\"\",path=\"root.input\"}\n"))
				return true
			}
			return false
		})
		defer s.close()

		_, err := NewTracker(nil).Scrape(context.Background(), s.port())
		Expect(err).To(MatchError(ContainSubstring("parse /metrics")))
	})

	It("fails the scrape when /version is not JSON", func() {
		s := newBenthosServer(1, 1, nil, func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path == "/version" {
				_, _ = w.Write([]byte("<html>not benthos</html>"))
				return true
			}
			return false
		})
		defer s.close()

		_, err := NewTracker(nil).Scrape(context.Background(), s.port())
		Expect(err).To(MatchError(ContainSubstring("decode /version")))
	})
})
