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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/snapshot"
	"go.uber.org/zap"
)

type CommunicationState struct {
	LoginResponse     *v2.LoginResponse
	mu                sync.Mutex
	Watchdog          *watchdog.Watchdog
	InboundChannel    chan *models.UMHMessage
	InsecureTLS       bool
	Puller            *pull.Puller
	Pusher            *push.Pusher
	SubscriberHandler *subscriber.Handler
	OutboundChannel   chan *models.UMHMessage
	Router            *router.Router
	ReleaseChannel    config.ReleaseChannel
	SystemSnapshot    *snapshot.SystemSnapshot
	ConfigManager     config.ConfigManager
	ApiUrl            string
	Logger            *zap.SugaredLogger
}

// InitialiseAndStartPuller creates a new Puller and starts it
func (c *CommunicationState) InitialiseAndStartPuller() {
	c.mu.Lock()
	defer c.mu.Unlock()
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
	c.Router = router.NewRouter(c.Watchdog, c.InboundChannel, c.LoginResponse.UUID, c.OutboundChannel, c.ReleaseChannel, c.SubscriberHandler, c.SystemSnapshot, c.ConfigManager, c.Logger)
	c.mu.Unlock()
	if c.Router == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, c.Logger, "Failed to create router")
	}
	c.Router.Start()
}

// InitialiseAndStartSubscriberHandler creates a new subscriber handler and starts it
// ttl is the time until a subscriber is considered dead (if no new subscriber message is received)
// cull is the cycle time to remove dead subscribers
func (s *CommunicationState) InitialiseAndStartSubscriberHandler(ttl time.Duration, cull time.Duration, config *config.FullConfig, state *snapshot.SystemSnapshot, systemMu *sync.Mutex, configManager config.ConfigManager) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Watchdog == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, s.Logger, "Watchdog is nil, cannot start subscriber handler")
		return
	}

	if s.Pusher == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, s.Logger, "Pusher is nil, cannot start subscriber handler")
		return
	}
	if s.LoginResponse == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, s.Logger, "LoginResponse is nil, cannot start subscriber handler")
		return
	}
	if config == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, s.Logger, "Config is nil, cannot start subscriber handler")
		return
	}

	s.SubscriberHandler = subscriber.NewHandler(
		s.Watchdog,
		s.Pusher,
		s.LoginResponse.UUID,
		ttl,
		cull,
		s.ReleaseChannel,
		false, // disableHardwareStatusCheck
		state,
		systemMu,
		configManager,
		s.Logger,
	)
	if s.SubscriberHandler == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, s.Logger, "Failed to create subscriber handler")
	}
	s.SubscriberHandler.StartNotifier()
}
