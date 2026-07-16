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
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	pkgfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
)

var _ = Describe("ProtocolConverter Resource Limiting", func() {
	var (
		service  *protocolconverter.ProtocolConverterService
		snapshot pkgfsm.SystemSnapshot
	)

	BeforeEach(func() {
		// Create service with mock logger (or use NewDefaultProtocolConverterService)
		service = protocolconverter.NewDefaultProtocolConverterService("test")

		// Initialize with empty snapshot with feature flag enabled by default
		snapshot = pkgfsm.SystemSnapshot{
			Managers: make(map[string]pkgfsm.ManagerSnapshot),
			CurrentConfig: config.FullConfig{
				Agent: config.AgentConfig{
					EnableResourceLimitBlocking: true,
				},
			},
		}
	})

	Context("IsResourceLimited - Resource Blocking Decision Tree", func() {
		Describe("1. Theoretical Limits (Bridge Count)", func() {
			var maxBridges int

			BeforeEach(func() {
				// Reserve 1 CPU core for Redpanda as per sizing guidelines
				availableCores := runtime.NumCPU() - 1
				if availableCores < 0 {
					availableCores = 0
				}
				maxBridges = availableCores * constants.MaxBridgesPerCPUCore

				// Add healthy container for these tests
				snapshot.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
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
								},
							},
						},
					},
				}
			})

			It("should block when exactly at bridge limit", func() {
				// Add protocol converter manager with bridges at limit
				instances := make(map[string]*pkgfsm.FSMInstanceSnapshot)
				for i := range maxBridges {
					instances[string(rune('a'+i))] = &pkgfsm.FSMInstanceSnapshot{
						ID:           string(rune('a' + i)),
						CurrentState: "active",
						DesiredState: "active",
					}
				}

				snapshot.Managers[constants.ProtocolConverterManagerName] = &MockManagerSnapshot{
					Instances: instances,
				}

				limited, reason := service.IsResourceLimited(snapshot)

				Expect(limited).To(BeTrue())
				Expect(reason).To(ContainSubstring("Cannot create bridge - limit exceeded"))
				Expect(reason).To(ContainSubstring("Cannot create bridge - limit exceeded"))
				Expect(reason).To(ContainSubstring("1 core reserved for Redpanda"))
			})

			It("should allow creation when below limit", func() {
				// Add bridges below limit
				instances := make(map[string]*pkgfsm.FSMInstanceSnapshot)
				for i := range maxBridges - 1 {
					instances[string(rune('a'+i))] = &pkgfsm.FSMInstanceSnapshot{
						ID:           string(rune('a' + i)),
						CurrentState: "active",
						DesiredState: "active",
					}
				}

				snapshot.Managers[constants.ProtocolConverterManagerName] = &MockManagerSnapshot{
					Instances: instances,
				}

				limited, reason := service.IsResourceLimited(snapshot)

				Expect(limited).To(BeFalse())
				Expect(reason).To(BeEmpty())
			})

			It("should use cgroup CPU quota when available for bridge limits", func() {
				// Container with cgroup limit of 2 cores
				snapshot.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
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
									CPU: &models.CPU{
										CgroupCores: 2.0, // Limited to 2 cores
									},
								},
							},
						},
					},
				}

				// Max bridges should be (2-1) * 5 = 5 (1 core reserved for Redpanda)
				cgroupMaxBridges := (2 - 1) * constants.MaxBridgesPerCPUCore

				// Add exactly at cgroup limit
				instances := make(map[string]*pkgfsm.FSMInstanceSnapshot)
				for i := range cgroupMaxBridges {
					instances[string(rune('a'+i))] = &pkgfsm.FSMInstanceSnapshot{
						ID:           string(rune('a' + i)),
						CurrentState: "active",
						DesiredState: "active",
					}
				}

				snapshot.Managers[constants.ProtocolConverterManagerName] = &MockManagerSnapshot{
					Instances: instances,
				}

				limited, reason := service.IsResourceLimited(snapshot)

				Expect(limited).To(BeTrue())
				Expect(reason).To(ContainSubstring("Cannot create bridge - limit exceeded"))
				Expect(reason).To(ContainSubstring("5 bridges maximum"))
				Expect(reason).To(ContainSubstring("2.0 CPU cores"))
				Expect(reason).To(ContainSubstring("1 core reserved for Redpanda"))
			})

			It("should not count removing/removed bridges", func() {
				// Mix of active and removing bridges
				instances := make(map[string]*pkgfsm.FSMInstanceSnapshot)

				// Add active bridges just below limit
				for i := range maxBridges - 1 {
					instances[string(rune('a'+i))] = &pkgfsm.FSMInstanceSnapshot{
						ID:           string(rune('a' + i)),
						CurrentState: "active",
						DesiredState: "active",
					}
				}

				// Add removing/removed (shouldn't count)
				instances["removing1"] = &pkgfsm.FSMInstanceSnapshot{
					ID:           "removing1",
					CurrentState: fsm.LifecycleStateRemoving,
					DesiredState: "removed",
				}
				instances["removed1"] = &pkgfsm.FSMInstanceSnapshot{
					ID:           "removed1",
					CurrentState: fsm.LifecycleStateRemoved,
					DesiredState: "removed",
				}

				snapshot.Managers[constants.ProtocolConverterManagerName] = &MockManagerSnapshot{
					Instances: instances,
				}

				limited, reason := service.IsResourceLimited(snapshot)

				Expect(limited).To(BeFalse())
				Expect(reason).To(BeEmpty())
			})
		})

		Describe("2. Resource Degradation Blocking", func() {
			Context("CPU Degradation", func() {
				It("should block when CPU is degraded", func() {
					snapshot.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
						Instances: map[string]*pkgfsm.FSMInstanceSnapshot{
							constants.CoreInstanceName: {
								ID:           constants.CoreInstanceName,
								CurrentState: "active",
								DesiredState: "active",
								LastObservedState: &container.ContainerObservedStateSnapshot{
									ServiceInfoSnapshot: container_monitor.ServiceInfo{
										OverallHealth: models.Degraded,
										CPUHealth:     models.Degraded,
										MemoryHealth:  models.Active,
										DiskHealth:    models.Active,
										CPU: &models.CPU{
											Health: &models.Health{
												Message: "CPU usage at 85%",
											},
										},
									},
								},
							},
						},
					}

					limited, reason := service.IsResourceLimited(snapshot)

					Expect(limited).To(BeTrue())
					Expect(reason).To(Equal("CPU degraded: CPU usage at 85%"))
				})

				It("should block when CPU is throttled with detailed message", func() {
					snapshot.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
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
										CPU: &models.CPU{
											CgroupCores: 2.0, // Limited to 2 cores
											VerdictBasis: &models.VerdictBasis{
												Throttle: models.VerdictBasisCause{
													Value: 0.15, // 15% throttled
													Fired: true,
												},
											},
										},
									},
								},
							},
						},
					}

					limited, reason := service.IsResourceLimited(snapshot)

					Expect(limited).To(BeTrue())
					Expect(reason).To(ContainSubstring("CPU throttled (15% of time)"))
					Expect(reason).To(ContainSubstring("Container limited to 2.0 cores"))
					Expect(reason).To(ContainSubstring("needs more during peaks"))
					Expect(reason).To(MatchRegexp(`host has \d+ cores available`))
				})
			})

			Context("Memory Degradation", func() {
				It("should block when memory is degraded", func() {
					snapshot.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
						Instances: map[string]*pkgfsm.FSMInstanceSnapshot{
							constants.CoreInstanceName: {
								ID:           constants.CoreInstanceName,
								CurrentState: "active",
								DesiredState: "active",
								LastObservedState: &container.ContainerObservedStateSnapshot{
									ServiceInfoSnapshot: container_monitor.ServiceInfo{
										OverallHealth: models.Degraded,
										CPUHealth:     models.Active,
										MemoryHealth:  models.Degraded,
										DiskHealth:    models.Active,
										Memory: &models.Memory{
											Health: &models.Health{
												Message: "Memory usage at 92%",
											},
										},
									},
								},
							},
						},
					}

					limited, reason := service.IsResourceLimited(snapshot)

					Expect(limited).To(BeTrue())
					Expect(reason).To(Equal("Memory degraded: Memory usage at 92%"))
				})

				It("should use generic message when health message unavailable", func() {
					snapshot.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
						Instances: map[string]*pkgfsm.FSMInstanceSnapshot{
							constants.CoreInstanceName: {
								ID:           constants.CoreInstanceName,
								CurrentState: "active",
								DesiredState: "active",
								LastObservedState: &container.ContainerObservedStateSnapshot{
									ServiceInfoSnapshot: container_monitor.ServiceInfo{
										OverallHealth: models.Degraded,
										CPUHealth:     models.Active,
										MemoryHealth:  models.Degraded,
										DiskHealth:    models.Active,
										// No Memory struct
									},
								},
							},
						},
					}

					limited, reason := service.IsResourceLimited(snapshot)

					Expect(limited).To(BeTrue())
					Expect(reason).To(Equal("Memory resources degraded"))
				})
			})

			Context("Disk Degradation", func() {
				It("should block when disk is degraded", func() {
					snapshot.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
						Instances: map[string]*pkgfsm.FSMInstanceSnapshot{
							constants.CoreInstanceName: {
								ID:           constants.CoreInstanceName,
								CurrentState: "active",
								DesiredState: "active",
								LastObservedState: &container.ContainerObservedStateSnapshot{
									ServiceInfoSnapshot: container_monitor.ServiceInfo{
										OverallHealth: models.Degraded,
										CPUHealth:     models.Active,
										MemoryHealth:  models.Active,
										DiskHealth:    models.Degraded,
										Disk: &models.Disk{
											Health: &models.Health{
												Message: "Disk usage at 95%",
											},
										},
									},
								},
							},
						},
					}

					limited, reason := service.IsResourceLimited(snapshot)

					Expect(limited).To(BeTrue())
					Expect(reason).To(Equal("Disk degraded: Disk usage at 95%"))
				})
			})

			Context("Multiple Resources Degraded", func() {
				It("should report first degraded resource in priority order", func() {
					// Priority: CPU -> Memory -> Disk
					snapshot.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
						Instances: map[string]*pkgfsm.FSMInstanceSnapshot{
							constants.CoreInstanceName: {
								ID:           constants.CoreInstanceName,
								CurrentState: "active",
								DesiredState: "active",
								LastObservedState: &container.ContainerObservedStateSnapshot{
									ServiceInfoSnapshot: container_monitor.ServiceInfo{
										OverallHealth: models.Degraded,
										CPUHealth:     models.Degraded, // This should be reported first
										MemoryHealth:  models.Degraded,
										DiskHealth:    models.Degraded,
										CPU: &models.CPU{
											Health: &models.Health{
												Message: "CPU overloaded",
											},
										},
										Memory: &models.Memory{
											Health: &models.Health{
												Message: "Memory exhausted",
											},
										},
										Disk: &models.Disk{
											Health: &models.Health{
												Message: "Disk full",
											},
										},
									},
								},
							},
						},
					}

					limited, reason := service.IsResourceLimited(snapshot)

					Expect(limited).To(BeTrue())
					Expect(reason).To(Equal("CPU degraded: CPU overloaded"))
				})
			})

			Context("Overall Health Degradation", func() {
				It("should use overall health as fallback when individual resources show active", func() {
					snapshot.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
						Instances: map[string]*pkgfsm.FSMInstanceSnapshot{
							constants.CoreInstanceName: {
								ID:           constants.CoreInstanceName,
								CurrentState: "active",
								DesiredState: "active",
								LastObservedState: &container.ContainerObservedStateSnapshot{
									ServiceInfoSnapshot: container_monitor.ServiceInfo{
										OverallHealth: models.Degraded, // Overall degraded
										CPUHealth:     models.Active,   // But individuals show active
										MemoryHealth:  models.Active,
										DiskHealth:    models.Active,
									},
								},
							},
						},
					}

					limited, reason := service.IsResourceLimited(snapshot)

					Expect(limited).To(BeTrue())
					Expect(reason).To(Equal("Overall system resources degraded"))
				})
			})
		})

		Describe("3. Container State Checks", func() {
			It("should block when container FSM state is degraded", func() {
				snapshot.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
					Instances: map[string]*pkgfsm.FSMInstanceSnapshot{
						constants.CoreInstanceName: {
							ID:           constants.CoreInstanceName,
							CurrentState: "degraded", // FSM state degraded
							DesiredState: "active",
							// No observed state needed for this check
						},
					},
				}

				limited, reason := service.IsResourceLimited(snapshot)

				Expect(limited).To(BeTrue())
				Expect(reason).To(Equal("System in degraded state"))
			})

			It("should return the curated per-cause block reason when a degraded FSM has a CPU with a dominant cause", func() {
				// Pin the literal wire-level message for each kind so drift
				// between cpuhealth.BlockReason and its copy is caught here
				// rather than re-asserting the same function the unit calls.
				// Host-contention is folded in v4 (never emitted) and its BlockReason
				// case was deleted, so it is not pinned here.
				expectedReasons := map[cpuhealth.CauseKind]string{
					cpuhealth.CauseKindThrottling: "Can't add another bridge: this instance is already hitting its CPU limit. Raise the limit or reduce load first.",
					cpuhealth.CauseKindPressure:   "Can't add another bridge: tasks on this instance are already waiting for a free CPU core. Reduce load, or give this instance more CPU, first.",
					cpuhealth.CauseKindSteal:      "Can't add another bridge: the server isn't giving this instance enough CPU (other VMs are using it). Free up CPU on the server first.",
					// This fixture has no VerdictBasis, so the signals are
					// zero-valued and BlockReason hits the saturation default
					// (the generic remediation, dropping the pre-existing
					// first-person "we"). The sub-latch dispatch is pinned
					// directly in pkg/cpuhealth/message_test.go.
					cpuhealth.CauseKindSaturation: "Can't add another bridge: CPU is running near full. Add CPU capacity, or set a CPU limit, first.",
				}

				for kind, expected := range expectedReasons {
					snapshot.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
						Instances: map[string]*pkgfsm.FSMInstanceSnapshot{
							constants.CoreInstanceName: {
								ID:           constants.CoreInstanceName,
								CurrentState: "degraded",
								DesiredState: "active",
								LastObservedState: &container.ContainerObservedStateSnapshot{
									ServiceInfoSnapshot: container_monitor.ServiceInfo{
										OverallHealth: models.Degraded,
										CPUHealth:     models.Degraded,
										MemoryHealth:  models.Active,
										DiskHealth:    models.Active,
										CPU: &models.CPU{
											State: "degraded",
											Causes: []models.Cause{
												{Kind: models.CauseKind(kind), Value: 0.5},
											},
										},
									},
								},
							},
						},
					}

					limited, reason := service.IsResourceLimited(snapshot)

					Expect(limited).To(BeTrue(), "kind %s should block", kind)
					Expect(reason).To(Equal(expected), "kind %s should map to its curated block reason", kind)
				}
			})

			It("should fall back to the generic degraded reason when memory or disk is also degraded, even if a CPU cause is present", func() {
				// Coincident degradation: when the FSM is degraded due to memory
				// or disk (not solely CPU), the CPU-specific block reason would
				// mislead the operator into raising the CPU limit, which would
				// not unblock bridge creation. The generic reason must be used.
				snapshot.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
					Instances: map[string]*pkgfsm.FSMInstanceSnapshot{
						constants.CoreInstanceName: {
							ID:           constants.CoreInstanceName,
							CurrentState: "degraded",
							DesiredState: "active",
							LastObservedState: &container.ContainerObservedStateSnapshot{
								ServiceInfoSnapshot: container_monitor.ServiceInfo{
									OverallHealth: models.Degraded,
									CPUHealth:     models.Degraded,
									MemoryHealth:  models.Degraded,
									DiskHealth:    models.Active,
									CPU: &models.CPU{
										State: "degraded",
										Causes: []models.Cause{
											{Kind: models.CauseKind(cpuhealth.CauseKindThrottling), Value: 0.5},
										},
									},
								},
							},
						},
					},
				}

				limited, reason := service.IsResourceLimited(snapshot)

				Expect(limited).To(BeTrue())
				Expect(reason).To(Equal("System in degraded state"))
			})

			It("should fall back to the generic degraded reason when CPU is nil", func() {
				snapshot.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
					Instances: map[string]*pkgfsm.FSMInstanceSnapshot{
						constants.CoreInstanceName: {
							ID:           constants.CoreInstanceName,
							CurrentState: "degraded",
							DesiredState: "active",
							LastObservedState: &container.ContainerObservedStateSnapshot{
								ServiceInfoSnapshot: container_monitor.ServiceInfo{
									OverallHealth: models.Degraded,
									CPUHealth:     models.Degraded,
									MemoryHealth:  models.Active,
									DiskHealth:    models.Active,
									CPU:           nil,
								},
							},
						},
					},
				}

				limited, reason := service.IsResourceLimited(snapshot)

				Expect(limited).To(BeTrue())
				Expect(reason).To(Equal("System in degraded state"))
			})

			It("should fall back to the generic degraded reason when CPU causes are empty", func() {
				snapshot.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
					Instances: map[string]*pkgfsm.FSMInstanceSnapshot{
						constants.CoreInstanceName: {
							ID:           constants.CoreInstanceName,
							CurrentState: "degraded",
							DesiredState: "active",
							LastObservedState: &container.ContainerObservedStateSnapshot{
								ServiceInfoSnapshot: container_monitor.ServiceInfo{
									OverallHealth: models.Degraded,
									CPUHealth:     models.Degraded,
									MemoryHealth:  models.Active,
									DiskHealth:    models.Active,
									CPU: &models.CPU{
										State:  "degraded",
										Causes: []models.Cause{},
									},
								},
							},
						},
					},
				}

				limited, reason := service.IsResourceLimited(snapshot)

				Expect(limited).To(BeTrue())
				Expect(reason).To(Equal("System in degraded state"))
			})

			It("should fall back to the generic degraded reason when LastObservedState is the wrong concrete type", func() {
				snapshot.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
					Instances: map[string]*pkgfsm.FSMInstanceSnapshot{
						constants.CoreInstanceName: {
							ID:                constants.CoreInstanceName,
							CurrentState:      "degraded",
							DesiredState:      "active",
							LastObservedState: &wrongTypeObservedState{},
						},
					},
				}

				limited, reason := service.IsResourceLimited(snapshot)

				Expect(limited).To(BeTrue())
				Expect(reason).To(Equal("System in degraded state"))
			})

			It("should block when container manager not present", func() {
				// No container manager at all
				limited, reason := service.IsResourceLimited(snapshot)

				Expect(limited).To(BeTrue())
				Expect(reason).To(Equal("Container monitor not available"))
			})

			It("should block when Core instance not present", func() {
				snapshot.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
					Instances: make(map[string]*pkgfsm.FSMInstanceSnapshot),
				}

				limited, reason := service.IsResourceLimited(snapshot)

				Expect(limited).To(BeTrue())
				Expect(reason).To(Equal("Container health status unavailable"))
			})
		})

		Describe("4. Edge Cases - Removal During Resource Limits", func() {
			It("should document that removal is allowed even when resources are limited", func() {
				// This test documents the expected behavior that bridges stuck in to_be_created
				// due to resource limits can still be removed. The actual removal logic is handled
				// in the FSM reconciliation, not in IsResourceLimited.

				// Setup: System at resource limits
				snapshot.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
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
									CPU: &models.CPU{
										CgroupCores: 2.0,
										VerdictBasis: &models.VerdictBasis{
											Throttle: models.VerdictBasisCause{
												Value: 0.20,
												Fired: true,
											},
										},
									},
								},
							},
						},
					},
				}

				// IsResourceLimited should still return true (resources are limited)
				limited, reason := service.IsResourceLimited(snapshot)

				Expect(limited).To(BeTrue())
				Expect(reason).To(ContainSubstring("CPU throttled"))

				// Note: The FSM reconciliation logic (not tested here) should allow
				// transition from to_be_created -> to_be_removed even when IsResourceLimited returns true
			})
		})

		Describe("5. Healthy System - Allow Creation", func() {
			It("should allow creation when all resources healthy and below limits", func() {
				// Healthy container
				snapshot.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
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
									CPU: &models.CPU{
										VerdictBasis: &models.VerdictBasis{
											Throttle: models.VerdictBasisCause{
												Value: 0.0,
												Fired: false,
											},
										},
									},
								},
							},
						},
					},
				}

				// A few bridges, below limit
				instances := make(map[string]*pkgfsm.FSMInstanceSnapshot)
				instances["bridge1"] = &pkgfsm.FSMInstanceSnapshot{
					ID:           "bridge1",
					CurrentState: "active",
					DesiredState: "active",
				}
				instances["bridge2"] = &pkgfsm.FSMInstanceSnapshot{
					ID:           "bridge2",
					CurrentState: "active",
					DesiredState: "active",
				}

				snapshot.Managers[constants.ProtocolConverterManagerName] = &MockManagerSnapshot{
					Instances: instances,
				}

				limited, reason := service.IsResourceLimited(snapshot)

				Expect(limited).To(BeFalse())
				Expect(reason).To(BeEmpty())
			})
		})

		Describe("6. Feature Flag - EnableResourceLimitBlocking", func() {
			BeforeEach(func() {
				// Set up container manager with degraded resources for these tests
				snapshot.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
					Instances: map[string]*pkgfsm.FSMInstanceSnapshot{
						constants.CoreInstanceName: {
							ID:           constants.CoreInstanceName,
							CurrentState: "active",
							DesiredState: "active",
							LastObservedState: &container.ContainerObservedStateSnapshot{
								ServiceInfoSnapshot: container_monitor.ServiceInfo{
									OverallHealth: models.Degraded,
									CPUHealth:     models.Degraded,
									MemoryHealth:  models.Active,
									DiskHealth:    models.Active,
								},
							},
						},
					},
				}
			})

			It("should block creation when feature flag is enabled and resources are degraded", func() {
				// Feature flag is already enabled in BeforeEach
				limited, reason := service.IsResourceLimited(snapshot)

				Expect(limited).To(BeTrue())
				Expect(reason).To(ContainSubstring("CPU resources degraded"))
			})

			It("should NOT block creation when feature flag is disabled even if resources are degraded", func() {
				// Disable feature flag
				snapshot.CurrentConfig.Agent.EnableResourceLimitBlocking = false

				limited, reason := service.IsResourceLimited(snapshot)

				Expect(limited).To(BeFalse())
				Expect(reason).To(BeEmpty())
			})

			It("should block creation when feature flag is enabled and bridge limit exceeded", func() {
				// Set up healthy container but too many bridges
				snapshot.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
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
								},
							},
						},
					},
				}

				// Calculate max bridges
				availableCores := runtime.NumCPU() - 1
				if availableCores < 0 {
					availableCores = 0
				}
				maxBridges := availableCores * constants.MaxBridgesPerCPUCore

				// Add bridges exceeding limit to trigger blocking
				instances := make(map[string]*pkgfsm.FSMInstanceSnapshot)
				// Add one more than the limit to ensure blocking
				for i := 0; i <= maxBridges; i++ {
					instances[string(rune('a'+i))] = &pkgfsm.FSMInstanceSnapshot{
						ID:           string(rune('a' + i)),
						CurrentState: "active",
						DesiredState: "active",
					}
				}

				snapshot.Managers[constants.ProtocolConverterManagerName] = &MockManagerSnapshot{
					Instances: instances,
				}

				// Feature flag enabled
				limited, reason := service.IsResourceLimited(snapshot)

				Expect(limited).To(BeTrue())
				Expect(reason).To(ContainSubstring("Cannot create bridge - limit exceeded"))
			})

			It("should NOT block creation when feature flag is disabled even if bridge limit exceeded", func() {
				// Set up healthy container but too many bridges
				snapshot.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
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
								},
							},
						},
					},
				}

				// Calculate max bridges
				availableCores := runtime.NumCPU() - 1
				if availableCores < 0 {
					availableCores = 0
				}
				maxBridges := availableCores * constants.MaxBridgesPerCPUCore

				// Add bridges over limit
				instances := make(map[string]*pkgfsm.FSMInstanceSnapshot)
				for i := 0; i <= maxBridges; i++ { // Note: <= to exceed limit
					instances[string(rune('a'+i))] = &pkgfsm.FSMInstanceSnapshot{
						ID:           string(rune('a' + i)),
						CurrentState: "active",
						DesiredState: "active",
					}
				}

				snapshot.Managers[constants.ProtocolConverterManagerName] = &MockManagerSnapshot{
					Instances: instances,
				}

				// Disable feature flag
				snapshot.CurrentConfig.Agent.EnableResourceLimitBlocking = false

				limited, reason := service.IsResourceLimited(snapshot)

				Expect(limited).To(BeFalse())
				Expect(reason).To(BeEmpty())
			})
		})
	})
})

