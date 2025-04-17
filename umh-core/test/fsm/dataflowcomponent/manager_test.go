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

package dataflowcomponent_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	dataflowcomponentfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	dataflowcomponentsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// Make the constants available directly
const (
	OperationalStateStopped = "stopped"
)

// Following the CursorRules, we never call manager.Reconcile(...) directly in loops.
// Instead, we use the fsmtest helpers.

var _ = Describe("DataflowComponentManager", func() {
	var (
		manager     *dataflowcomponentfsm.DataflowComponentManager
		mockService *dataflowcomponentsvc.MockDataFlowComponentService
		ctx         context.Context
		tick        uint64
		cancel      context.CancelFunc
		mockFS      *filesystem.MockFileSystem
	)

	AfterEach(func() {
		cancel()
	})

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Minute) // we need to have a deadline as the reconcile logic in the base fsm manager requires it
		tick = 0
		mockFS = filesystem.NewMockFileSystem()
		// Create a new DataflowComponentManager with the mock service
		manager, mockService = fsmtest.CreateMockDataflowComponentManager("test-manager")

		// Initialize the mock service state to empty
		mockService.ExistingComponents = make(map[string]bool)
		mockService.ComponentStates = make(map[string]*dataflowcomponentsvc.ServiceInfo)
	})

	// -------------------------------------------------------------------------
	//  INITIALIZATION
	// -------------------------------------------------------------------------
	Context("Initialization", func() {
		It("should handle empty config without errors", func() {
			emptyConfig := config.FullConfig{DataFlow: []config.DataFlowComponentConfig{}}

			// Single call to a helper that wraps Reconcile
			newTick, err := fsmtest.WaitForDataflowComponentManagerStable(
				ctx, fsm.SystemSnapshot{CurrentConfig: emptyConfig, Tick: tick}, manager, mockFS,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetInstances()).To(BeEmpty())
		})

		It("should create a service in stopped state and remain stable", func() {
			componentName := "test-stopped-component"
			cfg := config.FullConfig{
				DataFlow: []config.DataFlowComponentConfig{
					fsmtest.CreateDataflowComponentTestConfig(componentName, OperationalStateStopped),
				},
			}

			// Configure the mock service to allow transition to Stopped
			fsmtest.ConfigureDataflowComponentManagerForState(mockService, componentName, OperationalStateStopped)

			// Wait for instance creation and stable 'Stopped' state
			newTick, err := fsmtest.WaitForDataflowComponentManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick},
				manager,
				mockFS,
				componentName,
				OperationalStateStopped,
				10,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Double-check the manager state
			inst, exists := manager.GetInstance(componentName)
			Expect(exists).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(OperationalStateStopped))
		})
	})
})
