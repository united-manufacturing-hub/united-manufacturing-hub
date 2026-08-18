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

	"github.com/google/uuid"

	v2 "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/subscriber"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/router"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	topicbrowserfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
)

type CommunicationState struct {
	ConfigManager         config.ConfigManager
	LoginResponse         *v2.LoginResponse
	LoginResponseMu       *sync.RWMutex
	mu                    *sync.RWMutex
	Watchdog              *watchdog.Watchdog
	InboundChannel        chan *models.UMHMessage
	SubscriberHandler     *subscriber.Handler
	OutboundChannel       chan *models.UMHMessage
	Router                *router.Router
	SystemSnapshotManager *fsm.SnapshotManager
	Logger                *zap.SugaredLogger
	TopicBrowserCache     *topicbrowser.Cache
	// TopicBrowserSimulator is used to access the simulated topic browser state if the agent is running in simulator mode
	// it is accessed by the generator to generate the topic browser part of the status message
	TopicBrowserSimulator *topicbrowser.Simulator
	FeatureUsage          *models.FeatureUsage
	ReleaseChannel        config.ReleaseChannel
	// TopicBrowserSimulatorEnabled tracks whether simulator mode is enabled
	TopicBrowserSimulatorEnabled bool
}

// NewCommunicationState creates a new CommunicationState with initialized mutex.
func NewCommunicationState(
	watchdog *watchdog.Watchdog,
	inboundChannel chan *models.UMHMessage,
	outboundChannel chan *models.UMHMessage,
	releaseChannel config.ReleaseChannel,
	systemSnapshotManager *fsm.SnapshotManager,
	configManager config.ConfigManager,
	logger *zap.SugaredLogger,
	topicBrowserCache *topicbrowser.Cache,
	featureUsage *models.FeatureUsage,
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
		Logger:                logger,
		TopicBrowserCache:     topicBrowserCache,
		FeatureUsage:          featureUsage,
	}
}

// InitializeTopicBrowserSimulator initializes the topic browser simulator
// The cache update logic has been moved to the subscriber notification pipeline
// to eliminate the redundant ticker (architectural improvement).
func (c *CommunicationState) InitializeTopicBrowserSimulator(runSimulator bool) {
	c.TopicBrowserSimulatorEnabled = runSimulator

	if runSimulator {
		c.TopicBrowserSimulator = topicbrowser.NewSimulator()
		c.TopicBrowserSimulator.InitializeSimulator()
	}
}

// UpdateTopicBrowserCache updates the topic browser cache with the latest observed state
// This is called from the subscriber notification pipeline to consolidate the ticker logic.
func (c *CommunicationState) UpdateTopicBrowserCache() error {
	if c.TopicBrowserSimulatorEnabled {
		c.TopicBrowserSimulator.Tick()

		result, err := c.TopicBrowserCache.ProcessIncrementalUpdates(c.TopicBrowserSimulator.GetSimObservedState())
		if err != nil {
			c.Logger.Errorf("Failed to update topic browser cache: %v", err)

			return err
		}
		// Update sent timestamp if we processed new data
		if !result.LatestTimestamp.IsZero() {
			c.TopicBrowserCache.SetLastSentTimestamp(result.LatestTimestamp)
		}
	} else {
		// get observed state from system snapshot manager
		tbInstance, ok := fsm.FindInstance(c.SystemSnapshotManager.GetDeepCopySnapshot(), constants.TopicBrowserManagerName, constants.TopicBrowserInstanceName)
		if !ok || tbInstance == nil {
			c.Logger.Error("Topic browser instance not found")

			return nil // Not an error, just not ready yet
		}

		tbObservedState, ok := tbInstance.LastObservedState.(*topicbrowserfsm.ObservedStateSnapshot)
		if !ok || tbObservedState == nil {
			c.Logger.Error("Topic browser observed state not found")

			return nil // Not an error, just not ready yet
		}

		result, err := c.TopicBrowserCache.ProcessIncrementalUpdates(tbObservedState)
		if err != nil {
			c.Logger.Errorf("Failed to update topic browser cache: %v", err)

			return err
		}
		// Update sent timestamp if we processed new data
		if !result.LatestTimestamp.IsZero() {
			c.TopicBrowserCache.SetLastSentTimestamp(result.LatestTimestamp)
		}
	}

	return nil
}

