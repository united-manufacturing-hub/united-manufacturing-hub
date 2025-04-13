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

package dataflowcomponent

import (
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
)

func NewDataflowComponentManagerWitMockedServices(name string) (*DataflowComponentManager, *dataflowcomponent.MockDataFlowComponentService) {
	managerName := fmt.Sprintf("%s%s", logger.ComponentDataFlowComponentManager, name)
	mockService := dataflowcomponent.NewMockDataFlowComponentService()

	if mockService.ExistingComponents == nil {
		mockService.ExistingComponents = make(map[string]bool)
	}

	if mockService.ComponentStates == nil {
		mockService.ComponentStates = make(map[string]*dataflowcomponent.ServiceInfo)
	}

	baseManager := public_fsm.NewBaseFSMManager[config.DataFlowComponentConfig](
		managerName,
		"/dev/null",
		func(fullConfig config.FullConfig) ([]config.DataFlowComponentConfig, error) {
			return []config.DataFlowComponentConfig{}, nil
		},
		// Get name of DFC c
		func(cfg config.DataFlowComponentConfig) (string, error) {
			return cfg.Name, nil
		},
		func(cfg config.DataFlowComponentConfig) (string, error) {
			return cfg.DesiredFSMState, nil
		},
		func(cfg config.DataFlowComponentConfig) (public_fsm.FSMInstance, error) {
			return NewDataflowComponentInstance("/dev/null", cfg), nil
		},
		func(instance public_fsm.FSMInstance, cfg config.DataFlowComponentConfig) (bool, error) {
			return false, nil
		},
		func(instance public_fsm.FSMInstance, cfg config.DataFlowComponentConfig) error {
			return nil
		},
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			return constants.DataflowComponentExpectedMaxP95ExecutionTimePerInstance, nil
		},
	)

	metrics.InitErrorCounter(metrics.ComponentDataFlowCompManager, name)
	manager := &DataflowComponentManager{
		BaseFSMManager: baseManager,
	}
	return manager, mockService
}
