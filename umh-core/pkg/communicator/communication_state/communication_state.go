// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package communication_state

import (
	"sync"
	"time"

	v2 "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/pull"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/push"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/subscriber"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/router"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
)

type CommunicationState struct {
	LoginResponse         *v2.LoginResponse
	LoginResponseMu       *sync.RWMutex
	mu                    *sync.RWMutex
	Watchdog              *watchdog.Watchdog
	InboundChannel        chan *models.UMHMessage
	InsecureTLS           bool
	Puller                *pull.Puller
	Pusher                *push.Pusher
	SubscriberHandler     *subscriber.Handler
	OutboundChannel       chan *models.UMHMessage
	Router                *router.Router
	ReleaseChannel        config.ReleaseChannel
	SystemSnapshotManager *fsm.SnapshotManager
	ConfigManager         config.ConfigManager
	ApiUrl                string
	Logger                *zap.SugaredLogger
	TopicBrowserCache     *topicbrowser.Cache
}

// NewCommunicationState creates a new CommunicationState with initialized mutex
func NewCommunicationState(
	watchdog *watchdog.Watchdog,
	inboundChannel chan *models.UMHMessage,
	outboundChannel chan *models.UMHMessage,
	releaseChannel config.ReleaseChannel,
	systemSnapshotManager *fsm.SnapshotManager,
	configManager config.ConfigManager,
	apiUrl string,
	logger *zap.SugaredLogger,
	insecureTLS bool,
	topicBrowserCache *topicbrowser.Cache,
) *CommunicationState {
	return &CommunicationState{
		mu:                    &sync.RWMutex{},
		LoginResponseMu:       &sync.RWMutex{},
		Watchdog:              watchdog,
		InboundChannel:        inboundChannel,
		OutboundChannel:       outboundChannel,
		ReleaseChannel:        releaseChannel,
		SystemSnapshotManager: systemSnapshotManager,
		ConfigManager:         configManager,
		ApiUrl:                apiUrl,
		Logger:                logger,
		InsecureTLS:           insecureTLS,
		TopicBrowserCache:     topicBrowserCache,
	}
}

// InitialiseAndStartPuller creates a new Puller and starts it
func (c *CommunicationState) InitialiseAndStartPuller() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LoginResponseMu.RLock()
	defer c.LoginResponseMu.RUnlock()
	if c.LoginResponse == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, c.Logger, "LoginResponse is nil, cannot start puller")
		return
	}
	if c.Watchdog == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, c.Logger, "Watchdog is nil, cannot start puller")
		return
	}
	c.Puller = pull.NewPuller(c.LoginResponse.JWT, c.Watchdog, c.InboundChannel, c.InsecureTLS, c.ApiUrl, c.Logger)
	if c.Puller == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, c.Logger, "Failed to create puller")
	}
	c.Puller.Start()
}

// InitialiseAndStartPusher creates a new Pusher and starts it
func (c *CommunicationState) InitialiseAndStartPusher() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LoginResponseMu.RLock()
	defer c.LoginResponseMu.RUnlock()
	if c.LoginResponse == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, c.Logger, "LoginResponse is nil, cannot start pusher")
		return
	}
	if c.Watchdog == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, c.Logger, "Watchdog is nil, cannot start pusher")
		return
	}
	c.Pusher = push.NewPusher(c.LoginResponse.UUID, c.LoginResponse.JWT, c.Watchdog, c.OutboundChannel, push.DefaultDeadLetterChanBuffer(), push.DefaultBackoffPolicy(), c.InsecureTLS, c.ApiUrl, c.Logger)
	if c.Pusher == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, c.Logger, "Failed to create pusher")
	}
	c.Pusher.Start()
}

// InitialiseAndStartRouter creates a new Router and starts it
func (c *CommunicationState) InitialiseAndStartRouter() {
	if c.Puller == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, c.Logger, "Puller is nil, cannot start router")
		return
	}
	if c.Pusher == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, c.Logger, "Pusher is nil, cannot start router")
		return
	}
	if c.LoginResponse == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, c.Logger, "LoginResponse is nil, cannot start router")
		return
	}

	c.mu.Lock()
	c.LoginResponseMu.RLock()
	c.Router = router.NewRouter(c.Watchdog, c.InboundChannel, c.LoginResponse.UUID, c.OutboundChannel, c.ReleaseChannel, c.SubscriberHandler, c.SystemSnapshotManager, c.ConfigManager, c.Logger)
	c.LoginResponseMu.RUnlock()
	c.mu.Unlock()
	if c.Router == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, c.Logger, "Failed to create router")
	}
	c.Router.Start()
}

// InitialiseAndStartSubscriberHandler creates a new subscriber handler and starts it
// ttl is the time until a subscriber is considered dead (if no new subscriber message is received)
// cull is the cycle time to remove dead subscribers
func (c *CommunicationState) InitialiseAndStartSubscriberHandler(ttl time.Duration, cull time.Duration, config *config.FullConfig, systemSnapshotManager *fsm.SnapshotManager, configManager config.ConfigManager) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LoginResponseMu.RLock()
	defer c.LoginResponseMu.RUnlock()

	if c.Watchdog == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, c.Logger, "Watchdog is nil, cannot start subscriber handler")
		return
	}

	if c.Pusher == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, c.Logger, "Pusher is nil, cannot start subscriber handler")
		return
	}
	if c.LoginResponse == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, c.Logger, "LoginResponse is nil, cannot start subscriber handler")
		return
	}
	if config == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, c.Logger, "Config is nil, cannot start subscriber handler")
		return
	}

	c.SubscriberHandler = subscriber.NewHandler(
		c.Watchdog,
		c.Pusher,
		c.LoginResponse.UUID,
		ttl,
		cull,
		c.ReleaseChannel,
		false, // disableHardwareStatusCheck
		systemSnapshotManager,
		configManager,
		c.Logger,
		c.TopicBrowserCache,
	)
	if c.SubscriberHandler == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, c.Logger, "Failed to create subscriber handler")
	}
	c.SubscriberHandler.StartNotifier()
}

func (c *CommunicationState) InitialiseReAuthHandler(authToken string, insecureTLS bool) {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)

		// Register a watchdog with a timeout of 3 hours, allowing up to 3 ticks, before it fails.
		watchUUID := c.Watchdog.RegisterHeartbeat("communicationstate-re-auth-handler", 0, uint64((3 * time.Hour).Seconds()), false)
		for {
			<-ticker.C
			c.Logger.Debugf("Re-fetching login credentials")
			credentials := v2.NewLogin(authToken, insecureTLS, c.ApiUrl, c.Logger)
			if credentials == nil {
				continue
			}
			c.Watchdog.ReportHeartbeatStatus(watchUUID, watchdog.HEARTBEAT_STATUS_OK)

			c.mu.Lock()
			c.LoginResponseMu.Lock()
			c.LoginResponse = credentials

			if c.Puller != nil {
				c.Puller.UpdateJWT(c.LoginResponse.JWT)
			}
			if c.Pusher != nil {
				c.Pusher.UpdateJWT(c.LoginResponse.JWT)
			}
			c.LoginResponseMu.Unlock()
			c.mu.Unlock()
		}

		// The ticker will run for the lifetime of our program, therefore no cleanup is required.
	}()
}