// InitialiseAndStartSubscriberHandler creates a new subscriber handler and starts it
// ttl is the time until a subscriber is considered dead (if no new subscriber message is received)
// cull is the cycle time to remove dead subscribers.
func (c *CommunicationState) InitialiseAndStartSubscriberHandler(ttl time.Duration, cull time.Duration, config *config.FullConfig, systemSnapshotManager *fsm.SnapshotManager, configManager config.ConfigManager, fsmOutboundChannel chan<- *types.UMHMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.LoginResponseMu.RLock()
	defer c.LoginResponseMu.RUnlock()

	if c.Watchdog == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, c.Logger, "Watchdog is nil, cannot start subscriber handler")

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

	// Create topic browser communicator (replace cache and simulator)
	var topicBrowserCommunicator *topicbrowser.TopicBrowserCommunicator
	if c.TopicBrowserSimulatorEnabled {
		topicBrowserCommunicator = topicbrowser.NewTopicBrowserCommunicatorWithSimulator(c.Logger)
	} else {
		topicBrowserCommunicator = topicbrowser.NewTopicBrowserCommunicator(c.Logger)
	}

	c.SubscriberHandler = subscriber.NewHandler(
		c.Watchdog,
		c.LoginResponse.UUID,
		ttl,
		cull,
		c.ReleaseChannel,
		false, // disableHardwareStatusCheck
		systemSnapshotManager,
		configManager,
		c.Logger,
		topicBrowserCommunicator,
		fsmOutboundChannel, // FSMv2 direct channel for status delivery
		c.FeatureUsage,
	)
	if c.SubscriberHandler == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, c.Logger, "Failed to create subscriber handler")
	}

	c.SubscriberHandler.StartNotifier()
}

// SetLoginResponseForFSMv2 sets a minimal LoginResponse needed by the Router for FSMv2 mode.
// FSMv2 handles authentication separately, so we just need the instance UUID.
// Also updates the SubscriberHandler's and Router's instanceUUID if they exist (Bug #6, #8 fix).
//
// Lock ordering: Acquires mu.RLock() FIRST, then LoginResponseMu.Lock() SECOND.
// This consistent ordering prevents deadlocks. Bug #7 fix.
func (c *CommunicationState) SetLoginResponseForFSMv2(instanceUUID string) {
	// Acquire mu first (consistent lock ordering with other methods)
	c.mu.RLock()
	subscriberHandler := c.SubscriberHandler
	router := c.Router
	c.mu.RUnlock()

	// Now acquire LoginResponseMu to update LoginResponse
	c.LoginResponseMu.Lock()
	defer c.LoginResponseMu.Unlock()

	parsedUUID, err := parseUUIDForFSMv2(instanceUUID)
	if err != nil {
		c.Logger.Warnw("Failed to parse instance UUID for FSMv2, using nil UUID", "error", err)
	}

	c.LoginResponse = &v2.LoginResponse{
		UUID: parsedUUID,
		JWT:  "", // Empty JWT - FSMv2 handles auth
		Name: "FSMv2 Instance",
	}

	// Update SubscriberHandler's instanceUUID if it exists (Bug #6 fix)
	// We read subscriberHandler outside the lock, so it's safe to call SetInstanceUUID
	// (which is thread-safe due to Bug #7 fix)
	if subscriberHandler != nil {
		subscriberHandler.SetInstanceUUID(parsedUUID)
		c.Logger.Infow("Updated SubscriberHandler with backend UUID", "uuid", parsedUUID)
	}

	// Update Router's instanceUUID if it exists (Bug #8 fix)
	// The Router was created with a placeholder UUID, and now we have the real UUID
	// from the backend authentication response.
	if router != nil {
		router.SetInstanceUUID(parsedUUID)
		c.Logger.Infow("Updated Router with backend UUID", "uuid", parsedUUID)
	}
}

// InitializeRouterForFSMv2 initializes the Router for FSMv2 mode.
// FSMv2 handles pulling via its own transport worker, so no Puller is required.
func (c *CommunicationState) InitializeRouterForFSMv2() {
	if c.LoginResponse == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, c.Logger, "LoginResponse is nil, cannot start router for FSMv2")

		return
	}

	c.mu.Lock()
	c.LoginResponseMu.RLock()
	// Note: Puller is nil for FSMv2 mode - FSMv2 handles pulling via PullWorker
	c.Router = router.NewRouter(c.Watchdog, c.InboundChannel, c.LoginResponse.UUID, c.OutboundChannel, c.ReleaseChannel, c.SubscriberHandler, c.SystemSnapshotManager, c.ConfigManager, c.Logger)
	c.LoginResponseMu.RUnlock()
	c.mu.Unlock()

	if c.Router == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, c.Logger, "Failed to create router for FSMv2")

		return
	}

	c.Router.Start()
}

// parseUUIDForFSMv2 attempts to parse a UUID string, returning uuid.Nil on failure.
func parseUUIDForFSMv2(uuidStr string) (uuid.UUID, error) {
	if uuidStr == "" {
		return uuid.Nil, nil
	}

	return uuid.Parse(uuidStr)
}
