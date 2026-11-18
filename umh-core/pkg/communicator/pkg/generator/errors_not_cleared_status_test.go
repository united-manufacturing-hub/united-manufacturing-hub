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

// The not-cleared count's place in the status message.

package generator

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/benthosmetrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	communicatortopicbrowser "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// snapshotWithOneDFC builds the snapshot the spec reads: one DFC manager
// holding one instance, so GenerateStatusMessage maps a full message.
func snapshotWithOneDFC() fsm.SystemSnapshot {
	return fsm.SystemSnapshot{
		SnapshotTime: time.Now(),
		Managers: map[string]fsm.ManagerSnapshot{
			constants.DataflowcomponentManagerName: &fsm.BaseManagerSnapshot{
				Name:         constants.DataflowcomponentManagerName,
				Instances:    map[string]*fsm.FSMInstanceSnapshot{"fixture-dfc": {}},
				SnapshotTime: time.Now(),
			},
		},
	}
}

var _ = Describe("The not-cleared count in the status message", func() {
	// The status message's feature usage part carries how many registered
	// Benthos workers hold an error that did not clear in time under
	// benthosErrorsNotClearedInstanceCount.
	It("reports the not-cleared instance count", func() {
		logger := zap.NewNop().Sugar()

		snapshotManager := fsm.NewSnapshotManager()
		snap := snapshotWithOneDFC()
		snapshotManager.UpdateSnapshot(&snap)

		collector := NewStatusCollector(
			&watchdog.Watchdog{},
			snapshotManager,
			config.NewMockConfigManager(),
			logger,
			communicatortopicbrowser.NewTopicBrowserCommunicator(logger),
			&models.FeatureUsage{},
			func() int { return 0 },
			nil,
		)

		// With the flag off the count stays 0, whatever the store answers.
		setBenthosMetricsFlag("")
		registerBenthosWorkers(map[string]*benthosmetrics.Failures{
			dfcBenthosName: stuckOutputError(time.Now()),
		})

		status := collector.GenerateStatusMessage(context.Background(), true)
		Expect(status).NotTo(BeNil())
		Expect(status.Core.FeatureUsage).NotTo(BeNil())
		Expect(status.Core.FeatureUsage.BenthosErrorsNotClearedInstanceCount).To(BeZero(),
			"with the flag off the count stays 0")

		// With the flag on and a stuck instance in the registry, the count
		// reaches the status message.
		setBenthosMetricsFlag("true")

		status = collector.GenerateStatusMessage(context.Background(), true)
		Expect(status).NotTo(BeNil())
		Expect(status.Core.FeatureUsage).NotTo(BeNil())
		Expect(status.Core.FeatureUsage.BenthosErrorsNotClearedInstanceCount).To(Equal(1),
			"the registry's not-cleared instance count")
	})
})
