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

package generator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	fsmv2historian "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/historian"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var healthyTimescaleStatus = simple.Status[fsmv2historian.TimescaleStatus]{
	Reason: "running",
	Result: fsmv2historian.TimescaleStatus{
		Host:      "timescale.internal",
		Port:      5432,
		Auth:      models.TimescaleAuthValid,
		LatencyMs: 2.5,
		Reachable: true,
		TimescaleMetrics: fsmv2historian.TimescaleMetrics{
			ServerVersion:        "17.7",
			TimescaleVersion:     "2.24.0",
			DatabaseBytes:        917000000,
			UncompressedBytes:    460849152,
			CompressedBytes:      276061440,
			Hypertables:          40,
			Chunks:               7112,
			CompressedChunks:     7032,
			Jobs:                 80,
			CompressionJobs:      40,
			RetentionJobs:        40,
			CompressAfterSeconds: 604800,
			DropAfterSeconds:     2592000,
			PoliciesUniform:      true,
			FailedJobs:           1,
			LastJobError:         "columnstore policy failure",
			Tables: []fsmv2historian.TimescaleTable{
				{Name: "value_bench", Chunks: 105, CompressedChunks: 103, ChunkIntervalSeconds: 604800},
			},
		},
	},
}

var _ = Describe("Historian status mapping", func() {
	It("carries the connection fields through", func() {
		historian := historianFromStatus(healthyTimescaleStatus, fsmv2client.Fresh)

		Expect(historian.Timescale.Host).To(Equal("timescale.internal"))
		Expect(historian.Timescale.Port).To(Equal(uint16(5432)))
		Expect(historian.Timescale.Reachable).To(BeTrue())
		Expect(historian.Timescale.Health.Category).To(Equal(models.Active))
	})

	It("carries the database metrics through", func() {
		historian := historianFromStatus(healthyTimescaleStatus, fsmv2client.Fresh)

		Expect(historian.Timescale.ServerVersion).To(Equal("17.7"))
		Expect(historian.Timescale.TimescaleVersion).To(Equal("2.24.0"))
		Expect(historian.Timescale.DatabaseBytes).To(Equal(int64(917000000)))
		Expect(historian.Timescale.Hypertables).To(Equal(40))
		Expect(historian.Timescale.Chunks).To(Equal(7112))
		Expect(historian.Timescale.CompressedChunks).To(Equal(7032))
		Expect(historian.Timescale.UncompressedBytes).To(Equal(int64(460849152)))
		Expect(historian.Timescale.CompressedBytes).To(Equal(int64(276061440)))
		Expect(historian.Timescale.FailedJobs).To(Equal(1))
	})

	It("reports a stale observation as degraded", func() {
		historian := historianFromStatus(healthyTimescaleStatus, fsmv2client.Stale)

		Expect(historian.Timescale.Health.Category).To(Equal(models.Degraded))
		Expect(historian.Timescale.Health.Message).To(Equal("historian monitor observation is stale"))
	})

	It("still reports the last metrics when the connection is degraded", func() {
		degraded := healthyTimescaleStatus
		degraded.Degraded = true
		degraded.Reason = "poll error: connection refused"

		historian := historianFromStatus(degraded, fsmv2client.Fresh)

		Expect(historian.Timescale.Health.Category).To(Equal(models.Degraded))
		Expect(historian.Timescale.Health.Message).To(Equal("poll error: connection refused"))
		Expect(historian.Timescale.Hypertables).To(Equal(40), "the last known metrics survive a degraded tick")
	})
})

var _ = Describe("Historian policy and per-table mapping", func() {
	It("carries the policy aggregates through", func() {
		historian := historianFromStatus(healthyTimescaleStatus, fsmv2client.Fresh)

		Expect(historian.Timescale.CompressionJobs).To(Equal(40))
		Expect(historian.Timescale.RetentionJobs).To(Equal(40))
		Expect(historian.Timescale.CompressAfterSeconds).To(Equal(int64(604800)))
		Expect(historian.Timescale.DropAfterSeconds).To(Equal(int64(2592000)))
		Expect(historian.Timescale.PoliciesUniform).To(BeTrue())
		Expect(historian.Timescale.LastJobError).To(Equal("columnstore policy failure"))
	})

	It("carries the per-table entries through", func() {
		historian := historianFromStatus(healthyTimescaleStatus, fsmv2client.Fresh)

		Expect(historian.Timescale.Tables).To(HaveLen(1))
		Expect(historian.Timescale.Tables[0].Name).To(Equal("value_bench"))
		Expect(historian.Timescale.Tables[0].Chunks).To(Equal(105))
		Expect(historian.Timescale.Tables[0].ChunkIntervalSeconds).To(Equal(int64(604800)))
	})
})
