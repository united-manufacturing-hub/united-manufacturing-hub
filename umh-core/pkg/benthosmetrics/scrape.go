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

// scrapeClientTimeout bounds each of the four requests of a scrape.
const scrapeClientTimeout = 400 * time.Millisecond

// maxScrapeBody caps a response body, so a misbehaving Benthos cannot be
// buffered without bound on each scrape.
const maxScrapeBody = 4 << 20 // 4 MiB

// Tracker is the state kept about one Benthos instance. Build it with
// NewTracker; the caller drops it when the instance goes away. A Tracker
// is not safe for concurrent use; one goroutine owns it.
type Tracker struct {
	client *http.Client

	// engine holds the windows that turn counters into throughput across
	// scrapes.
	engine *diagnosis.Engine[readings]

	// prev is the previous observed scrape, nil before the first.
	prev *observation

	// offsets folds counter restarts into the series the engine is fed, one
	// per direction; see adjustForRestart.
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
		engine: mustNewEngine(),
		now:    now,
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

	// The port is passed because a config update can change it without a
	// restart.
	return probes, t.observe(port, metrics, readAt), nil
}

// readyResponse is the JSON body of /ready; Error is empty when every
// connection is up.
type readyResponse struct {
	Error string `json:"error"`
}

// versionResponse is the JSON body of /version.
type versionResponse struct {
	Version string `json:"version"`
}

// probePing reports whether /ping answered pong. Benthos answers pong; another
// server on the port does not.
func probePing(ctx context.Context, client *http.Client, base string) (bool, error) {
	body, err := get(ctx, client, base+pingPath)
	if err != nil {
		return false, err
	}

	return bytes.Contains(body, []byte("pong")), nil
}

// probeReady reports whether /ready answered 2xx with no error.
func probeReady(ctx context.Context, client *http.Client, base string) (bool, error) {
	body, err := get(ctx, client, base+readyPath)
	if err != nil {
		return false, err
	}

	var r readyResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return false, fmt.Errorf("decode %s: %w", readyPath, err)
	}

	return r.Error == "", nil
}

// fetchVersion returns the version string /version reports.
func fetchVersion(ctx context.Context, client *http.Client, base string) (string, error) {
	body, err := get(ctx, client, base+versionPath)
	if err != nil {
		return "", err
	}

	var v versionResponse
	if err := json.Unmarshal(body, &v); err != nil {
		return "", fmt.Errorf("decode %s: %w", versionPath, err)
	}

	return v.Version, nil
}

// fetchMetrics returns the parsed /metrics and the time now reported when its
// body was read.
func fetchMetrics(ctx context.Context, client *http.Client, base string, now func() time.Time) (Metrics, time.Time, error) {
	body, err := get(ctx, client, base+metricsPath)
	if err != nil {
		return Metrics{}, time.Time{}, err
	}

	readAt := now()

	m, err := parseMetrics(body)
	if err != nil {
		return Metrics{}, time.Time{}, fmt.Errorf("parse %s: %w", metricsPath, err)
	}

	return m, readAt, nil
}

// get performs one GET with ctx and returns its body. A non-2xx answer is an
// error: an error page is not a payload, and parsing it yields garbage.
func get(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err // a *url.Error, which already names the method and URL
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
		return nil, fmt.Errorf("GET %s: read body: %w", url, err)
	}

	if len(body) > maxScrapeBody {
		return nil, fmt.Errorf("GET %s: response body exceeds %d bytes", url, maxScrapeBody)
	}

	return body, nil
}
