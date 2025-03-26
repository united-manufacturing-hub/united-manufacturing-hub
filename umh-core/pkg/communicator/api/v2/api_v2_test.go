package v2_test

import (
	"context"

	"github.com/h2non/gock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/ManagementConsole/companion/cmd/api/mocks"
	v2 "github.com/united-manufacturing-hub/ManagementConsole/companion/cmd/api/v2"
	"github.com/united-manufacturing-hub/ManagementConsole/companion/cmd/appstate"
	"github.com/united-manufacturing-hub/ManagementConsole/shared/helper"
	kubernetes2 "github.com/united-manufacturing-hub/ManagementConsole/shared/kubernetes"
)

var _ = Describe("ApiV2", Ordered, Serial, func() {
	var state *appstate.ApplicationState
	var ctx context.Context
	var cancel context.CancelFunc
	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		By("setting the DEMO_MODE environment variable")
		// Set DEMO_MODE to true in order to use the fake kube clientset in this suite
		// instead of a real one
		helper.Setenv("DEMO_MODE", "true")

		By("setting the API_URL environment variable")
		// Set API_URL to the production API URL in order for the gock library to correctly
		// intercept the requests to the production API done by the code under test
		helper.Setenv("API_URL", "https://management.umh.app/api")
		var err error
		state, _, _ = appstate.NewTestAppState()
		Expect(err).ToNot(HaveOccurred())

		// Mock the login endpoint every time a test is run
		mocks.MockLogin()
		state.SetLoginResponse(v2.NewLogin(kubernetes2.GetSecrets(ctx, state.ResourceController.Cache).Secret.AuthToken, false))
		state.InitialiseAndStartPusher()
	})

	AfterEach(func() {
		// Ensure that all gock mocks are turned off after each test, even the unmatched ones
		cancel()
		gock.OffAll()
	})

	// The tests in this context are ported from the old login_test.go file
	Context("SetLoginResponse", func() {
		It("should return a response for a valid token", func() {
			response := state.LoginResponse
			Expect(response).ToNot(BeNil())
			Expect(response.JWT).ToNot(BeEmpty())
			Expect(response.UUID).ToNot(BeEmpty())
			Expect(response.Name).ToNot(BeEmpty())
		})
	})

})
