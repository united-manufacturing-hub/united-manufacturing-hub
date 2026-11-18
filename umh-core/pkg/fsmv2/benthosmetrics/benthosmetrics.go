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

// Package fsmv2benthosmetrics is the FSMv2 simple monitor that scrapes each Benthos
// instance's metrics through pkg/benthosmetrics. Importing it registers the
// "benthos_metrics" worker; NewManager runs one worker per
// enabled BenthosMonitorConfig.
package fsmv2benthosmetrics

import (
	"context"
	"fmt"
	"time"

	"github.com/benbjohnson/clock"

	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/benthosmetrics"
)

// WorkerType is the worker-type name this monitor registers under in config
// and CSE storage.
const WorkerType = "benthos_metrics"

// benthosDeps is one worker's dependencies: the Tracker holding this Benthos
// instance's scrape state. The framework creates and drops it with the worker.
type benthosDeps struct {
	*deps.BaseDependencies

	tracker *benthosmetrics.Tracker

	// lastGood is the last scrape that succeeded. Poll returns it on a failed
	// scrape, as FSMv1's monitor keeps its last scan.
	lastGood Scrape
}

// Scrape is what the worker stores per poll: what the probe endpoints
// answered and the measured Sample, both from the same successful scrape.
type Scrape struct {
	Probes benthosmetrics.Probes
	Sample benthosmetrics.Sample
}

// Config is what the worker needs to scrape one Benthos instance. The
// framework reads it from the child spec with yaml.Unmarshal, so the key is
// the yaml tag.
type Config struct {
	MetricsPort uint16 `yaml:"metricsPort"`
}

// scrapeClock is the clock NewDeps hands each worker's Tracker when the
// dependency map holds nothing under ClockKey, and the clock mapObserved
// judges freshness by.
var scrapeClock = time.Now

// ClockKey names the clock.Clock the worker stamps each scrape from. It does
// not reach mapObserved, whose freshness check reads scrapeClock.
var ClockKey = fsmv2config.NewDependencyKey[clock.Clock]("benthos_metrics.clock")

// Poll scrapes the configured Benthos instance once and returns the Scrape.
// A failed scrape returns the last good Scrape with the error, so one failed
// scrape does not read as a dead Benthos.
func Poll(ctx context.Context, d *benthosDeps, cfg Config) (Scrape, error) {
	if d == nil || d.tracker == nil {
		return Scrape{}, fmt.Errorf("benthos worker has no Tracker; register the worker with NewDeps")
	}

	probes, sample, err := d.tracker.Scrape(ctx, int(cfg.MetricsPort))
	if err != nil {
		return d.lastGood, err
	}

	d.lastGood = Scrape{Probes: probes, Sample: sample}

	return d.lastGood, nil
}

func init() {
	simple.Register(simple.MonitorSpec[Config, Scrape, *benthosDeps]{
		WorkerType: WorkerType,
		// The library owns the cadence its windows are sized for; the shell
		// polls at it.
		Interval: benthosmetrics.ScrapeInterval,
		NewDeps: func(_ deps.Identity, bd *deps.BaseDependencies, dependencies map[string]any) *benthosDeps {
			now := scrapeClock
			if clk, ok := fsmv2config.LookupDependency(dependencies, ClockKey); ok {
				now = clk.Now
			}

			return &benthosDeps{BaseDependencies: bd, tracker: benthosmetrics.NewTracker(now)}
		},
		Poll: Poll,
	})
}
