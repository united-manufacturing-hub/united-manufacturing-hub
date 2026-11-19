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

package fsmv2timescale

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/historian/timescalemetrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var fullSummary = timescalemetrics.Summary{
	HistorianOccupiedDiskBytes: 4096,
	DataSpanSeconds:            86400,
	JobCount:                   3,
	FailedJobCount:             1,
}

var fullDatabaseOccupiedDiskBytes int64 = 8192

var connectedStatus = TimescaleStatus{
	Host:                      "db",
	Port:                      5432,
	Auth:                      models.TimescaleAuthValid,
	LatencyMs:                 12.5,
	Reachable:                 true,
	Summary:                   &fullSummary,
	DatabaseOccupiedDiskBytes: &fullDatabaseOccupiedDiskBytes,
}

var connectedStatusBeforeFirstRead = TimescaleStatus{
	Host:      "db",
	Port:      5432,
	Auth:      models.TimescaleAuthValid,
	LatencyMs: 12.5,
	Reachable: true,
}

var failedCheckStatus = TimescaleStatus{
	Host: "db",
	Port: 5432,
	Auth: models.TimescaleAuthUnknown,
}

var figureGauges = []deps.GaugeName{
	GaugeHistorianOccupiedDiskBytes,
	GaugeHistorianDataSpanSeconds,
	GaugeHistorianJobs,
	GaugeHistorianFailedJobs,
	GaugeHistorianDatabaseOccupiedDiskBytes,
}

// recordedGauges records status and returns the gauges the collector would drain.
func recordedGauges(status TimescaleStatus) map[string]float64 {
	recorder := deps.NewMetricsRecorder()
	recordStatus(recorder, status)

	return recorder.Drain().Gauges
}

func gaugeKeys(names []deps.GaugeName) []string {
	keys := make([]string, 0, len(names))
	for _, name := range names {
		keys = append(keys, string(name))
	}

	return keys
}

var _ = Describe("recordStatus", func() {
	It("sets every gauge from a connected status with its figures read", func() {
		gauges := recordedGauges(connectedStatus)

		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianReachable), 1.0))
		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianAuthValid), 1.0))
		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianAuthInvalid), 0.0))
		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianAuthUnknown), 0.0))
		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianQueryLatencySeconds), connectedStatus.LatencyMs/1000))
		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianSummaryReady), 1.0))
		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianOccupiedDiskBytes), float64(fullSummary.HistorianOccupiedDiskBytes)))
		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianDataSpanSeconds), float64(fullSummary.DataSpanSeconds)))
		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianJobs), float64(fullSummary.JobCount)))
		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianFailedJobs), float64(fullSummary.FailedJobCount)))
		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianDatabaseSizeReady), 1.0))
		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianDatabaseOccupiedDiskBytes), float64(fullDatabaseOccupiedDiskBytes)))
	})

	It("sets the ready flags to 0 and no figures before the first read", func() {
		gauges := recordedGauges(connectedStatusBeforeFirstRead)

		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianSummaryReady), 0.0))
		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianDatabaseSizeReady), 0.0))
		Expect(gauges).NotTo(HaveKey(BeElementOf(gaugeKeys(figureGauges))))
	})

	It("sets no latency and no figures after a failed connection check", func() {
		gauges := recordedGauges(failedCheckStatus)

		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianReachable), 0.0))
		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianAuthUnknown), 1.0))
		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianSummaryReady), 0.0))
		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianDatabaseSizeReady), 0.0))
		Expect(gauges).NotTo(HaveKey(string(GaugeHistorianQueryLatencySeconds)),
			"a failed check measures no latency, and a 0 would read as a fast database")
		Expect(gauges).NotTo(HaveKey(BeElementOf(gaugeKeys(figureGauges))))
	})
})

var _ = Describe("pollAndRecord", func() {
	var fakeDatabase *timescalemetrics.FakeDatabase

	pollDeps := func() Deps {
		return Deps{
			BaseDependencies: baseUnder("parent-a"),
			pool:             &poolHolder{},
			database:         fakeDatabase,
			summary:          &readCache[timescalemetrics.Summary]{interval: summaryInterval},
			databaseSize:     &readCache[int64]{interval: databaseSizeInterval},
		}
	}

	historianConfig := config.HistorianConfig{Timescale: config.TimescaleConfig{Host: "db", Password: "secret"}}

	BeforeEach(func() {
		fakeDatabase = timescalemetrics.NewFakeDatabase(timescalemetrics.FakeContents{
			PostgresVersion:           "17.2",
			TimescaleVersion:          "2.19.0",
			JobCount:                  3,
			FailedJobCount:            1,
			DatabaseOccupiedDiskBytes: fullDatabaseOccupiedDiskBytes,
		})
	})

	It("records the status Poll returned", func() {
		d := pollDeps()

		status, err := pollAndRecord(context.Background(), d, historianConfig)

		Expect(err).NotTo(HaveOccurred())

		gauges := d.MetricsRecorder().Drain().Gauges
		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianReachable), 1.0))
		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianJobs), float64(status.Summary.JobCount)))
		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianDatabaseOccupiedDiskBytes), float64(*status.DatabaseOccupiedDiskBytes)))
	})

	It("records a failed connection check", func() {
		fakeDatabase.SetErr(timescalemetrics.ErrFakeConnectionRefused)
		d := pollDeps()

		_, err := pollAndRecord(context.Background(), d, historianConfig)

		Expect(err).NotTo(HaveOccurred())
		Expect(d.MetricsRecorder().Drain().Gauges).To(HaveKeyWithValue(string(GaugeHistorianReachable), 0.0))
	})

	It("records an unreachable status when the pool cannot be built", func() {
		cfg := config.HistorianConfig{Timescale: config.TimescaleConfig{
			Host:    "127.0.0.1",
			SSLMode: "not-a-mode",
		}}
		d := isolatedPoolDeps()

		_, err := pollAndRecord(context.Background(), d, cfg)

		Expect(err).To(HaveOccurred())

		gauges := d.MetricsRecorder().Drain().Gauges
		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianReachable), 0.0))
		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianAuthUnknown), 1.0))
		Expect(gauges).To(HaveKeyWithValue(string(GaugeHistorianSummaryReady), 0.0))
	})

	It("records nothing when the context is cancelled", func() {
		cfg := config.HistorianConfig{Timescale: config.TimescaleConfig{
			Host:    "127.0.0.1",
			Port:    closedPort(),
			SSLMode: config.HistorianSSLModeDisable,
		}}
		d := isolatedPoolDeps()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := pollAndRecord(ctx, d, cfg)

		Expect(err).To(MatchError(context.Canceled))
		Expect(d.MetricsRecorder().Drain().Gauges).To(BeEmpty())
	})
})

// isolatedPoolDeps builds Deps with a pool holder of their own, so a spec does
// not close a pool that sharedPool holds for another spec.
func isolatedPoolDeps() Deps {
	holder := &poolHolder{}
	DeferCleanup(closeTestPool, holder)

	return Deps{
		BaseDependencies: baseUnder("parent-a"),
		pool:             holder,
		summary:          &readCache[timescalemetrics.Summary]{interval: summaryInterval},
		databaseSize:     &readCache[int64]{interval: databaseSizeInterval},
	}
}

func closeTestPool(holder *poolHolder) {
	if holder.pool != nil {
		holder.pool.Close()
	}
}
