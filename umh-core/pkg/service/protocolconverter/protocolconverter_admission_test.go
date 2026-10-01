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

package protocolconverter_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	pkgfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
)

const admissionHint = "agent.enableResourceLimitBlocking: false"

// admissionSnapshot is an instance whose container is active with every
// resource healthy and a CPU limit of 4 cores, so the bridge limit is
// (4 - 1) * 5 = 15. bridges lists the bridge names in config.yaml order, each
// in the given state.
func admissionSnapshot(state string, bridges ...string) pkgfsm.SystemSnapshot {
	pcConfigs := make([]config.ProtocolConverterConfig, 0, len(bridges))
	instances := make(map[string]*pkgfsm.FSMInstanceSnapshot, len(bridges))

	for _, name := range bridges {
		pcConfigs = append(pcConfigs, config.ProtocolConverterConfig{
			FSMInstanceConfig: config.FSMInstanceConfig{Name: name, DesiredFSMState: "active"},
		})
		instances[name] = &pkgfsm.FSMInstanceSnapshot{ID: name, CurrentState: state, DesiredState: "active"}
	}

	return pkgfsm.SystemSnapshot{
		CurrentConfig: config.FullConfig{
			Agent:             config.AgentConfig{EnableResourceLimitBlocking: true},
			ProtocolConverter: pcConfigs,
		},
		Managers: map[string]pkgfsm.ManagerSnapshot{
			constants.ContainerManagerName: &MockManagerSnapshot{
				Instances: map[string]*pkgfsm.FSMInstanceSnapshot{
					constants.CoreInstanceName: {
						ID:           constants.CoreInstanceName,
						CurrentState: "active",
						DesiredState: "active",
						LastObservedState: &container.ContainerObservedStateSnapshot{
							ServiceInfoSnapshot: container_monitor.ServiceInfo{
								OverallHealth: models.Active,
								CPUHealth:     models.Active,
								MemoryHealth:  models.Active,
								DiskHealth:    models.Active,
								CPU:           &models.CPU{CgroupCores: 4},
							},
						},
					},
				},
			},
			constants.ProtocolConverterManagerName: &MockManagerSnapshot{Instances: instances},
		},
	}
}

func bridgeNames(n int) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("bridge-%02d", i+1)
	}

	return names
}

func coreInstance(s pkgfsm.SystemSnapshot) *pkgfsm.FSMInstanceSnapshot {
	return managerInstances(s, constants.ContainerManagerName)[constants.CoreInstanceName]
}

func managerInstances(s pkgfsm.SystemSnapshot, manager string) map[string]*pkgfsm.FSMInstanceSnapshot {
	m, ok := s.Managers[manager]
	if !ok || m == nil {
		panic("test snapshot has no " + manager)
	}

	return m.GetInstances()
}

