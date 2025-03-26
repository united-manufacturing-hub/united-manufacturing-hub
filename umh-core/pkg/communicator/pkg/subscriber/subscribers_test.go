package subscriber_test

import (
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/ManagementConsole/companion/cmd/appstate"
	"github.com/united-manufacturing-hub/ManagementConsole/companion/cmd/helper/config"
	"github.com/united-manufacturing-hub/ManagementConsole/companion/cmd/models"
	"github.com/united-manufacturing-hub/ManagementConsole/companion/cmd/subscriber"
	"github.com/united-manufacturing-hub/ManagementConsole/shared/helper"
	models2 "github.com/united-manufacturing-hub/ManagementConsole/shared/models"
)

var _ = Describe("Probes", func() {
	var state *appstate.ApplicationState

	BeforeEach(func() {
		helper.Setenv("DEMO_MODE", "true")
		state, _, _ = appstate.NewTestAppState()
		state.SetLoginResponse(&models.LoginResponse{
			JWT:  "valid-jwt",
			UUID: uuid.New(),
			Name: "some-name",
		})
		state.InitialiseAndStartPuller()
		state.InitialiseAndStartPusher()
		state.InitialiseAndStartRouter()
		cfg := config.GetConfig(state.ResourceController.Cache)
		state.InitialiseAndStartSubscriberHandler(time.Second*10, time.Second, cfg)
	})

	It("should add a subscriber", func() {
		state.SubscriberHandler.AddSubscriber("alice", nil)
		Eventually(func() map[string]*subscriber.SubscriberPermissions {
			return state.SubscriberHandler.GetSubscribers()
		}).Should(HaveKey("alice"))
	})

	It("should drop subscriber after 20s", func() {
		state.SubscriberHandler.AddSubscriber("alice", nil)
		Eventually(func() map[string]*subscriber.SubscriberPermissions {
			return state.SubscriberHandler.GetSubscribers()
		}).Should(HaveKey("alice"))
		Eventually(func() map[string]*subscriber.SubscriberPermissions {
			return state.SubscriberHandler.GetSubscribers()
		}, "20s").ShouldNot(HaveKey("alice"))
	})

	// This is slow in GH actions, so we need to retry
	It("should notify subscribers", func() {
		state.SubscriberHandler.AddSubscriber("alice", nil)
		Eventually(func() map[string]*subscriber.SubscriberPermissions {
			return state.SubscriberHandler.GetSubscribers()
		}).Should(HaveKey("alice"))
		Eventually(func() *models2.UMHMessage {
			timeout := time.NewTicker(1 * time.Millisecond)
			select {
			case <-timeout.C:
				return nil
			case msg := <-state.OutboundChannel:
				return msg
			}
		}, "20s").ShouldNot(BeNil())
	})
})
