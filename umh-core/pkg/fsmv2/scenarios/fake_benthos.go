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

package scenarios

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"time"

	"github.com/benbjohnson/clock"
)

// benthosReadAdvance is how much scrape time one /metrics read advances by
// default, so a monitor fills the 60 s rate window in seconds of real time at
// its 1 s poll. A scenario that waits out a longer window steps the clock
// faster with setReadAdvance.
const benthosReadAdvance = 10 * time.Second

// fakeBenthosVersion is what the fake's /version reports.
const fakeBenthosVersion = "1.2.3"

// fakeBenthos serves the four endpoints a real Benthos exposes, modelled on
// the shared fake in pkg/fsmv2/benthos/fakes_test.go, which a test file cannot
// export. Each /metrics read advances the shared scrape clock once and raises
// the counters by their per-read traffic. The clock is shared, so a monitor's
// rate is exact only while no other fake is scraped. Run drives the fake
// through its setters; setFailing makes one endpoint answer a status instead.
type fakeBenthos struct {
	server *httptest.Server
	clk    *clock.Mock

	input  atomic.Int64
	output atomic.Int64
	errors atomic.Int64

	inPerRead  atomic.Int64
	outPerRead atomic.Int64
	errPerRead atomic.Int64

	// advance is how much scrape time one /metrics read adds to the shared
	// clock, and inputConnected is whether the input's connection gauges
	// report it connected. Run steps both through their setters.
	advance        atomic.Int64
	inputConnected atomic.Bool

	closeOnce sync.Once
	mu        sync.Mutex
	failOn    map[string]int
}

// newFakeBenthos builds a fake that answers like a healthy, idle Benthos: pong
// on /ping, an empty error on /ready, and zero counters on /metrics.
func newFakeBenthos(clk *clock.Mock) *fakeBenthos {
	s := &fakeBenthos{clk: clk, failOn: map[string]int{}}
	s.advance.Store(int64(benthosReadAdvance))
	s.inputConnected.Store(true)

	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		code, fail := s.failOn[r.URL.Path]
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
			fmt.Fprintf(w, `{"version":%q,"built":"fake"}`, fakeBenthosVersion)
		case "/metrics":
			s.clk.Add(time.Duration(s.advance.Load()))
			input := s.input.Add(s.inPerRead.Load())
			output := s.output.Add(s.outPerRead.Load())

			// The error counter moves under mu together with its per-read
			// increment, so a read cannot apply a stale increment to a
			// counter setCounters has just reset.
			s.mu.Lock()
			errors := s.errors.Add(s.errPerRead.Load())
			s.mu.Unlock()

			// The input's connection gauges: up while Benthos holds the
			// connection, and up below lost with a failure behind it while it
			// does not, the shape benthosmetrics counts as not connected.
			up, lost, failed := int64(1), int64(0), int64(0)
			if !s.inputConnected.Load() {
				up, lost, failed = 0, 1, 1
			}

			// Real exposition shape: per-leaf series with a path label and no
			// top-level aggregate, so the parser must sum across them.
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintf(w, "# HELP input_received Benthos Counter metric\n")
			fmt.Fprintf(w, "# TYPE input_received counter\n")
			fmt.Fprintf(w, "input_received{label=\"\",path=\"root.input\"} %d\n", input)
			fmt.Fprintf(w, "# HELP input_connection_up Benthos Counter metric\n")
			fmt.Fprintf(w, "# TYPE input_connection_up counter\n")
			fmt.Fprintf(w, "input_connection_up{label=\"\",path=\"root.input\"} %d\n", up)
			fmt.Fprintf(w, "# HELP input_connection_lost Benthos Counter metric\n")
			fmt.Fprintf(w, "# TYPE input_connection_lost counter\n")
			fmt.Fprintf(w, "input_connection_lost{label=\"\",path=\"root.input\"} %d\n", lost)
			fmt.Fprintf(w, "# HELP input_connection_failed Benthos Counter metric\n")
			fmt.Fprintf(w, "# TYPE input_connection_failed counter\n")
			fmt.Fprintf(w, "input_connection_failed{label=\"\",path=\"root.input\"} %d\n", failed)
			fmt.Fprintf(w, "# HELP output_sent Benthos Counter metric\n")
			fmt.Fprintf(w, "# TYPE output_sent counter\n")
			fmt.Fprintf(w, "output_sent{label=\"\",path=\"root.output.broker.0\"} %d\n", output/2)
			fmt.Fprintf(w, "output_sent{label=\"\",path=\"root.output.broker.1\"} %d\n", output-output/2)
			fmt.Fprintf(w, "# HELP output_error Benthos Counter metric\n")
			fmt.Fprintf(w, "# TYPE output_error counter\n")
			fmt.Fprintf(w, "output_error{label=\"\",path=\"root.output\"} %d\n", errors)
		default:
			http.NotFound(w, r)
		}
	}))

	return s
}

// port is the port the fake listens on.
func (s *fakeBenthos) port() uint16 {
	return uint16(s.server.Listener.Addr().(*net.TCPAddr).Port)
}

// counters returns the counters the last scrape read.
func (s *fakeBenthos) counters() (input, output, errors int64) {
	return s.input.Load(), s.output.Load(), s.errors.Load()
}

// setTraffic sets how much the input, output and error counters rise on every
// /metrics read; zero stops the traffic.
func (s *fakeBenthos) setTraffic(inputPerRead, outputPerRead, errorPerRead int64) {
	s.inPerRead.Store(inputPerRead)
	s.outPerRead.Store(outputPerRead)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.errPerRead.Store(errorPerRead)
}

// setReadAdvance sets how much scrape time one /metrics read advances the
// shared clock. A scenario waiting out a window much longer than the rate
// window steps the clock in bigger strides, so the wait costs one real
// second per stride instead of one per scrape second.
func (s *fakeBenthos) setReadAdvance(d time.Duration) {
	s.advance.Store(int64(d))
}

// setInputConnected sets whether the input's connection gauges report it
// connected: up at 1 while connected, and a lost, failed connect while not.
func (s *fakeBenthos) setInputConnected(connected bool) {
	s.inputConnected.Store(connected)
}

// now returns the shared scrape clock's current time, for a wait that names
// how far the clock has stepped past an entry's own timestamps.
func (s *fakeBenthos) now() time.Time {
	return s.clk.Now()
}

// setCounters sets the counters absolutely, the way a Benthos restart resets
// them. Call it after setTraffic has stopped the error traffic: an error
// increment still in flight would otherwise land on the reset counter.
func (s *fakeBenthos) setCounters(input, output, errors int64) {
	s.input.Store(input)
	s.output.Store(output)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.errors.Store(errors)
}

// setFailing makes path answer the given status instead; status 0 serves it
// again.
func (s *fakeBenthos) setFailing(path string, status int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if status == 0 {
		delete(s.failOn, path)
		return
	}

	s.failOn[path] = status
}

// close shuts the server down; a fake closed mid-run stays closed.
func (s *fakeBenthos) close() {
	s.closeOnce.Do(s.server.Close)
}
