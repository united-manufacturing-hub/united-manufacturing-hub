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
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

var _ = Describe("InfrastructureHealthChecker", func() {
	var checker *supervisor.InfrastructureHealthChecker

	BeforeEach(func() {
		checker = supervisor.NewInfrastructureHealthChecker(5, 5*time.Minute)
	})

	Describe("CheckChildConsistency", func() {
		It("returns nil when no children provided", func() {
			err := checker.CheckChildConsistency(nil)
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns nil when all children are healthy", func() {
			healthyChild1 := supervisor.CreateTestSupervisorWithCircuitState(false)
			healthyChild2 := supervisor.CreateTestSupervisorWithCircuitState(false)

			children := map[string]supervisor.SupervisorInterface{
				"child1": healthyChild1,
				"child2": healthyChild2,
			}

			err := checker.CheckChildConsistency(children)
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns error when a child has circuit breaker open", func() {
			healthyChild := supervisor.CreateTestSupervisorWithCircuitState(false)
			unhealthyChild := supervisor.CreateTestSupervisorWithCircuitState(true)

			children := map[string]supervisor.SupervisorInterface{
				"healthy":   healthyChild,
				"unhealthy": unhealthyChild,
			}

			err := checker.CheckChildConsistency(children)
			Expect(err).To(HaveOccurred())

			var childHealthErr *supervisor.ChildHealthError
			Expect(err).To(BeAssignableToTypeOf(childHealthErr))

			healthErr := &supervisor.ChildHealthError{}
			ok := errors.As(err, &healthErr)
			Expect(ok).To(BeTrue())
			Expect(healthErr.ChildName).To(Equal("unhealthy"))
		})

		It("should skip nil children gracefully (defensive)", func() {
			children := map[string]supervisor.SupervisorInterface{
				"healthy":   supervisor.CreateTestSupervisorWithCircuitState(false),
				"nil":       nil,
				"unhealthy": supervisor.CreateTestSupervisorWithCircuitState(true),
			}

			err := checker.CheckChildConsistency(children)

			Expect(err).To(HaveOccurred())
			Expect(err).To(BeAssignableToTypeOf(&supervisor.ChildHealthError{}))
			healthErr := func() *supervisor.ChildHealthError {
				target := &supervisor.ChildHealthError{}
				_ = errors.As(err, &target)

				return target
			}()
			Expect(healthErr.ChildName).To(Equal("unhealthy"))
		})
	})
})
