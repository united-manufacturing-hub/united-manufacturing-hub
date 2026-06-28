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

package benthosmetrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Observe scrapes a benthos instance's /ping, /ready, /version and /metrics
// HTTP endpoints over the given port and returns a Scan carrying the parsed
// metrics and health check.
//
// The four endpoints are scraped independently: a failure on one does not zero
// the others. A non-context failure on any endpoint (transport, non-200,
// body-read, parse) folds into the Scan (the corresponding field stays at its
// zero/false value, MetricsAvailable stays false for /metrics) with a NIL
// error, and the remaining endpoints are still scraped. Only context
// cancellation (context.Canceled or context.DeadlineExceeded) propagates as a
// non-nil error wrapping ctx.Err, so the caller can distinguish an aborted
// observation from an observed benthos-down state.
//
// A fully-down benthos (every endpoint unreachable) yields an all-false/zero
// Scan with a nil error: a down benthos is an observed state, not an aborted
// observation. Callers must check IsLive and MetricsAvailable rather than
// relying on err != nil.
func Observe(ctx context.Context, client *http.Client, port uint16) (Scan, error) {
	base := fmt.Sprintf("http://localhost:%d", port)

	var scan Scan

	// /ping -> IsLive (status only; no body decode).
	pingResp, err := get(ctx, client, base+"/ping")
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return scan, fmt.Errorf("ping: %w", ctxErr)
		}
		// Fold: IsLive stays false; continue to /ready.
	} else {
		defer func() {
			_, _ = io.Copy(io.Discard, pingResp.Body)
			_ = pingResp.Body.Close()
		}()

		scan.HealthCheck.IsLive = pingResp.StatusCode == http.StatusOK
	}

	// /ready -> IsReady + ConnectionStatuses.
	var rr ReadyResponse
	if ok, ctxErr := fetchJSON(ctx, client, base+"/ready", &rr); ctxErr != nil {
		return scan, fmt.Errorf("ready: %w", ctxErr)
	} else if ok {
		scan.HealthCheck.IsReady = rr.Error == ""
		scan.HealthCheck.ReadyError = rr.Error
		scan.HealthCheck.ConnectionStatuses = rr.Statuses
	}

	// /version -> Version.
	var vr VersionResponse
	if ok, ctxErr := fetchJSON(ctx, client, base+"/version", &vr); ctxErr != nil {
		return scan, fmt.Errorf("version: %w", ctxErr)
	} else if ok {
		scan.HealthCheck.Version = vr.Version
	}

	// /metrics -> Metrics (raw Prometheus body, not JSON).
	metricsResp, err := get(ctx, client, base+"/metrics")
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return scan, fmt.Errorf("metrics: %w", ctxErr)
		}

		return scan, nil
	}

	defer func() {
		_, _ = io.Copy(io.Discard, metricsResp.Body)
		_ = metricsResp.Body.Close()
	}()

	if metricsResp.StatusCode != http.StatusOK {
		return scan, nil
	}

	metricsBody, err := io.ReadAll(metricsResp.Body)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return scan, fmt.Errorf("metrics body: %w", ctxErr)
		}

		return scan, nil
	}

	metrics, err := ParseMetricsFromBytes(metricsBody)
	if err != nil {
		return scan, nil
	}

	scan.Metrics = metrics
	scan.MetricsAvailable = true

	return scan, nil
}

// fetchJSON GETs url, drains the body on non-200, and decodes a 200 body into
// out. It returns ok=true only on a 200 response with a successfully decoded
// body. On any non-context failure (transport, non-200, body-read, parse) it
// returns ok=false, ctxErr=nil (the caller folds). On context cancellation
// during the scrape it returns ok=false, ctxErr=ctx.Err() (the caller
// propagates). The body is always drained before close so the connection
// returns to the keep-alive pool.
func fetchJSON[T any](ctx context.Context, client *http.Client, url string, out *T) (ok bool, ctxErr error) {
	resp, err := get(ctx, client, url)
	if err != nil {
		if cErr := ctx.Err(); cErr != nil {
			return false, cErr
		}

		return false, nil
	}

	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if cErr := ctx.Err(); cErr != nil {
			return false, cErr
		}

		return false, nil
	}

	if err := json.Unmarshal(body, out); err != nil {
		return false, nil
	}

	return true, nil
}

// get is a small helper that builds a GET request bound to ctx.
func get(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	return client.Do(req)
}
