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

// A refusal names the resource that caused it. A degraded resource also
// degrades the container's own state, so the resource checks have to come
// before the state check or every refusal reads "System in degraded state".
// Moving them must not change admission, so every combination of container
// state and resource health is checked against the decision the old order made.
var _ = Describe("IsResourceLimited refusal reasons", func() {
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

	// oldRefused is the admission decision of the order before this change,
	// written out rather than derived from the code: a degraded container state
	// refused first, and any degraded resource refused after it. No bridges are
	// staged, so the bridge-count ceiling never refuses here.
	oldRefused := func(stateDegraded, cpu, mem, disk bool) bool {
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
													CPU:           &models.CPU{Health: withMessage(cpu, "cpu is full")},
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

						limited, reason := protocolconverter.NewDefaultProtocolConverterService("test").IsResourceLimited(snapshot)

						Expect(limited).To(Equal(oldRefused(stateDegraded, cpu, mem, disk)),
							"admission must be what the old order decided")

						switch {
						case cpu:
							Expect(reason).To(Equal("CPU degraded: cpu is full"))
						case mem:
							Expect(reason).To(Equal("Memory degraded: memory is full"))
						case disk:
							Expect(reason).To(Equal("Disk degraded: disk is full"))
						case stateDegraded:
							Expect(reason).To(Equal("System in degraded state"),
								"with no resource degraded the container's own state is the only cause left to name")
						default:
							Expect(reason).To(BeEmpty())
						}
					})
				}
			}
		}
	}

	// The branches below the three resource checks: throttling, the overall
	// health, and a degraded resource that carries no message. Each is staged
	// with the container's own state degraded, the case where the state check
	// used to answer first, and each must still name its own cause.
	DescribeTable("names the cause below the resource checks, with the container's state degraded",
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

			limited, got := protocolconverter.NewDefaultProtocolConverterService("test").IsResourceLimited(snapshot)

			Expect(limited).To(BeTrue(), "a degraded container state refused before the change, so it must still refuse")
			Expect(got).To(HavePrefix(reason))
		},
		Entry("CPU throttled",
			container_monitor.ServiceInfo{
				OverallHealth: models.Active, CPUHealth: models.Active, MemoryHealth: models.Active, DiskHealth: models.Active,
				CPU: &models.CPU{IsThrottled: true, ThrottleRatio: 0.3, CgroupCores: 2},
			},
			"CPU throttled (30% of time)"),
		Entry("only the overall health degraded",
			container_monitor.ServiceInfo{
				OverallHealth: models.Degraded, CPUHealth: models.Active, MemoryHealth: models.Active, DiskHealth: models.Active,
			},
			"Overall system resources degraded"),
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
