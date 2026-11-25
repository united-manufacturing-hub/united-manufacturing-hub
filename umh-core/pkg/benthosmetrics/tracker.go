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

// The per-instance state the scrape feeds.

package benthosmetrics

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
)

// Tracker is the state kept about one Benthos instance. Build it with
// NewTracker; the caller drops it when the instance goes away. A Tracker
// is not safe for concurrent use; one goroutine owns it.
type Tracker struct {
	client *http.Client

	// engine holds the windows that turn counters into throughput across
	// scrapes.
	engine *diagnosis.Engine[readings]

	// failures holds the error state across scrapes. It is separate from
	// engine so a Benthos restart can clear the error state on its own,
	// without wiping the throughput windows (see observe).
	failures *failureTracker

	// previous is the previous observed scrape, nil before the first.
	previous *observation

	// offsets folds counter restarts into the series the engine is fed, one
	// per direction; see observe.
	offsets offsets

	now func() time.Time
}

// offsets is the per-counter restart bookkeeping observe keeps: when a
// counter reads lower than its previous value, that value is added to its
// offset, so the series value + offset never decreases.
type offsets struct {
	input  int64
	output int64
}

// NewTracker returns a Tracker for one Benthos instance that reads the time
// from now, which must not be nil.
func NewTracker(now func() time.Time) *Tracker {
	return &Tracker{
		client: &http.Client{
			Timeout: scrapeClientTimeout,
			// A 3xx answer is an error, not a hop: the scrape never leaves
			// this Benthos' endpoints. See
			// https://pkg.go.dev/net/http#ErrUseLastResponse: the 3xx
			// response itself is returned.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		engine:   mustNewEngine(table()),
		failures: newFailureTracker(),
		now:      now,
	}
}

// Scrape scrapes the Benthos at port's /ping, /ready, /version and /metrics
// endpoints once, records the counters in the windows t holds, and returns
// what the probes answered and the measured Sample. Probes is valid also when
// the error is not nil; Sample is valid only when it is nil. All requests
// carry ctx, so cancellation surfaces as a request error.
func (t *Tracker) Scrape(ctx context.Context, port int) (Probes, Sample, error) {
	base := fmt.Sprintf("http://localhost:%d", port)
	probes := Probes{At: t.now()}

	// A failed /ping or /ready does not fail the scrape.
	probes.PingAlive, _ = probePing(ctx, t.client, base)
	probes.Ready, _ = probeReady(ctx, t.client, base)

	version, err := fetchVersion(ctx, t.client, base)
	if err != nil {
		return probes, Sample{}, err
	}

	probes.Version = version

	// The counters are stamped when /metrics was read, not when the scrape
	// began: the probes before it take a varying time.
	metrics, readAt, err := fetchMetrics(ctx, t.client, base, t.now)
	if err != nil {
		return probes, Sample{}, err
	}

	// The port is passed because a config change can set a new one; S6
	// recreates the service on any config change, so observe reads a new
	// port as a restart and a new counter series.
	return probes, t.observe(port, metrics, readAt), nil
}
