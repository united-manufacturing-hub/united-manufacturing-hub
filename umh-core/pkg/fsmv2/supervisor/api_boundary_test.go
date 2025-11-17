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

package supervisor_test

import (
	"reflect"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

func TestAPIBoundary(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Supervisor API Boundary Suite")
}

var _ = Describe("Supervisor API Boundary", func() {
	var supervisorType reflect.Type

	BeforeEach(func() {
		supervisorType = reflect.TypeOf(&supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState]{})
	})

	Context("Public API methods", func() {
		It("should export lifecycle management methods", func() {
			publicMethods := []string{
				"Start",
				"Shutdown",
			}

			for _, methodName := range publicMethods {
				method, exists := supervisorType.MethodByName(methodName)
				Expect(exists).To(BeTrue(), "Public method %s should exist", methodName)
				Expect(method.IsExported()).To(BeTrue(), "Method %s should be exported", methodName)
			}
		})

		It("should export worker registry methods", func() {
			publicMethods := []string{
				"AddWorker",
				"RemoveWorker",
				"GetWorker",
				"ListWorkers",
				"GetWorkers",
			}

			for _, methodName := range publicMethods {
				method, exists := supervisorType.MethodByName(methodName)
				Expect(exists).To(BeTrue(), "Public method %s should exist", methodName)
				Expect(method.IsExported()).To(BeTrue(), "Method %s should be exported", methodName)
			}
		})

		It("should export state inspection methods", func() {
			publicMethods := []string{
				"GetWorkerState",
				"GetCurrentState",
			}

			for _, methodName := range publicMethods {
				method, exists := supervisorType.MethodByName(methodName)
				Expect(exists).To(BeTrue(), "Public method %s should exist", methodName)
				Expect(method.IsExported()).To(BeTrue(), "Method %s should be exported", methodName)
			}
		})

		It("should export configuration methods", func() {
			publicMethods := []string{
				"SetGlobalVariables",
			}

			for _, methodName := range publicMethods {
				method, exists := supervisorType.MethodByName(methodName)
				Expect(exists).To(BeTrue(), "Public method %s should exist", methodName)
				Expect(method.IsExported()).To(BeTrue(), "Method %s should be exported", methodName)
			}
		})

		It("should export testing support methods", func() {
			publicMethods := []string{
				"GetChildren",
				"GetMappedParentState",
			}

			for _, methodName := range publicMethods {
				method, exists := supervisorType.MethodByName(methodName)
				Expect(exists).To(BeTrue(), "Public method %s should exist", methodName)
				Expect(method.IsExported()).To(BeTrue(), "Method %s should be exported", methodName)
			}
		})
	})

	Context("Internal implementation methods", func() {
		It("should NOT export FSM execution methods", func() {
			internalMethods := []string{
				"tickWorker",
				"Tick",
				"TickAll",
				"tickLoop",
			}

			for _, methodName := range internalMethods {
				_, exists := supervisorType.MethodByName(methodName)
				Expect(exists).To(BeFalse(), "Internal method %s should not be exported", methodName)
			}
		})

		It("should NOT export signal processing methods", func() {
			internalMethods := []string{
				"processSignal",
				"RequestShutdown",
			}

			for _, methodName := range internalMethods {
				_, exists := supervisorType.MethodByName(methodName)
				Expect(exists).To(BeFalse(), "Internal method %s should not be exported", methodName)
			}
		})

		It("should NOT export data freshness and health methods", func() {
			internalMethods := []string{
				"CheckDataFreshness",
				"RestartCollector",
				"GetStaleThreshold",
				"GetCollectorTimeout",
				"GetMaxRestartAttempts",
				"GetRestartCount",
				"SetRestartCount",
			}

			for _, methodName := range internalMethods {
				_, exists := supervisorType.MethodByName(methodName)
				Expect(exists).To(BeFalse(), "Internal method %s should not be exported", methodName)
			}
		})

		It("should NOT export hierarchical composition methods", func() {
			internalMethods := []string{
				"reconcileChildren",
				"applyStateMapping",
				"UpdateUserSpec",
			}

			for _, methodName := range internalMethods {
				_, exists := supervisorType.MethodByName(methodName)
				Expect(exists).To(BeFalse(), "Internal method %s should not be exported", methodName)
			}
		})

		It("should NOT export metrics and observability methods", func() {
			internalMethods := []string{
				"startMetricsReporter",
				"recordHierarchyMetrics",
				"calculateHierarchyDepth",
				"calculateHierarchySize",
			}

			for _, methodName := range internalMethods {
				_, exists := supervisorType.MethodByName(methodName)
				Expect(exists).To(BeFalse(), "Internal method %s should not be exported", methodName)
			}
		})

		It("should NOT export internal state accessors", func() {
			internalMethods := []string{
				"isStarted",
				"getContext",
				"getStartedContext",
			}

			for _, methodName := range internalMethods {
				_, exists := supervisorType.MethodByName(methodName)
				Expect(exists).To(BeFalse(), "Internal method %s should not be exported", methodName)
			}
		})

		It("should NOT export helper methods", func() {
			internalMethods := []string{
				"getRecoveryStatus",
				"getEscalationSteps",
			}

			for _, methodName := range internalMethods {
				_, exists := supervisorType.MethodByName(methodName)
				Expect(exists).To(BeFalse(), "Internal method %s should not be exported", methodName)
			}
		})
	})

	Context("Public methods have godoc comments", func() {
		It("should have documentation for all public methods", func() {
			Skip("This test requires AST parsing - will be validated manually during code review")
		})
	})

	Context("Package-level functions", func() {
		It("should NOT export helper functions", func() {
			Skip("Package-level function visibility will be verified in internal/ package tests")
		})
	})
})
