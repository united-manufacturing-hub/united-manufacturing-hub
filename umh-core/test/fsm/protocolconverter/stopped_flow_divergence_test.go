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

//go:build test

package protocolconverter_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	dfcfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	protocolconverterfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	protocolconvertersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter/runtime_config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

var _ = Describe("Config divergence of a flow that waits for its connection", func() {
	var (
		instance      *protocolconverterfsm.ProtocolConverterInstance
		mockService   *protocolconvertersvc.MockProtocolConverterService
		componentName string
		ctx           context.Context
		tick          uint64
		mockRegistry  *serviceregistry.Registry
	)

	BeforeEach(func() {
		componentName = "test-pc-stopped-flow-divergence"
		ctx = context.Background()
		tick = 0
		startTime := time.Now()

		instance, mockService, _ = fsmtest.SetupProtocolConverterInstance(componentName, protocolconverterfsm.OperationalStateStopped)
		mockRegistry = serviceregistry.NewMockRegistry()

		var err error
		tick, err = fsmtest.TestProtocolConverterStateTransition(
			ctx, instance, mockService, mockRegistry, componentName,
			internalfsm.LifecycleStateToBeCreated,
			internalfsm.LifecycleStateCreating, 5, tick, startTime)
		Expect(err).NotTo(HaveOccurred())

		mockService.ExistingComponents[componentName] = true
		fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStopped)

		tick, err = fsmtest.TestProtocolConverterStateTransition(
			ctx, instance, mockService, mockRegistry, componentName,
			internalfsm.LifecycleStateCreating,
			protocolconverterfsm.OperationalStateStopped, 5, tick, startTime)
		Expect(err).NotTo(HaveOccurred())

		Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())
		mockService.ServiceExistsResult = true
	})

	It("writes the rendered flow config to the manager once, then reports no divergence", func() {
		rendered, err := runtime_config.BuildRuntimeConfig(
			instance.GetConfig(),
			map[string]string{},
			nil,
			nil,
			runtime_config.BridgedByPlaceholder,
			componentName,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(rendered.DataflowComponentReadServiceConfig.BenthosConfig.Input).NotTo(BeEmpty())

		// A flow kept stopped until the connection is up never writes its benthos config
		// to disk, so the observed read flow stays empty.
		observedOnDisk := rendered
		observedOnDisk.DataflowComponentReadServiceConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}
		mockService.GetConfigResult = observedOnDisk
		mockService.ManagerReadDFC.DesiredFSMState = dfcfsm.OperationalStateStopped

		tick++
		Expect(instance.UpdateObservedStateOfInstance(ctx, mockRegistry, fsm.SystemSnapshot{Tick: tick})).To(Succeed())
		Expect(mockService.UpdateInManagerCalled).To(BeTrue())
		Expect(mockService.ManagerReadDFC.DataFlowComponentServiceConfig).To(Equal(rendered.DataflowComponentReadServiceConfig))

		mockService.UpdateInManagerCalled = false
		tick++
		Expect(instance.UpdateObservedStateOfInstance(ctx, mockRegistry, fsm.SystemSnapshot{Tick: tick})).To(Succeed())
		Expect(instance.ObservedState.ConfigDivergence).To(BeEmpty())
		Expect(mockService.UpdateInManagerCalled).To(BeFalse())
	})
})
