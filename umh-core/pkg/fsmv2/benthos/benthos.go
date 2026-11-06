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

// Package fsmv2benthos is the FSMv2 simple monitor that scrapes each Benthos
// instance's metrics through pkg/benthosmetrics. Importing it registers the
// "benthos_metrics" worker; NewFsmv2BenthosMetricsManager runs one worker per
// enabled BenthosMonitorConfig.
package fsmv2benthos

import (
	"context"
	"fmt"
	"time"

	"github.com/benbjohnson/clock"

	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/benthosmetrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

// WorkerType is the worker-type name this monitor registers under in config
// and CSE storage.
const WorkerType = "benthos_metrics"

// benthosDeps is one worker's dependencies: the Tracker holding this Benthos
// instance's scrape state. The framework creates and drops it with the worker.
type benthosDeps struct {
	*deps.BaseDependencies

	tracker *benthosmetrics.Tracker

	// lastGood is the last sample whose scrape succeeded. Poll returns it on
	// a failed scrape, as FSMv1's monitor keeps its last scan.
	lastGood benthosmetrics.Sample
}

// scrapeClock is the clock NewDeps hands each worker's Tracker when the
// dependency map holds nothing under ClockKey, and the clock mapObserved
// judges freshness by.
var scrapeClock = time.Now

// ClockKey names the clock.Clock the worker stamps each scrape from. It does
// not reach mapObserved, whose freshness check reads scrapeClock.
var ClockKey = fsmv2config.NewDependencyKey[clock.Clock]("benthos_metrics.clock")

// Poll scrapes the configured Benthos instance once and returns the Sample.
// A failed scrape returns the last good Sample with the error, so one failed
// scrape does not read as a dead Benthos.
func Poll(ctx context.Context, d *benthosDeps, cfg config.BenthosMonitorConfig) (benthosmetrics.Sample, error) {
	if d == nil || d.tracker == nil {
		return benthosmetrics.Sample{}, fmt.Errorf("benthos worker has no Tracker; register the worker with NewDeps")
	}

	sample, err := d.tracker.Scrape(ctx, int(cfg.MetricsPort))
	if err != nil {
		return d.lastGood, err
	}

	d.lastGood = sample

	return sample, nil
}

func init() {
	simple.Register(simple.MonitorSpec[config.BenthosMonitorConfig, benthosmetrics.Sample, *benthosDeps]{
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
