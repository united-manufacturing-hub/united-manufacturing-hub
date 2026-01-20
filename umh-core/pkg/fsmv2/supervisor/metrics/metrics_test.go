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

package metrics_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/metrics"
)

var _ = Describe("Infrastructure Recovery Metrics", Label("metrics"), func() {
	Context("RecordCircuitOpen", func() {
		It("should record circuit breaker open state", func() {
			supervisorID := "test-supervisor-1"

			Expect(func() {
				metrics.RecordCircuitOpen(supervisorID, true)
			}).NotTo(Panic())

			Expect(func() {
				metrics.RecordCircuitOpen(supervisorID, false)
			}).NotTo(Panic())
		})
	})

	Context("RecordInfrastructureRecovery", func() {
		It("should record recovery event with duration", func() {
			supervisorID := "test-supervisor-2"
			duration := 500 * time.Millisecond

			Expect(func() {
				metrics.RecordInfrastructureRecovery(supervisorID, duration)
			}).NotTo(Panic())
		})
	})

	Context("RecordChildHealthCheck", func() {
		It("should record child health check with status", func() {
			supervisorID := "test-supervisor-3"
			childName := "test-child"

			Expect(func() {
				metrics.RecordChildHealthCheck(supervisorID, childName, "healthy")
			}).NotTo(Panic())

			Expect(func() {
				metrics.RecordChildHealthCheck(supervisorID, childName, "unhealthy")
			}).NotTo(Panic())
		})
	})
})

var _ = Describe("Action Execution Metrics", Label("metrics"), func() {
	Context("RecordActionQueued", func() {
		It("should record action queuing events", func() {
			supervisorID := "test-supervisor-4"
			actionType := "restart"

			Expect(func() {
				metrics.RecordActionQueued(supervisorID, actionType)
			}).NotTo(Panic())
		})
	})

	Context("RecordActionQueueSize", func() {
		It("should record action queue size", func() {
			supervisorID := "test-supervisor-5"

			Expect(func() {
				metrics.RecordActionQueueSize(supervisorID, 0)
			}).NotTo(Panic())

			Expect(func() {
				metrics.RecordActionQueueSize(supervisorID, 5)
			}).NotTo(Panic())

			Expect(func() {
				metrics.RecordActionQueueSize(supervisorID, 100)
			}).NotTo(Panic())
		})
	})

	Context("RecordActionExecutionDuration", func() {
		It("should record action execution duration with status", func() {
			supervisorID := "test-supervisor-6"
			actionType := "restart"
			duration := 250 * time.Millisecond

			Expect(func() {
				metrics.RecordActionExecutionDuration(supervisorID, actionType, "success", duration)
			}).NotTo(Panic())

			Expect(func() {
				metrics.RecordActionExecutionDuration(supervisorID, actionType, "failure", duration)
			}).NotTo(Panic())

			Expect(func() {
				metrics.RecordActionExecutionDuration(supervisorID, actionType, "timeout", duration)
			}).NotTo(Panic())
		})
	})

	Context("RecordActionTimeout", func() {
		It("should record action timeout events", func() {
			supervisorID := "test-supervisor-7"
			actionType := "restart"

			Expect(func() {
				metrics.RecordActionTimeout(supervisorID, actionType)
			}).NotTo(Panic())
		})
	})

	Context("RecordWorkerPoolUtilization", func() {
		It("should record worker pool utilization", func() {
			poolName := "default-pool"

			Expect(func() {
				metrics.RecordWorkerPoolUtilization(poolName, 0.0)
			}).NotTo(Panic())

			Expect(func() {
				metrics.RecordWorkerPoolUtilization(poolName, 0.5)
			}).NotTo(Panic())

			Expect(func() {
				metrics.RecordWorkerPoolUtilization(poolName, 1.0)
			}).NotTo(Panic())
		})
	})

	Context("RecordWorkerPoolQueueSize", func() {
		It("should record worker pool queue size", func() {
			poolName := "default-pool"

			Expect(func() {
				metrics.RecordWorkerPoolQueueSize(poolName, 0)
			}).NotTo(Panic())

			Expect(func() {
				metrics.RecordWorkerPoolQueueSize(poolName, 10)
			}).NotTo(Panic())

			Expect(func() {
				metrics.RecordWorkerPoolQueueSize(poolName, 50)
			}).NotTo(Panic())
		})
	})
})

