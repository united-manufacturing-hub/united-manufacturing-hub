package supervisor_test

import (
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
			Expect(err).To(BeNil())
		})

		It("returns nil when all children are healthy", func() {
			healthyChild1 := supervisor.CreateTestSupervisorWithCircuitState(false)
			healthyChild2 := supervisor.CreateTestSupervisorWithCircuitState(false)

			children := map[string]*supervisor.Supervisor{
				"child1": healthyChild1,
				"child2": healthyChild2,
			}

			err := checker.CheckChildConsistency(children)
			Expect(err).To(BeNil())
		})

		It("returns error when a child has circuit breaker open", func() {
			healthyChild := supervisor.CreateTestSupervisorWithCircuitState(false)
			unhealthyChild := supervisor.CreateTestSupervisorWithCircuitState(true)

			children := map[string]*supervisor.Supervisor{
				"healthy":   healthyChild,
				"unhealthy": unhealthyChild,
			}

			err := checker.CheckChildConsistency(children)
			Expect(err).ToNot(BeNil())

			var childHealthErr *supervisor.ChildHealthError
			Expect(err).To(BeAssignableToTypeOf(childHealthErr))

			healthErr, ok := err.(*supervisor.ChildHealthError)
			Expect(ok).To(BeTrue())
			Expect(healthErr.ChildName).To(Equal("unhealthy"))
		})

		It("should skip nil children gracefully (defensive)", func() {
			children := map[string]*supervisor.Supervisor{
				"healthy":   supervisor.CreateTestSupervisorWithCircuitState(false),
				"nil":       nil,
				"unhealthy": supervisor.CreateTestSupervisorWithCircuitState(true),
			}

			err := checker.CheckChildConsistency(children)

			Expect(err).To(HaveOccurred())
			Expect(err).To(BeAssignableToTypeOf(&supervisor.ChildHealthError{}))
			healthErr := err.(*supervisor.ChildHealthError)
			Expect(healthErr.ChildName).To(Equal("unhealthy"))
		})
	})
})
