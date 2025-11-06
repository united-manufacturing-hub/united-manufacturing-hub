package health_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/health"
)

var _ = Describe("InfrastructureHealthChecker", func() {
	var checker *health.InfrastructureHealthChecker

	BeforeEach(func() {
		checker = health.NewInfrastructureHealthChecker(5, 5*time.Minute)
	})

	Describe("CheckChildConsistency", func() {
		It("returns nil when no children provided", func() {
			err := checker.CheckChildConsistency(nil)
			Expect(err).To(BeNil())
		})

		It("returns nil when all children are healthy", func() {
			healthyChild1 := &supervisor.Supervisor{
				circuitOpen: false,
			}
			healthyChild2 := &supervisor.Supervisor{
				circuitOpen: false,
			}

			children := map[string]*supervisor.Supervisor{
				"child1": healthyChild1,
				"child2": healthyChild2,
			}

			err := checker.CheckChildConsistency(children)
			Expect(err).To(BeNil())
		})

		It("returns error when a child has circuit breaker open", func() {
			healthyChild := &supervisor.Supervisor{
				circuitOpen: false,
			}
			unhealthyChild := &supervisor.Supervisor{
				circuitOpen: true,
			}

			children := map[string]*supervisor.Supervisor{
				"healthy":   healthyChild,
				"unhealthy": unhealthyChild,
			}

			err := checker.CheckChildConsistency(children)
			Expect(err).ToNot(BeNil())

			var childHealthErr *health.ChildHealthError
			Expect(err).To(BeAssignableToTypeOf(childHealthErr))

			healthErr, ok := err.(*health.ChildHealthError)
			Expect(ok).To(BeTrue())
			Expect(healthErr.ChildName).To(Equal("unhealthy"))
		})

		It("should skip nil children gracefully (defensive)", func() {
			children := map[string]*supervisor.Supervisor{
				"healthy":   {circuitOpen: false},
				"nil":       nil,
				"unhealthy": {circuitOpen: true},
			}

			err := checker.CheckChildConsistency(children)

			Expect(err).To(HaveOccurred())
			Expect(err).To(BeAssignableToTypeOf(&health.ChildHealthError{}))
			healthErr := err.(*health.ChildHealthError)
			Expect(healthErr.ChildName).To(Equal("unhealthy"))
		})
	})
})