var _ = Describe("Hierarchical Composition Metrics", Label("metrics"), func() {
	Context("RecordChildCount", func() {
		It("should record child count", func() {
			supervisorID := "test-supervisor-8"

			Expect(func() {
				metrics.RecordChildCount(supervisorID, 0)
			}).NotTo(Panic())

			Expect(func() {
				metrics.RecordChildCount(supervisorID, 5)
			}).NotTo(Panic())

			Expect(func() {
				metrics.RecordChildCount(supervisorID, 10)
			}).NotTo(Panic())
		})
	})

	Context("RecordReconciliation", func() {
		It("should record reconciliation with result and duration", func() {
			supervisorID := "test-supervisor-9"
			duration := 100 * time.Millisecond

			Expect(func() {
				metrics.RecordReconciliation(supervisorID, "success", duration)
			}).NotTo(Panic())

			Expect(func() {
				metrics.RecordReconciliation(supervisorID, "failure", duration)
			}).NotTo(Panic())

			Expect(func() {
				metrics.RecordReconciliation(supervisorID, "partial", duration)
			}).NotTo(Panic())
		})
	})

	Context("RecordTickPropagationDepth", func() {
		It("should record tick propagation depth", func() {
			supervisorID := "test-supervisor-10"

			Expect(func() {
				metrics.RecordTickPropagationDepth(supervisorID, 1)
			}).NotTo(Panic())

			Expect(func() {
				metrics.RecordTickPropagationDepth(supervisorID, 3)
			}).NotTo(Panic())

			Expect(func() {
				metrics.RecordTickPropagationDepth(supervisorID, 10)
			}).NotTo(Panic())
		})
	})

	Context("RecordTickPropagationDuration", func() {
		It("should record tick propagation duration", func() {
			supervisorID := "test-supervisor-11"
			duration := 50 * time.Millisecond

			Expect(func() {
				metrics.RecordTickPropagationDuration(supervisorID, duration)
			}).NotTo(Panic())

			Expect(func() {
				metrics.RecordTickPropagationDuration(supervisorID, 200*time.Millisecond)
			}).NotTo(Panic())
		})
	})
})

var _ = Describe("Template Rendering Metrics", Label("metrics"), func() {
	Context("RecordTemplateRenderingDuration", func() {
		It("should record template rendering duration with status", func() {
			supervisorID := "test-supervisor-12"
			duration := 25 * time.Millisecond

			Expect(func() {
				metrics.RecordTemplateRenderingDuration(supervisorID, "success", duration)
			}).NotTo(Panic())

			Expect(func() {
				metrics.RecordTemplateRenderingDuration(supervisorID, "failure", duration)
			}).NotTo(Panic())
		})
	})

	Context("RecordTemplateRenderingError", func() {
		It("should record template rendering errors by type", func() {
			supervisorID := "test-supervisor-13"

			Expect(func() {
				metrics.RecordTemplateRenderingError(supervisorID, "parse_error")
			}).NotTo(Panic())

			Expect(func() {
				metrics.RecordTemplateRenderingError(supervisorID, "execution_error")
			}).NotTo(Panic())

			Expect(func() {
				metrics.RecordTemplateRenderingError(supervisorID, "validation_error")
			}).NotTo(Panic())
		})
	})

	Context("RecordVariablePropagation", func() {
		It("should record variable propagation events", func() {
			supervisorID := "test-supervisor-14"

			Expect(func() {
				metrics.RecordVariablePropagation(supervisorID)
			}).NotTo(Panic())
		})
	})
})

var _ = Describe("State Transition Metrics", Label("metrics"), func() {
	Context("RecordStateTransition", func() {
		It("should record state transition without panic", func() {
			// RED: This will fail until RecordStateTransition() is implemented
			Expect(func() {
				metrics.RecordStateTransition("communicator", "TryingToAuthenticate", "Syncing")
			}).NotTo(Panic())
		})

		It("should record multiple state transitions", func() {
			Expect(func() {
				metrics.RecordStateTransition("communicator", "Stopped", "TryingToAuthenticate")
				metrics.RecordStateTransition("communicator", "TryingToAuthenticate", "Syncing")
				metrics.RecordStateTransition("communicator", "Syncing", "Degraded")
			}).NotTo(Panic())
		})
	})

	Context("RecordStateDuration", func() {
		It("should record state duration without panic", func() {
			// RED: This will fail until RecordStateDuration() is implemented
			Expect(func() {
				metrics.RecordStateDuration("communicator", "worker-1", "Syncing", 10*time.Second)
			}).NotTo(Panic())
		})

		It("should handle zero duration", func() {
			Expect(func() {
				metrics.RecordStateDuration("communicator", "worker-1", "Stopped", 0)
			}).NotTo(Panic())
		})

		It("should record duration for different workers", func() {
			Expect(func() {
				metrics.RecordStateDuration("communicator", "worker-1", "Syncing", 10*time.Second)
				metrics.RecordStateDuration("communicator", "worker-2", "Syncing", 20*time.Second)
			}).NotTo(Panic())
		})
	})

	Context("CleanupStateDuration", func() {
		It("should cleanup state duration without panic", func() {
			// RED: This will fail until CleanupStateDuration() is implemented
			Expect(func() {
				metrics.CleanupStateDuration("communicator", "worker-1", "Syncing")
			}).NotTo(Panic())
		})

		It("should cleanup after recording", func() {
			Expect(func() {
				metrics.RecordStateDuration("communicator", "worker-1", "Syncing", 10*time.Second)
				metrics.CleanupStateDuration("communicator", "worker-1", "Syncing")
			}).NotTo(Panic())
		})
	})
})