var _ = Describe("BridgeMustWait admission", func() {
	var service *protocolconverter.ProtocolConverterService

	BeforeEach(func() {
		service = protocolconverter.NewDefaultProtocolConverterService("test")
	})

	Describe("after a restart, with every bridge waiting in to_be_created", func() {
		It("admits the first 15 bridges in config.yaml order and refuses the 16th", func() {
			names := bridgeNames(16)
			snapshot := admissionSnapshot(internalfsm.LifecycleStateToBeCreated, names...)

			for _, name := range names[:15] {
				mustWait, reason := service.BridgeMustWait(snapshot, name)
				Expect(mustWait).To(BeFalse(), "bridge %s: %s", name, reason)
			}

			mustWait, reason := service.BridgeMustWait(snapshot, names[15])
			Expect(mustWait).To(BeTrue())
			Expect(reason).To(ContainSubstring("limit exceeded"))
			Expect(reason).To(ContainSubstring(admissionHint))
		})

		It("uses config.yaml order, not name order", func() {
			names := bridgeNames(16)
			ordered := append([]string{names[15]}, names[:15]...)
			snapshot := admissionSnapshot(internalfsm.LifecycleStateToBeCreated, ordered...)

			mustWait, _ := service.BridgeMustWait(snapshot, names[15])
			Expect(mustWait).To(BeFalse())

			mustWait, _ = service.BridgeMustWait(snapshot, names[14])
			Expect(mustWait).To(BeTrue())
		})
	})

	Describe("with bridges already created", func() {
		It("admits the 15th bridge after 14 are running", func() {
			snapshot := admissionSnapshot("active", bridgeNames(14)...)
			addWaitingBridge(&snapshot, "new-bridge")

			mustWait, reason := service.BridgeMustWait(snapshot, "new-bridge")
			Expect(mustWait).To(BeFalse(), reason)
		})

		It("refuses a 16th bridge after 15 are running", func() {
			snapshot := admissionSnapshot("active", bridgeNames(15)...)
			addWaitingBridge(&snapshot, "new-bridge")

			mustWait, reason := service.BridgeMustWait(snapshot, "new-bridge")
			Expect(mustWait).To(BeTrue())
			Expect(reason).To(ContainSubstring("limit exceeded"))
		})

		It("does not count bridges being removed", func() {
			snapshot := admissionSnapshot(internalfsm.LifecycleStateRemoving, bridgeNames(15)...)
			addWaitingBridge(&snapshot, "new-bridge")

			mustWait, reason := service.BridgeMustWait(snapshot, "new-bridge")
			Expect(mustWait).To(BeFalse(), reason)
		})
	})

	Describe("the CPU limit behind the bridge limit", func() {
		It("reads the fsmv2 CPU worker's capacity without depending on USE_FSMV2_CPU in the config", func() {
			snapshot := admissionSnapshot("active", bridgeNames(15)...)
			addWaitingBridge(&snapshot, "new-bridge")
			// The fsmv2 CPU path fills CPUHealth and leaves CgroupCores empty.
			// USE_FSMV2_CPU is an environment variable and is not in config.yaml,
			// so the per-tick config always reads it as false.
			cpu := coreInstance(snapshot).LastObservedState.(*container.ContainerObservedStateSnapshot).ServiceInfoSnapshot.CPU
			cpu.CgroupCores = 0
			cpu.CPUHealth = &models.CPUHealth{}
			cpu.CPUHealth.CapacityCores = 4
			snapshot.CurrentConfig.Agent.UseFSMv2CPU = false

			mustWait, reason := service.BridgeMustWait(snapshot, "new-bridge")
			Expect(mustWait).To(BeTrue())
			Expect(reason).To(ContainSubstring("15 bridges maximum with 4.0 CPU cores"))
		})
	})

	Describe("before health is proven", func() {
		DescribeTable("refuses, and admits only with enableResourceLimitBlocking false",
			func(mutate func(*pkgfsm.SystemSnapshot)) {
				snapshot := admissionSnapshot(internalfsm.LifecycleStateToBeCreated, "bridge-01")
				mutate(&snapshot)

				mustWait, reason := service.BridgeMustWait(snapshot, "bridge-01")
				Expect(mustWait).To(BeTrue())
				Expect(reason).To(HavePrefix("Resource health not proven yet"))
				Expect(reason).To(ContainSubstring(admissionHint))

				snapshot.CurrentConfig.Agent.EnableResourceLimitBlocking = false
				mustWait, reason = service.BridgeMustWait(snapshot, "bridge-01")
				Expect(mustWait).To(BeFalse(), reason)
			},
			Entry("no container monitor", func(s *pkgfsm.SystemSnapshot) {
				delete(s.Managers, constants.ContainerManagerName)
			}),
			Entry("no container instance", func(s *pkgfsm.SystemSnapshot) {
				s.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{Instances: map[string]*pkgfsm.FSMInstanceSnapshot{}}
			}),
			Entry("container active but not observed yet", func(s *pkgfsm.SystemSnapshot) {
				coreInstance(*s).LastObservedState = nil
			}),
			Entry("container still starting", func(s *pkgfsm.SystemSnapshot) {
				coreInstance(*s).CurrentState = "monitoring_starting"
			}),
			Entry("container degraded right after start, every resource healthy", func(s *pkgfsm.SystemSnapshot) {
				coreInstance(*s).CurrentState = "degraded"
			}),
			Entry("CPU record without a core count", func(s *pkgfsm.SystemSnapshot) {
				coreInstance(*s).LastObservedState.(*container.ContainerObservedStateSnapshot).ServiceInfoSnapshot.CPU = &models.CPU{}
			}),
			Entry("container observed before its first health reading", func(s *pkgfsm.SystemSnapshot) {
				coreInstance(*s).LastObservedState = &container.ContainerObservedStateSnapshot{}
			}),
		)
	})

	It("still names a degraded resource while the container is degraded", func() {
		snapshot := admissionSnapshot(internalfsm.LifecycleStateToBeCreated, "bridge-01")
		core := coreInstance(snapshot)
		core.CurrentState = "degraded"
		observed := core.LastObservedState.(*container.ContainerObservedStateSnapshot)
		observed.ServiceInfoSnapshot.CPUHealth = models.Degraded
		observed.ServiceInfoSnapshot.OverallHealth = models.Degraded
		observed.ServiceInfoSnapshot.CPU.Health = &models.Health{Category: models.Degraded, Message: "cpu is full"}

		mustWait, reason := service.BridgeMustWait(snapshot, "bridge-01")
		Expect(mustWait).To(BeTrue())
		Expect(reason).To(HavePrefix("CPU degraded: cpu is full"))
		Expect(reason).To(ContainSubstring(admissionHint))
	})
})

// addWaitingBridge adds a bridge in to_be_created at the end of config.yaml.
func addWaitingBridge(s *pkgfsm.SystemSnapshot, name string) {
	s.CurrentConfig.ProtocolConverter = append(s.CurrentConfig.ProtocolConverter, config.ProtocolConverterConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{Name: name, DesiredFSMState: "active"},
	})
	managerInstances(*s, constants.ProtocolConverterManagerName)[name] = &pkgfsm.FSMInstanceSnapshot{
		ID: name, CurrentState: internalfsm.LifecycleStateToBeCreated, DesiredState: "active",
	}
}
