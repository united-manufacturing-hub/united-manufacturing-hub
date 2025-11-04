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
		It("returns nil when no children provided (stub)", func() {
			err := checker.CheckChildConsistency(nil)
			Expect(err).To(BeNil())
		})

		// More tests will be added in Phase 3 when ChildSupervisor exists
	})
})
