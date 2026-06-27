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
// A non-200 /metrics response yields a nil error with MetricsAvailable=false
// and a populated HealthCheck; callers must check MetricsAvailable rather than
// rely on err != nil.
func Observe(ctx context.Context, client *http.Client, port uint16) (Scan, error) {
	base := fmt.Sprintf("http://localhost:%d", port)

	var scan Scan

	// /ping -> IsLive
	pingResp, err := get(ctx, client, base+"/ping")
	if err != nil {
		return Scan{}, fmt.Errorf("ping: %w", err)
	}

	defer func() { _ = pingResp.Body.Close() }()

	scan.HealthCheck.IsLive = pingResp.StatusCode == http.StatusOK

	// /ready -> IsReady + ConnectionStatuses
	readyResp, err := get(ctx, client, base+"/ready")
	if err != nil {
		return Scan{}, fmt.Errorf("ready: %w", err)
	}

	defer func() { _ = readyResp.Body.Close() }()

	readyBody, err := io.ReadAll(readyResp.Body)
	if err != nil {
		return Scan{}, fmt.Errorf("ready body: %w", err)
	}

	var rr ReadyResponse
	if err := json.Unmarshal(readyBody, &rr); err != nil {
		return Scan{}, fmt.Errorf("ready unmarshal: %w", err)
	}

	scan.HealthCheck.IsReady = rr.Error == ""
	scan.HealthCheck.ConnectionStatuses = rr.Statuses

	// /version -> Version
	versionResp, err := get(ctx, client, base+"/version")
	if err != nil {
		return Scan{}, fmt.Errorf("version: %w", err)
	}

	defer func() { _ = versionResp.Body.Close() }()

	versionBody, err := io.ReadAll(versionResp.Body)
	if err != nil {
		return Scan{}, fmt.Errorf("version body: %w", err)
	}

	var vr VersionResponse
	if err := json.Unmarshal(versionBody, &vr); err != nil {
		return Scan{}, fmt.Errorf("version unmarshal: %w", err)
	}

	scan.HealthCheck.Version = vr.Version

	// /metrics -> Metrics
	metricsResp, err := get(ctx, client, base+"/metrics")
	if err != nil {
		return Scan{}, fmt.Errorf("metrics: %w", err)
	}

	defer func() { _ = metricsResp.Body.Close() }()

	if metricsResp.StatusCode != http.StatusOK {
		return scan, nil
	}

	metricsBody, err := io.ReadAll(metricsResp.Body)
	if err != nil {
		return Scan{}, fmt.Errorf("metrics body: %w", err)
	}

	metrics, err := ParseMetricsFromBytes(metricsBody)
	if err != nil {
		return Scan{}, fmt.Errorf("metrics parse: %w", err)
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
