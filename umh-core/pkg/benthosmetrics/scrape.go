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
// The scrape of the Benthos endpoints.
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
)

// ScrapeInterval is how often the caller should call Scrape. The windows in
// this package are sized for it.
const ScrapeInterval = 1 * time.Second

// scrapeClientTimeout bounds each of the four requests of a scrape.
const scrapeClientTimeout = 400 * time.Millisecond

// maxScrapeBody caps a response body, so a misbehaving Benthos cannot be
// buffered without bound on each scrape.
const maxScrapeBody = 4 << 20 // 4 MiB

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
