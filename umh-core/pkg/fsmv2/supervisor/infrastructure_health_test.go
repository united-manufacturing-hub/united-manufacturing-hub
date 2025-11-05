package supervisor

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("InfrastructureHealthChecker", func() {
	var checker *InfrastructureHealthChecker

	BeforeEach(func() {
		checker = NewInfrastructureHealthChecker(5, 5*time.Minute)
	})

	Describe("CheckChildConsistency", func() {
		It("returns nil when no children provided", func() {
			err := checker.CheckChildConsistency(nil)
			Expect(err).To(BeNil())
		})

		It("returns nil when all children are healthy", func() {
			healthyChild1 := &Supervisor{
				circuitOpen: false,
			}
			healthyChild2 := &Supervisor{
				circuitOpen: false,
			}

			children := map[string]*Supervisor{
				"child1": healthyChild1,
				"child2": healthyChild2,
			}

			err := checker.CheckChildConsistency(children)
			Expect(err).To(BeNil())
		})

		It("returns error when a child has circuit breaker open", func() {
			healthyChild := &Supervisor{
				circuitOpen: false,
			}
			unhealthyChild := &Supervisor{
				circuitOpen: true,
			}

			children := map[string]*Supervisor{
				"healthy":   healthyChild,
				"unhealthy": unhealthyChild,
			}

			err := checker.CheckChildConsistency(children)
			Expect(err).ToNot(BeNil())

			var childHealthErr *ChildHealthError
			Expect(err).To(BeAssignableToTypeOf(childHealthErr))

			healthErr, ok := err.(*ChildHealthError)
			Expect(ok).To(BeTrue())
			Expect(healthErr.ChildName).To(Equal("unhealthy"))
		})
	})
})
