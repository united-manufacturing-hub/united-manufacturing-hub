package communication_state

import (
	"sync"

	v2 "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/pull"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/push"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/fail"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/router"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/subscriber"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/models"
)

type CommunicationState struct {
	LoginResponse     *v2.LoginResponse
	mu                sync.Mutex
	Watchdog          *watchdog.Watchdog
	InboundChannel    chan *models.UMHMessage
	InsecureTLS       bool
	Puller            *pull.Puller
	Pusher            *push.Pusher
	SubscriberHandler *subscriber.SubscriberHandler
	OutboundChannel   chan *models.UMHMessage
	Router            *router.Router
	ReleaseChannel    config.ReleaseChannel
}

// InitialiseAndStartPuller creates a new Puller and starts it
func (c *CommunicationState) InitialiseAndStartPuller() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.LoginResponse == nil {
		fail.Fatalf("LoginResponse is nil, cannot start puller")
	}
	if c.Watchdog == nil {
		fail.Fatalf("Watchdog is nil, cannot start puller")
	}
	c.Puller = pull.NewPuller(c.LoginResponse.JWT, c.Watchdog, c.InboundChannel, c.InsecureTLS)
	if c.Puller == nil {
		fail.Fatalf("Failed to create puller")
	}
	c.Puller.Start()
}

// InitialiseAndStartPusher creates a new Pusher and starts it
func (c *CommunicationState) InitialiseAndStartPusher() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.LoginResponse == nil {
		fail.Fatalf("LoginResponse is nil, cannot start pusher")
	}
	if c.Watchdog == nil {
		fail.Fatalf("Watchdog is nil, cannot start pusher")
	}
	c.Pusher = push.NewPusher(c.LoginResponse.UUID, c.LoginResponse.JWT, c.Watchdog, c.OutboundChannel, push.DefaultDeadLetterChanBuffer(), push.DefaultBackoffPolicy(), c.InsecureTLS)
	if c.Pusher == nil {
		fail.Fatalf("Failed to create pusher")
	}
	c.Pusher.Start()
}

// InitialiseAndStartRouter creates a new Router and starts it
func (c *CommunicationState) InitialiseAndStartRouter() {
	if c.Puller == nil {
		fail.Fatalf("Puller is nil, cannot start router")
	}
	if c.Pusher == nil {
		fail.Fatalf("Pusher is nil, cannot start router")
	}
	if c.LoginResponse == nil {
		fail.Fatalf("LoginResponse is nil, cannot start router")
	}

	c.mu.Lock()
	c.Router = router.NewRouter(c.Watchdog, c.InboundChannel, c.LoginResponse.UUID, c.OutboundChannel, c.ReleaseChannel)
	c.mu.Unlock()
	if c.Router == nil {
		fail.Fatalf("Failed to create router")
	}
	c.Router.Start()
}
