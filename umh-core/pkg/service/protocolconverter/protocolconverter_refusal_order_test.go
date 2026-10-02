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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	pkgfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
)

// A refusal names the resource that caused it, even while the container's own
// state is degraded. Every combination of container state and resource health
// is checked: a bridge is refused when the container is degraded or any
// resource is, and the reason names the first degraded resource.
var _ = Describe("BridgeMustWait refusal reasons", func() {
	hostCores := 8

	health := func(degraded bool) models.HealthCategory {
		if degraded {
			return models.Degraded
		}

		return models.Active
	}

	withMessage := func(degraded bool, message string) *models.Health {
		if !degraded {
			return nil
		}

		return &models.Health{Message: message, Category: models.Degraded}
	}

	// refusedWhenDegradedOrAnyResource is written out rather than derived from
	// the code. No bridges are staged, so the bridge-count ceiling never refuses here.
	refusedWhenDegradedOrAnyResource := func(stateDegraded, cpu, mem, disk bool) bool {
		return stateDegraded || cpu || mem || disk
	}

	for _, stateDegraded := range []bool{false, true} {
		for _, cpu := range []bool{false, true} {
			for _, mem := range []bool{false, true} {
				for _, disk := range []bool{false, true} {
					state := "active"
					if stateDegraded {
						state = "degraded"
					}

					It(fmt.Sprintf("state=%s cpu=%v memory=%v disk=%v", state, cpu, mem, disk), func() {
						overall := health(cpu || mem || disk)

						snapshot := pkgfsm.SystemSnapshot{
							Managers: map[string]pkgfsm.ManagerSnapshot{
								constants.ContainerManagerName: &MockManagerSnapshot{
									Instances: map[string]*pkgfsm.FSMInstanceSnapshot{
										constants.CoreInstanceName: {
											ID:           constants.CoreInstanceName,
											CurrentState: state,
											DesiredState: "active",
											LastObservedState: &container.ContainerObservedStateSnapshot{
												ServiceInfoSnapshot: container_monitor.ServiceInfo{
													OverallHealth: overall,
													CPUHealth:     health(cpu),
													MemoryHealth:  health(mem),
													DiskHealth:    health(disk),
													CPU:           &models.CPU{Health: withMessage(cpu, "cpu is full"), CoreCount: &hostCores},
													Memory:        &models.Memory{Health: withMessage(mem, "memory is full")},
													Disk:          &models.Disk{Health: withMessage(disk, "disk is full")},
												},
											},
										},
									},
								},
							},
							CurrentConfig: config.FullConfig{Agent: config.AgentConfig{EnableResourceLimitBlocking: true}},
						}

						mustWait, reason := protocolconverter.NewDefaultProtocolConverterService("test").BridgeMustWait(snapshot, "new-bridge")

						Expect(mustWait).To(Equal(refusedWhenDegradedOrAnyResource(stateDegraded, cpu, mem, disk)),
							"a bridge must be refused when the container or any resource is degraded")

						switch {
						case cpu:
							Expect(reason).To(HavePrefix("CPU degraded: cpu is full"))
						case mem:
							Expect(reason).To(HavePrefix("Memory degraded: memory is full"))
						case disk:
							Expect(reason).To(HavePrefix("Disk degraded: disk is full"))
						case stateDegraded:
							Expect(reason).To(HavePrefix("Resource health not proven yet"),
								"with no resource degraded the unproven health is the only cause left to name")
						default:
							Expect(reason).To(BeEmpty())
						}
					})
				}
			}
		}
	}

	// A degraded resource with no message still names itself, with the generic
	// reason. Each entry also sets the container's own state to degraded.
	DescribeTable("names a degraded resource without a message while the container is degraded",
		func(info container_monitor.ServiceInfo, reason string) {
			snapshot := pkgfsm.SystemSnapshot{
				Managers: map[string]pkgfsm.ManagerSnapshot{
					constants.ContainerManagerName: &MockManagerSnapshot{
						Instances: map[string]*pkgfsm.FSMInstanceSnapshot{
							constants.CoreInstanceName: {
								ID:                constants.CoreInstanceName,
								CurrentState:      "degraded",
								DesiredState:      "active",
								LastObservedState: &container.ContainerObservedStateSnapshot{ServiceInfoSnapshot: info},
							},
						},
					},
				},
				CurrentConfig: config.FullConfig{Agent: config.AgentConfig{EnableResourceLimitBlocking: true}},
			}

			mustWait, got := protocolconverter.NewDefaultProtocolConverterService("test").BridgeMustWait(snapshot, "new-bridge")

			Expect(mustWait).To(BeTrue(), "a degraded container state must refuse")
			Expect(got).To(HavePrefix(reason))
		},
		Entry("CPU degraded with an empty message",
			container_monitor.ServiceInfo{
				OverallHealth: models.Degraded, CPUHealth: models.Degraded, MemoryHealth: models.Active, DiskHealth: models.Active,
				CPU: &models.CPU{Health: &models.Health{Category: models.Degraded}},
			},
			"CPU resources degraded"),
		Entry("memory degraded with an empty message",
			container_monitor.ServiceInfo{
				OverallHealth: models.Degraded, CPUHealth: models.Active, MemoryHealth: models.Degraded, DiskHealth: models.Active,
				Memory: &models.Memory{Health: &models.Health{Category: models.Degraded}},
			},
			"Memory resources degraded"),
		Entry("disk degraded with an empty message",
			container_monitor.ServiceInfo{
				OverallHealth: models.Degraded, CPUHealth: models.Active, MemoryHealth: models.Active, DiskHealth: models.Degraded,
				Disk: &models.Disk{Health: &models.Health{Category: models.Degraded}},
			},
			"Disk resources degraded"),
	)
})
