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

// The shared fake Benthos the specs in this package scrape: one server for
// the four endpoints, with the counters a spec raises between scrapes.

package fsmv2benthos

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
)

// fakeBenthos serves the four endpoints a real Benthos exposes. input and
// output are atomic so a spec can raise them between scrapes, the way a live
// Benthos does as messages flow. metricsBody, when set, overrides the
// exposition it serves; failOn answers that path with that status instead;
// version is what /version reports. Set them before the first scrape.
type fakeBenthos struct {
	server *httptest.Server

	input  atomic.Int64
	output atomic.Int64

	mu          sync.Mutex
	metricsBody string
	failOn      map[string]int
	version     string
	requested   []string
}

// newFakeBenthos builds a fake that answers like a healthy Benthos: pong on
// /ping, an empty error on /ready, and the counters on /metrics.
func newFakeBenthos() *fakeBenthos {
	s := &fakeBenthos{version: "1.2.3"}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requested = append(s.requested, r.URL.Path)
		code, fail := s.failOn[r.URL.Path]
		metricsBody, version := s.metricsBody, s.version
		input, output := s.input.Load(), s.output.Load()
		s.mu.Unlock()

		if fail {
			http.Error(w, "boom", code)
			return
		}

		switch r.URL.Path {
		case "/ping":
			_, _ = w.Write([]byte("pong"))
		case "/ready":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"error":"","statuses":[]}`))
		case "/version":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"version":%q,"built":"fake"}`, version)
		case "/metrics":
			w.Header().Set("Content-Type", "text/plain")
			if metricsBody != "" {
				_, _ = w.Write([]byte(metricsBody))
				return
			}
			// Real exposition shape: per-leaf series with a path label and no
			// top-level aggregate, so the parser must sum across them.
			fmt.Fprintf(w, "# HELP input_received Benthos Counter metric\n")
			fmt.Fprintf(w, "# TYPE input_received counter\n")
			fmt.Fprintf(w, "input_received{label=\"\",path=\"root.input\"} %d\n", input)
			fmt.Fprintf(w, "# HELP output_sent Benthos Counter metric\n")
			fmt.Fprintf(w, "# TYPE output_sent counter\n")
			fmt.Fprintf(w, "output_sent{label=\"\",path=\"root.output.broker.0\"} %d\n", output/2)
			fmt.Fprintf(w, "output_sent{label=\"\",path=\"root.output.broker.1\"} %d\n", output-output/2)
		default:
			http.NotFound(w, r)
		}
	}))

	return s
}

// requests holds the paths scraped so far.
func (s *fakeBenthos) requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.requested...)
}

// port is the port the fake listens on.
func (s *fakeBenthos) port() uint16 {
	return uint16(s.server.Listener.Addr().(*net.TCPAddr).Port)
}

// setCounters sets the input and output counters the next scrape reads.
func (s *fakeBenthos) setCounters(input, output int64) {
	s.input.Store(input)
	s.output.Store(output)
}

// close shuts the server down.
func (s *fakeBenthos) close() { s.server.Close() }
