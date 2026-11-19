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

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/historian/timescalemetrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var metricsHierarchyPath = idUnder("parent-a").HierarchyPath

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

var figureGauges = append([]*prometheus.GaugeVec{historianDatabaseOccupiedDiskBytes}, historianSummaryGauges...)

// gaugeValue reads a series that is expected to exist; reading creates it.
func gaugeValue(gauge *prometheus.GaugeVec, labelValues ...string) float64 {
	var metric dto.Metric
	Expect(gauge.WithLabelValues(labelValues...).Write(&metric)).To(Succeed())

	return metric.GetGauge().GetValue()
}

// collectedSeries counts the series one gauge vec would serve on /metrics.
func collectedSeries(gauge *prometheus.GaugeVec) int {
	collected := make(chan prometheus.Metric)

	go func() {
		gauge.Collect(collected)
		close(collected)
	}()

	count := 0
	for range collected {
		count++
	}

	return count
}

func seriesCount(gauges []*prometheus.GaugeVec) int {
	count := 0
	for _, gauge := range gauges {
		count += collectedSeries(gauge)
	}

	return count
}

// The gauges are package globals registered once, so every spec starts and
// ends with them empty.
var _ = BeforeEach(func() {
	CleanupHistorianMetrics()
	DeferCleanup(CleanupHistorianMetrics)
})

var _ = Describe("RecordHistorianStatus", func() {
	It("sets every gauge from a connected status with its figures read", func() {
		RecordHistorianStatus(metricsHierarchyPath, connectedStatus)

		Expect(gaugeValue(historianReachable, metricsHierarchyPath)).To(Equal(1.0))
		Expect(gaugeValue(historianQueryLatencySeconds, metricsHierarchyPath)).To(Equal(connectedStatus.LatencyMs / 1000))
		Expect(gaugeValue(historianOccupiedDiskBytes, metricsHierarchyPath)).To(Equal(float64(fullSummary.HistorianOccupiedDiskBytes)))
		Expect(gaugeValue(historianDataSpanSeconds, metricsHierarchyPath)).To(Equal(float64(fullSummary.DataSpanSeconds)))
		Expect(gaugeValue(historianJobs, metricsHierarchyPath)).To(Equal(float64(fullSummary.JobCount)))
		Expect(gaugeValue(historianFailedJobs, metricsHierarchyPath)).To(Equal(float64(fullSummary.FailedJobCount)))
		Expect(gaugeValue(historianDatabaseOccupiedDiskBytes, metricsHierarchyPath)).To(Equal(float64(fullDatabaseOccupiedDiskBytes)))

		Expect(gaugeValue(historianAuth, metricsHierarchyPath, string(models.TimescaleAuthValid))).To(Equal(1.0))
		Expect(gaugeValue(historianAuth, metricsHierarchyPath, string(models.TimescaleAuthInvalid))).To(Equal(0.0))
		Expect(gaugeValue(historianAuth, metricsHierarchyPath, string(models.TimescaleAuthUnknown))).To(Equal(0.0))
	})

	It("exports the connection gauges but no figures before the first read", func() {
		RecordHistorianStatus(metricsHierarchyPath, connectedStatusBeforeFirstRead)

		Expect(collectedSeries(historianReachable)).To(Equal(1))
		Expect(collectedSeries(historianQueryLatencySeconds)).To(Equal(1))
		Expect(collectedSeries(historianAuth)).To(Equal(len(authStates)))
		Expect(seriesCount(figureGauges)).To(BeZero())
	})

	It("deletes the figure series after a failed connection check", func() {
		RecordHistorianStatus(metricsHierarchyPath, connectedStatus)
		Expect(seriesCount(figureGauges)).To(Equal(len(figureGauges)), "the full status exported every figure")

		RecordHistorianStatus(metricsHierarchyPath, failedCheckStatus)

		Expect(seriesCount(figureGauges)).To(BeZero())
		Expect(collectedSeries(historianQueryLatencySeconds)).To(BeZero(),
			"a failed check measures no latency, and a 0 would read as a fast database")
		Expect(gaugeValue(historianReachable, metricsHierarchyPath)).To(Equal(0.0))
		Expect(gaugeValue(historianAuth, metricsHierarchyPath, string(models.TimescaleAuthUnknown))).To(Equal(1.0))
	})
})

var _ = Describe("CleanupHistorianMetrics", func() {
	It("deletes every historian series", func() {
		RecordHistorianStatus(metricsHierarchyPath, connectedStatus)
		Expect(seriesCount(historianGauges)).NotTo(BeZero())

		CleanupHistorianMetrics()

		Expect(seriesCount(historianGauges)).To(BeZero())
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
		status, err := pollAndRecord(context.Background(), pollDeps(), historianConfig)

		Expect(err).NotTo(HaveOccurred())
		Expect(gaugeValue(historianReachable, metricsHierarchyPath)).To(Equal(1.0))
		Expect(gaugeValue(historianJobs, metricsHierarchyPath)).To(Equal(float64(status.Summary.JobCount)))
		Expect(gaugeValue(historianDatabaseOccupiedDiskBytes, metricsHierarchyPath)).To(Equal(float64(*status.DatabaseOccupiedDiskBytes)))
	})

	It("records a failed connection check", func() {
		fakeDatabase.SetErr(timescalemetrics.ErrFakeConnectionRefused)

		_, _ = pollAndRecord(context.Background(), pollDeps(), historianConfig)

		Expect(gaugeValue(historianReachable, metricsHierarchyPath)).To(Equal(0.0))
		Expect(seriesCount(figureGauges)).To(BeZero())
	})
})
