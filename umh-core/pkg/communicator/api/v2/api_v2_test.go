package v2_test

import (
	"github.com/h2non/gock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/mocks"
	v2 "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/helper"
)

var _ = Describe("ApiV2", Ordered, Serial, func() {
	BeforeEach(func() {

		By("setting the API_URL environment variable")
		// Set API_URL to the production API URL in order for the gock library to correctly
		// intercept the requests to the production API done by the code under test
		helper.Setenv("API_URL", "https://management.umh.app/api")
		var err error
		Expect(err).ToNot(HaveOccurred())

		// Mock the login endpoint every time a test is run
		mocks.MockLogin()
	})

	AfterEach(func() {
		// Ensure that all gock mocks are turned off after each test, even the unmatched ones
		gock.OffAll()
	})

	// The tests in this context are ported from the old login_test.go file
	Context("SetLoginResponse", func() {
		It("should return a response for a valid token", func() {
			response := v2.NewLogin(AuthToken, false)
			Expect(response).ToNot(BeNil())
			Expect(response.JWT).ToNot(BeEmpty())
			Expect(response.UUID).ToNot(BeEmpty())
			Expect(response.Name).ToNot(BeEmpty())
		})
	})

})
