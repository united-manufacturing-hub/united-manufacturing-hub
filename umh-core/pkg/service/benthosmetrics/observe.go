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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// Observe scrapes a benthos instance's /ping, /ready, /version and /metrics
// HTTP endpoints over the given port and returns a Scan carrying the parsed
// metrics and health check.
//
// logger receives the observability signal for /metrics regressions (a non-200
// /metrics response, a body-read failure, or a parse failure). It may be nil,
// in which case the regression is silently folded like a transport failure.
// Transport failures (the /metrics endpoint is unreachable) stay folded
// silently: an unreachable /metrics is an observed-down state, not a
// regression. A 200 /metrics whose body cannot be parsed is a regression (for
// example a benthos release bumping the Prometheus exposition format) and is
// logged distinctly via logger.SentryWarn so operators can tell a parser break
// from a benthos-down state. The log carries the port, the response status,
// the raw body length and a short body snippet.
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
func Observe(ctx context.Context, client *http.Client, port uint16, logger deps.FSMLogger) (Scan, error) {
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
		// A non-200 /metrics while /ping is 200 is a regression, not an
		// observed-down state: surface it so operators can tell a broken
		// metrics endpoint from an unreachable one.
		logMetricsRegression(logger, "benthos_metrics_non_200",
			deps.Int("port", int(port)),
			deps.Int("status_code", metricsResp.StatusCode))

		return scan, nil
	}

	metricsBody, err := io.ReadAll(metricsResp.Body)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return scan, fmt.Errorf("metrics body: %w", ctxErr)
		}

		// /metrics returned 200 but the body could not be read: a regression
		// distinct from the transport-unreachable fold above.
		logMetricsRegression(logger, "benthos_metrics_body_read_failed",
			deps.Int("port", int(port)),
			deps.String("error", err.Error()))

		return scan, nil
	}

	metrics, err := ParseMetricsFromBytes(metricsBody)
	if err != nil {
		// /metrics returned 200 and was read, but the body could not be parsed.
		// A persistent parse failure (e.g. a benthos release bumping the
		// Prometheus exposition format) would otherwise freeze the throughput
		// window indefinitely while /ping stays 200, invisible to operators.
		logMetricsRegression(logger, "benthos_metrics_parse_failed",
			deps.Int("port", int(port)),
			deps.Int("body_len", len(metricsBody)),
			deps.String("snippet", metricsSnippet(metricsBody)),
			deps.String("error", err.Error()))

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

// logMetricsRegression emits a SentryWarn for a /metrics regression (non-200,
// body-read failure, or parse failure). A nil logger folds the regression
// silently, matching the pre-observability transport-fold behavior, so callers
// that do not care about the signal (benchmarks, down-state folds) can pass
// nil. The message identifies the regression class; the fields carry the
// diagnostic detail.
func logMetricsRegression(logger deps.FSMLogger, msg string, fields ...deps.Field) {
	if logger == nil {
		return
	}

	logger.SentryWarn(deps.FeatureFSMv2, "", msg, fields...)
}

// metricsSnippet returns a short, safe prefix of the raw /metrics body for log
// output. It is capped so a runaway body never floods the log line.
func metricsSnippet(body []byte) string {
	const max = 128

	if len(body) <= max {
		return string(body)
	}

	return string(body[:max])
}
