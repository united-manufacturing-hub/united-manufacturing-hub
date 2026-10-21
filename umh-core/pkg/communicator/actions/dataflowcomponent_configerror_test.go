// Copyright 2026 UMH Systems GmbH
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

package actions_test

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

var _ = Describe("Stand-alone flow with a benthos config error", func() {
	const flowName = "rejected-flow"

	var (
		mockCfg     *config.MockConfigManager
		stateMocker *actions.StateMocker
		outbound    chan *models.UMHMessage
		msgs        []*models.UMHMessage
		mu          sync.Mutex
	)

	flowPayload := func(port int) map[string]any {
		return map[string]any{
			"name":  flowName,
			"uuid":  dataflowcomponentserviceconfig.GenerateUUIDFromName(flowName).String(),
			"meta":  map[string]any{"type": "custom"},
			"state": "active",
			"payload": map[string]any{
				"customDataFlowComponent": map[string]any{
					"inputs": map[string]any{
						"type": "yaml",
						"data": fmt.Sprintf("type: http_server\nhttp_server:\n  path: /input\n  port: %d", port),
					},
					"outputs": map[string]any{"type": "yaml", "data": "type: stdout"},
					"pipeline": map[string]any{
						"processors": map[string]any{
							"0": map[string]any{"type": "yaml", "data": "type: mapping\nprocs: []"},
						},
					},
				},
			},
		}
	}

	rejectFlowConfig := func() {
		stateMocker.RejectedConfigLogs[flowName] = []s6.LogEntry{{Timestamp: time.Now().Add(time.Minute), Content: lintErrorLine}}
	}

	BeforeEach(func() {
		outbound = make(chan *models.UMHMessage, 200)

		mu.Lock()
		msgs = nil
		mu.Unlock()

		go actions.ConsumeOutboundMessages(outbound, &msgs, &mu, false)
	})

	AfterEach(func() {
		stateMocker.Stop()
		close(outbound)
	})

	It("removes a deployed flow that benthos rejects and names the lint message", func() {
		mockCfg = config.NewMockConfigManager().WithConfig(config.FullConfig{
			Agent: config.AgentConfig{MetricsPort: 8080},
		})
		stateMocker = actions.NewStateMocker(mockCfg)
		stateMocker.Tick()
		rejectFlowConfig()

		action := actions.NewDeployDataflowComponentAction("probe@example.com", uuid.New(), uuid.New(), outbound, mockCfg, stateMocker.GetStateManager())
		Expect(action.Parse(flowPayload(8000))).To(Succeed())
		Expect(stateMocker.Start()).To(Succeed())

		_, _, err := action.Execute()

		Expect(err).To(MatchError(ContainSubstring("was removed because of a configuration error")))
		Expect(err).To(MatchError(ContainSubstring(lintErrorText)))
	})

	It("rolls back an edited flow that benthos rejects and names the lint message", func() {
		mockCfg = config.NewMockConfigManager().WithConfig(config.FullConfig{
			Agent: config.AgentConfig{MetricsPort: 8080},
			DataFlow: []config.DataFlowComponentConfig{{
				FSMInstanceConfig: config.FSMInstanceConfig{Name: flowName, DesiredFSMState: "active"},
				DataFlowComponentServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Input:  map[string]any{"type": "http_server", "http_server": map[string]any{"path": "/input", "port": 8000}},
						Output: map[string]any{"type": "stdout"},
					},
				},
			}},
		})
		stateMocker = actions.NewStateMocker(mockCfg)
		stateMocker.Tick()
		rejectFlowConfig()

		action := actions.NewEditDataflowComponentAction("probe@example.com", uuid.New(), uuid.New(), outbound, mockCfg, stateMocker.GetStateManager())
		Expect(action.Parse(flowPayload(8001))).To(Succeed())
		Expect(stateMocker.Start()).To(Succeed())

		_, _, err := action.Execute()

		Expect(err).To(MatchError(ContainSubstring("was rolled back to its previous configuration because of a configuration error")))
		Expect(err).To(MatchError(ContainSubstring(lintErrorText)))
	})
})
