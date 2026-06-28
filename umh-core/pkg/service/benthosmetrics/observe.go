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
// A /ping transport failure (including a fully-down benthos where every
// endpoint is unreachable) yields an all-false/zero Scan with a nil error: a
// down benthos is an observed state, not an aborted observation. A canceled
// context is propagated as a non-nil error so the caller can distinguish an
// aborted observation from the nil-error Scan a down benthos returns. When the
// context is canceled, the GET failure (or, for /metrics, the body-read
// failure) wraps ctx.Err() (errors.Is(err, context.Canceled) or
// errors.Is(err, context.DeadlineExceeded)); on /ready and /version body-read
// the cancel surfaces as the body-read error, still non-nil. Cancellation is a
// caller-side fault, not an observed benthos-down state. With a live context,
// transport GET failures fold on /ping, /ready and /metrics (see below) but
// propagate as a non-nil error on /version.
//
// A /ready transport failure occurring after a successful /ping is folded into
// a partial Scan with IsLive preserved and IsReady false; /version and /metrics
// are scraped independently, so MetricsAvailable reflects the /metrics result
// (it is not forced false by the /ready fold). The fold yields a nil error only
// when /version and /metrics do not subsequently transport-fail; a /version
// transport, body-read, or parse failure is propagated as a non-nil error (see
// below). A /ready body-read or parse failure is likewise propagated as a
// non-nil error wrapping the cause, alongside the partial Scan collected so far.
//
// A /version transport, body-read, or parse failure is propagated as a non-nil
// error wrapping the cause, alongside the partial Scan collected so far (e.g.
// IsLive when /ping succeeded); /version is not folded. A non-200 /version
// response leaves Version empty and continues to /metrics.
//
// A /metrics non-200, transport, body-read, or parse failure yields a nil
// error with MetricsAvailable=false and the already-collected /ping, /ready
// and /version HealthCheck fields preserved; callers must check
// MetricsAvailable rather than rely on err != nil. A canceled-context
// body-read failure, however, is propagated as a non-nil error wrapping
// ctx.Err().
func Observe(ctx context.Context, client *http.Client, port uint16) (Scan, error) {
	base := fmt.Sprintf("http://localhost:%d", port)

	var scan Scan

	// /ping -> IsLive
	pingResp, err := get(ctx, client, base+"/ping")
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return scan, fmt.Errorf("ping: %w", ctxErr)
		}

		return scan, nil
	}

	defer func() { _ = pingResp.Body.Close() }()

	scan.HealthCheck.IsLive = pingResp.StatusCode == http.StatusOK

	// /ready -> IsReady + ConnectionStatuses
	readyResp, err := get(ctx, client, base+"/ready")
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return scan, fmt.Errorf("ready: %w", ctxErr)
		}
		// Fold: a /ready transport failure after a successful /ping preserves
		// IsLive and leaves IsReady false; /version and /metrics are scraped
		// independently.
	} else {
		defer func() { _ = readyResp.Body.Close() }()

		readyBody, err := io.ReadAll(readyResp.Body)
		if err != nil {
			return scan, fmt.Errorf("ready body: %w", err)
		}

		var rr ReadyResponse
		if err := json.Unmarshal(readyBody, &rr); err != nil {
			return scan, fmt.Errorf("ready unmarshal: %w", err)
		}

		scan.HealthCheck.IsReady = rr.Error == ""
		scan.HealthCheck.ReadyError = rr.Error
		scan.HealthCheck.ConnectionStatuses = rr.Statuses
	}

	// /version -> Version
	versionResp, err := get(ctx, client, base+"/version")
	if err != nil {
		return scan, fmt.Errorf("version: %w", err)
	}

	defer func() { _ = versionResp.Body.Close() }()

	if versionResp.StatusCode == http.StatusOK {
		versionBody, err := io.ReadAll(versionResp.Body)
		if err != nil {
			return scan, fmt.Errorf("version body: %w", err)
		}

		var vr VersionResponse
		if err := json.Unmarshal(versionBody, &vr); err != nil {
			return scan, fmt.Errorf("version unmarshal: %w", err)
		}

		scan.HealthCheck.Version = vr.Version
	} else {
		// Drain the body so the deferred Close returns the connection to the
		// pool instead of discarding an unread body (which forfeits keep-alive).
		_, _ = io.Copy(io.Discard, versionResp.Body)
	}

	// /metrics -> Metrics
	metricsResp, err := get(ctx, client, base+"/metrics")
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return scan, fmt.Errorf("metrics: %w", ctxErr)
		}

		return scan, nil
	}

	defer func() { _ = metricsResp.Body.Close() }()

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

// get is a small helper that builds a GET request bound to ctx.
func get(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	return client.Do(req)
}
