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
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	dfcfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	dfcsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
)

var _ = Describe("GenerateStatusMessage feature usage", func() {
	It("reports the stale metric degraded instance count of the snapshot", func() {
		snapshots := fsm.NewSnapshotManager()
		snapshots.UpdateSnapshot(&fsm.SystemSnapshot{Managers: map[string]fsm.ManagerSnapshot{
			constants.DataflowcomponentManagerName: manager(constants.DataflowcomponentManagerName, map[string]fsm.ObservedStateSnapshot{
				"dfc-stale": &dfcfsm.DataflowComponentObservedStateSnapshot{ServiceInfo: dfcsvc.ServiceInfo{BenthosObservedState: benthosWith(verdictObservedAfter(12*time.Minute, true))}},
			}),
		}})
		logger := zap.NewNop().Sugar()
		collector := NewStatusCollector(nil, snapshots, config.NewMockConfigManager(), logger,
			topicbrowser.NewTopicBrowserCommunicator(logger), &models.FeatureUsage{}, func() int { return 0 })

		msg := collector.GenerateStatusMessage(context.Background(), true)

		Expect(msg).NotTo(BeNil())
		Expect(msg.Core.FeatureUsage).NotTo(BeNil())
		Expect(msg.Core.FeatureUsage.StaleMetricDegradedInstanceCount).To(Equal(1))
	})
})
