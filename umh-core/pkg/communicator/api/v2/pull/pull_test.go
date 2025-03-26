package pull_test

// https://github.com/united-manufacturing-hub/MgmtIssues/issues/1970
// These tests are currently disabled because they fail without an error message.
// This failure however is not reproducible in a local environment.

import (
	"context"

	"github.com/united-manufacturing-hub/ManagementConsole/shared/backend_api_structs"
	"github.com/united-manufacturing-hub/ManagementConsole/shared/encoding"

	"github.com/google/uuid"
	"github.com/h2non/gock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/ManagementConsole/companion/cmd/api/mocks"
	"github.com/united-manufacturing-hub/ManagementConsole/companion/cmd/appstate"
	models2 "github.com/united-manufacturing-hub/ManagementConsole/companion/cmd/models"
	"github.com/united-manufacturing-hub/ManagementConsole/shared/helper"
	"github.com/united-manufacturing-hub/ManagementConsole/shared/models"
)

var _ = Describe("Requester", func() {
	var state *appstate.ApplicationState
	var cncl context.CancelFunc
	BeforeEach(func() {
		By("setting the DEMO_MODE environment variable")
		// Set DEMO_MODE to true in order to use the fake kube clientset in this suite
		// instead of a real one
		helper.Setenv("DEMO_MODE", "true")

		By("setting the API_URL environment variable")
		// Set API_URL to the production API URL in order for the gock library to correctly
		// intercept the requests to the production API done by the code under test
		helper.Setenv("API_URL", "https://management.umh.app/api")
		state, cncl, _ = appstate.NewTestAppState()
		state.SetLoginResponse(&models2.LoginResponse{JWT: "valid-jwt"})
	})

	AfterEach(func() {
		cncl()
	})

	// The tests in this context are ported from the old pull_test.go file
	Context("Pull", func() {
		BeforeEach(func() {
			By("Mocking the subscribe endpoint with a status message payload")
			messageContent, err := encoding.EncodeMessageFromUserToUMHInstance(models.UMHMessageContent{MessageType: models.Subscribe, Payload: ""})
			Expect(err).ToNot(HaveOccurred())

			mocks.MockSubscribeMessageWithPayload(backend_api_structs.PullPayload{
				UMHMessages: []models.UMHMessage{
					{
						InstanceUUID: uuid.Nil,
						Email:        "some-email@umh.app",
						Content:      messageContent,
					},
				},
			}) // no need to defer gock.Off, as it is done in the cleanup
			state.InitialiseAndStartPuller()

			By("Mocking the user certificate endpoint")
			mocks.MockUserCertificateEndpoint()
		})

		AfterEach(func() {
			state.Puller.Stop()
			gock.Off()
		})

		It("should return a response for a valid token", func() {
			Eventually(func() int {
				return len(state.InboundChannel)
			}, "2s").Should(BeNumerically(">=", 1))
			messages := state.InboundChannel
			msg := <-messages
			Expect(msg).ToNot(BeNil())
			Expect(msg.Email).To(Equal("some-email@umh.app"))
			Expect(msg.InstanceUUID).To(Equal(uuid.Nil))
			encryptedContent, _ := encoding.EncodeMessageFromUserToUMHInstance(models.UMHMessageContent{
				Payload:     "",
				MessageType: models.Subscribe,
			})
			Expect(msg.Content).To(Equal(encryptedContent))
		})
	})
})
