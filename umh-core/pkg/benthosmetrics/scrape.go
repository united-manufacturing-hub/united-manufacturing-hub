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

// The scrape of the Benthos endpoints, and the per-instance state it feeds.

package benthosmetrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
)

// ScrapeInterval is how often the caller should call Scrape. The windows in
// this package are sized for it.
const ScrapeInterval = 1 * time.Second

// scrapeClientTimeout bounds each request. A scrape makes four requests
// in sequence — /ping, /ready, /version, /metrics — so its worst case is
// four timeouts, 1.6 s, longer than ScrapeInterval: the caller polls
// serially, so a slow scrape delays only itself and scrapes do not back up.
const scrapeClientTimeout = 400 * time.Millisecond

// maxScrapeBody caps a response body, so a misbehaving Benthos cannot be
// buffered without bound on each scrape.
const maxScrapeBody = 4 << 20 // 4 MiB

// Tracker is the state kept about one Benthos instance. Build it with
// NewTracker; the caller drops it when the instance goes away. A Tracker
// is not safe for concurrent use; one goroutine owns it.
type Tracker struct {
	client *http.Client

	// engine holds the windows that turn counters into rates across scrapes.
	engine *diagnosis.Engine[readings]

	// errorEngine holds the windows that turn error counters into failure
	// entries across scrapes. It is separate from engine so a Benthos
	// restart can clear the error state on its own, without wiping the rate
	// windows (see observe).
	errorEngine *diagnosis.Engine[readings]

	// prev is the previous observed scrape, nil before the first.
	prev *observation

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

// NewTracker returns a Tracker for one Benthos instance. A nil clock means
// time.Now.
func NewTracker(clock func() time.Time) *Tracker {
	if clock == nil {
		clock = time.Now
	}

	return &Tracker{
		client: &http.Client{
			Timeout: scrapeClientTimeout,
			// A 3xx answer is an error, not a hop: the scrape never leaves
			// this Benthos' endpoints. See
			// https://pkg.go.dev/net/http#ErrUseLastResponse: the 3xx
			// response itself is returned.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		engine:      newEngine(rateTable()),
		errorEngine: newEngine(errorTable()),
		now:         clock,
	}
}

// Scrape requests /ping, /ready, /version and /metrics from the Benthos at
// port once, records the counters in t, and returns the Sample with its rates
// and Failures. Requests carry ctx.
func (t *Tracker) Scrape(ctx context.Context, port int) (Sample, error) {
	if t == nil || t.client == nil || t.engine == nil || t.errorEngine == nil || t.now == nil {
		return Sample{}, fmt.Errorf("benthosmetrics: Tracker has no HTTP client, engine or clock; build it with NewTracker")
	}

	base := fmt.Sprintf("http://localhost:%d", port)
	sample := Sample{At: t.now()}

	// /ping and /ready only fill PingAlive and Ready; a failed probe does not
	// fail the scrape. Benthos answers /ping with "pong"; another server does
	// not. The endpoints and their handlers are cited in names.go.
	if pingBody, err := get(ctx, t.client, base+pingPath); err == nil {
		sample.PingAlive = bytes.Contains(pingBody, []byte("pong"))
	}

	if readyBody, err := get(ctx, t.client, base+readyPath); err == nil {
		// /ready reports readiness as JSON whose error field is empty when
		// every connection is up.
		var r struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(readyBody, &r); err == nil {
			sample.Ready = r.Error == ""
		}
	}

	versionBody, err := get(ctx, t.client, base+versionPath)
	if err != nil {
		return sample, fmt.Errorf("scrape /version: %w", err)
	}
	var v struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(versionBody, &v); err != nil {
		return sample, fmt.Errorf("decode /version: %w", err)
	}
	sample.Version = v.Version

	metricsBody, err := get(ctx, t.client, base+metricsPath)
	if err != nil {
		return sample, fmt.Errorf("scrape /metrics: %w", err)
	}
	// The counters are stamped when they were read, not when the scrape
	// began: the three probes before /metrics take a varying time.
	countersAt := t.now()
	m, err := ParseMetricsFromBytes(metricsBody)
	if err != nil {
		return sample, fmt.Errorf("parse /metrics: %w", err)
	}
	sample.Metrics = m

	// The port is passed because a config change can set a new one; S6
	// recreates the service on any config change, so observe reads a new
	// port as a restart and a new counter series.
	rates, failures := t.observe(port, m, countersAt)
	sample.rates = rates
	sample.Failures = &failures

	return sample, nil
}

// get performs one GET with ctx and returns its body. A non-2xx answer is an
// error: an error page is not a payload, and parsing it yields garbage.
func get(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	// nilaway cannot prove resp is non-nil when Do returns a nil error.
	if resp == nil {
		return nil, fmt.Errorf("GET %s: nil response", url)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Drain the error body, so the connection can be reused: a Benthos
		// that is not ready answers /ready with a non-2xx on every scrape. See
		// https://pkg.go.dev/net/http#Response: a Body not read to
		// completion may keep the keep-alive connection from being reused.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxScrapeBody))

		return nil, fmt.Errorf("GET %s: unexpected status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxScrapeBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxScrapeBody {
		return nil, fmt.Errorf("GET %s: response body exceeds %d bytes", url, maxScrapeBody)
	}

	return body, nil
}
