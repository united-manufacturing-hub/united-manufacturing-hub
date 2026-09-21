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

package generator_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/channelusage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/generator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

func monitorAt(fillPercent int) *channelusage.Monitor {
	monitor, err := channelusage.NewMonitor()
	Expect(err).NotTo(HaveOccurred())

	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for range int(channelusage.Window / channelusage.SampleInterval) {
		monitor.Observe(fillPercent, 100, at)
		at = at.Add(channelusage.SampleInterval)
	}

	return monitor
}

var _ = Describe("CommunicatorFromMonitor", func() {
	DescribeTable("reports health from the monitor verdict",
		func(monitor func() *channelusage.Monitor, fillPercent float64, expected models.HealthCategory) {
			communicator := generator.CommunicatorFromMonitor(monitor(), 2)
			Expect(communicator.SubscriberCount).To(Equal(2))
			Expect(communicator.OutboundChannelFillPercent).To(BeNumerically("~", fillPercent, 0.001))
			Expect(communicator.Health.Category).To(Equal(expected))
		},
		Entry("no monitor", func() *channelusage.Monitor { return nil }, 0.0, models.Neutral),
		Entry("healthy queue", func() *channelusage.Monitor { return monitorAt(40) }, 40.0, models.Active),
		Entry("full queue", func() *channelusage.Monitor { return monitorAt(100) }, 100.0, models.Degraded),
	)
})