var _ = Describe("IsResourceLimited: held stale-degraded verdict blocks bridges during a sampler outage (Rung 7.4)", func() {
	It("returns limited=true when the container monitor holds a stale degraded verdict across a sampler failure", func() {
		// Integration bridge: drive a real ContainerMonitorService through a
		// degraded tick then a sampler-failure tick, feed the failure-tick
		// ServiceInfo into IsResourceLimited, and assert it blocks. This pins
		// the design decision that a held stale-degraded verdict blocks bridge
		// creation during a sampler outage. The conservative choice: block
		// rather than admit a new bridge to a possibly-degraded host. The
		// sampler is down, so we cannot know whether the host degraded during
		// the outage. Holding the last verdict (degraded) blocks bridges until
		// the sampler recovers and recomputes.
		ctx := context.Background()

		testDataPath, err := os.MkdirTemp("", "rung7-c2-isresourcelimited")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(testDataPath) }()

		const cpuMax = "200000 100000\n" // quota 2.0 cores (capped, limit mode)
		var (
			nrPeriods, nrThrottled, usageUsec int64
			samplerFails                        bool
		)

		mockFS := filesystem.NewMockFileSystem()
		mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
			switch path {
			case "/sys/fs/cgroup/cpu.max":
				return []byte(cpuMax), nil
			case "/sys/fs/cgroup/cpu.stat":
				if samplerFails {
					return nil, errors.New("cpu.stat transient read error")
				}
				return []byte(fmt.Sprintf(
					"usage_usec %d\nnr_periods %d\nnr_throttled %d\nthrottled_usec 0\n",
					usageUsec, nrPeriods, nrThrottled,
				)), nil
			default:
				return nil, errors.New("file not found: " + path)
			}
		})

		monSvc := container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

		// Tick 0: baseline the throttle ring (healthy, single point).
		samplerFails = false
		nrPeriods, nrThrottled, usageUsec = 1000, 0, 1_000_000
		_, err = monSvc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Tick 1: throttle fires (ratio 0.10 > 0.05), verdict degrades. The
		// wire carries CPUHealth=Degraded + a verdict basis with a fired cause.
		nrPeriods, nrThrottled, usageUsec = 2000, 100, 2_000_000
		time.Sleep(1 * time.Second)
		status1, err := monSvc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(status1.CPUHealth).To(Equal(models.Degraded),
			"precondition: tick 1 degrades so CPUHealth=Degraded is held")
		Expect(status1.CPU.Causes).NotTo(BeEmpty(),
			"precondition: tick 1 carries a verdict cause so the degraded-FSM branch's len>0 guard holds")

		// Tick 2: sampler fails. The held degraded verdict + basis + causes
		// persist on the wire (the failure-tick ServiceInfo).
		samplerFails = true
		status2, err := monSvc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(status2.CPUHealth).To(Equal(models.Degraded),
			"the held CPUHealth=Degraded persists across the sampler failure")
		Expect(status2.CPU.Causes).NotTo(BeEmpty(),
			"the held verdict causes persist across the sampler failure")

		// Feed the failure-tick ServiceInfo into IsResourceLimited via a
		// snapshot mirroring the FSM wire shape. The container FSM is in
		// "active" (not "degraded"), so the degraded-FSM branch at line 1160
		// is skipped and the per-resource CPUHealth check at line 1196 fires.
		svc := protocolconverter.NewDefaultProtocolConverterService("test")
		snap := pkgfsm.SystemSnapshot{
			Managers: make(map[string]pkgfsm.ManagerSnapshot),
			CurrentConfig: config.FullConfig{
				Agent: config.AgentConfig{
					EnableResourceLimitBlocking: true,
				},
			},
		}
		snap.Managers[constants.ContainerManagerName] = &MockManagerSnapshot{
			Name: constants.ContainerManagerName,
			Instances: map[string]*pkgfsm.FSMInstanceSnapshot{
				constants.CoreInstanceName: {
					ID:           constants.CoreInstanceName,
					CurrentState: "active",
					DesiredState: "active",
					LastObservedState: &container.ContainerObservedStateSnapshot{
						ServiceInfoSnapshot: *status2,
					},
				},
			},
		}

		limited, reason := svc.IsResourceLimited(snap)
		Expect(limited).To(BeTrue(),
			"a held stale-degraded CPU verdict must block bridge creation during a sampler outage (conservative: block rather than admit to a possibly-degraded host)")
		Expect(reason).NotTo(BeEmpty(),
			"a block reason is surfaced so the MC can explain why bridge creation is blocked")
	})
})

// MockManagerSnapshot implements pkgfsm.ManagerSnapshot for testing.
type MockManagerSnapshot struct {
	Name         string
	Instances    map[string]*pkgfsm.FSMInstanceSnapshot
	Tick         uint64
	SnapshotTime time.Time
}

func (m *MockManagerSnapshot) GetName() string {
	return m.Name
}

func (m *MockManagerSnapshot) GetInstances() map[string]*pkgfsm.FSMInstanceSnapshot {
	return m.Instances
}

func (m *MockManagerSnapshot) GetInstance(name string) *pkgfsm.FSMInstanceSnapshot {
	return m.Instances[name]
}

func (m *MockManagerSnapshot) GetSnapshotTime() time.Time {
	if m.SnapshotTime.IsZero() {
		return time.Now()
	}

	return m.SnapshotTime
}

func (m *MockManagerSnapshot) GetManagerTick() uint64 {
	return m.Tick
}

// wrongTypeObservedState implements pkgfsm.ObservedStateSnapshot but is not a
// *container.ContainerObservedStateSnapshot, used to pin the type-assertion
// fall-through in the degraded-FSM branch of IsResourceLimited.
type wrongTypeObservedState struct{}

func (wrongTypeObservedState) IsObservedStateSnapshot() {}
