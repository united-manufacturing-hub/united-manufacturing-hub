package api_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api"
)

var _ = Describe("Checking API reachability", func() {
	When("API is reachable", func() {
		It("can be reached", func() {
			Eventually(func() bool {
				return api.CheckIfAPIIsReachable(false)
			}, "5m", "1s").Should(BeTrue())
		})
	})
})
