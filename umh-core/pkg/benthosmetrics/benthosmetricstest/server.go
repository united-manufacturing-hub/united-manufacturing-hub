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

// Package benthosmetricstest provides a fake Benthos for tests that scrape
// one, in the way net/http/httptest provides a fake HTTP server.
package benthosmetricstest

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
)

// Version is what a new Server's /version reports.
const Version = "1.2.3"

// Server is a fake Benthos that answers the four endpoints a scrape reads:
// /ping, /ready, /version and /metrics. A new Server answers like a healthy,
// idle Benthos. Its methods are safe to call while it is being scraped.
type Server struct {
	server    *httptest.Server
	closeOnce sync.Once

	mu sync.Mutex

	// input, output and errors are the counters /metrics serves.
	input, output, errors int64

	// inPerRead, outPerRead and errPerRead are added to the counters on
	// every /metrics read; see SetTraffic.
	inPerRead, outPerRead, errPerRead int64

	failOn      map[string]int
	version     string
	metricsBody string
	handler     func(w http.ResponseWriter, r *http.Request) bool
	onMetrics   func()
	requested   []string
}

// NewServer starts a Server. The caller closes it with Close.
func NewServer() *Server {
	s := &Server{failOn: map[string]int{}, version: Version}
	s.server = httptest.NewServer(http.HandlerFunc(s.serve))

	return s
}

// Close shuts the server down. Closing a closed Server does nothing, so a
// test can close it mid-run and again in its cleanup.
func (s *Server) Close() {
	s.closeOnce.Do(s.server.Close)
}

// Port is the port the server listens on.
func (s *Server) Port() uint16 {
	return uint16(s.server.Listener.Addr().(*net.TCPAddr).Port) //nolint:gosec // a TCP port fits in uint16
}

// Requests returns the paths requested so far, in order.
func (s *Server) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.requested...)
}

// SetCounters sets the input, output and output-error counters /metrics
// serves, as a Benthos restart resets them.
func (s *Server) SetCounters(input, output, errors int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.input, s.output, s.errors = input, output, errors
}

// Counters returns the counters the last /metrics read served.
func (s *Server) Counters() (input, output, errors int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.input, s.output, s.errors
}

// SetTraffic sets how much the counters rise on every /metrics read, before
// the read serves them. Zero stops the traffic.
func (s *Server) SetTraffic(inputPerRead, outputPerRead, errorPerRead int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.inPerRead, s.outPerRead, s.errPerRead = inputPerRead, outputPerRead, errorPerRead
}

// SetFailing makes path answer status instead of 200. The body stays valid
// for the endpoint, so a scrape fails only by checking the status. Status 0
// serves path normally again.
func (s *Server) SetFailing(path string, status int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if status == 0 {
		delete(s.failOn, path)

		return
	}

	s.failOn[path] = status
}

// SetVersion sets what /version reports.
func (s *Server) SetVersion(version string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.version = version
}

// SetMetricsBody makes /metrics serve body instead of the counters. An empty
// body serves the counters again.
func (s *Server) SetMetricsBody(body string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metricsBody = body
}

// SetHandler makes handler see every request first. It returns true when it
// answered; false lets the Server answer as usual. Use it for an answer no
// setter gives, such as a redirect or a body that is not Benthos's.
func (s *Server) SetHandler(handler func(w http.ResponseWriter, r *http.Request) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.handler = handler
}

// OnMetricsRead makes every /metrics read call fn first, before the traffic
// is added. A scenario uses it to advance a mock clock once per scrape.
func (s *Server) OnMetricsRead(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.onMetrics = fn
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requested = append(s.requested, r.URL.Path)
	handler, onMetrics := s.handler, s.onMetrics
	status, failing := s.failOn[r.URL.Path]
	s.mu.Unlock()

	// The handler and onMetrics run without mu, so they may call the setters.
	if handler != nil && handler(w, r) {
		return
	}

	// A failing /metrics read is not a scrape Benthos answered, so it
	// neither advances the clock nor adds traffic.
	reading := r.URL.Path == "/metrics" && !failing
	if reading && onMetrics != nil {
		onMetrics()
	}

	s.mu.Lock()
	if reading {
		s.input += s.inPerRead
		s.output += s.outPerRead
		s.errors += s.errPerRead
	}

	input, output, errors := s.input, s.output, s.errors
	version, metricsBody := s.version, s.metricsBody
	s.mu.Unlock()

	if failing {
		w.WriteHeader(status)
	}

	switch r.URL.Path {
	case "/ping":
		_, _ = w.Write([]byte("pong"))
	case "/ready":
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"","statuses":[]}`))
	case "/version":
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"version":%q,"built":"fake"}`, version)
	case "/metrics":
		w.Header().Set("Content-Type", "text/plain")

		if metricsBody != "" {
			_, _ = w.Write([]byte(metricsBody))

			return
		}

		writeCounters(w, input, output, errors)
	default:
		http.NotFound(w, r)
	}
}

// writeCounters writes the counters in Benthos's exposition shape: one series
// per leaf with a path label and no top-level aggregate, so a parser has to
// add up the two output brokers.
func writeCounters(w http.ResponseWriter, input, output, errors int64) {
	_, _ = fmt.Fprintf(w, "# HELP input_received Benthos Counter metric\n")
	_, _ = fmt.Fprintf(w, "# TYPE input_received counter\n")
	_, _ = fmt.Fprintf(w, "input_received{label=\"\",path=\"root.input\"} %d\n", input)
	_, _ = fmt.Fprintf(w, "# HELP output_sent Benthos Counter metric\n")
	_, _ = fmt.Fprintf(w, "# TYPE output_sent counter\n")
	_, _ = fmt.Fprintf(w, "output_sent{label=\"\",path=\"root.output.broker.0\"} %d\n", output/2)
	_, _ = fmt.Fprintf(w, "output_sent{label=\"\",path=\"root.output.broker.1\"} %d\n", output-output/2)
	_, _ = fmt.Fprintf(w, "# HELP output_error Benthos Counter metric\n")
	_, _ = fmt.Fprintf(w, "# TYPE output_error counter\n")
	_, _ = fmt.Fprintf(w, "output_error{label=\"\",path=\"root.output\"} %d\n", errors)
}
