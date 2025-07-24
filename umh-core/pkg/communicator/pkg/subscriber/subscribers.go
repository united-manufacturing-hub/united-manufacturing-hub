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

package subscriber

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/push"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"

	"github.com/google/uuid"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/generator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/subscribers"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

type Handler struct {
	dog                        watchdog.Iface
	configManager              config.ConfigManager
	subscriberRegistry         *subscribers.Registry
	pusher                     *push.Pusher
	StatusCollector            *generator.StatusCollectorType
	systemSnapshotManager      *fsm.SnapshotManager
	topicBrowserCommunicator   *topicbrowser.TopicBrowserCommunicator
	logger                     *zap.SugaredLogger
	instanceUUID               uuid.UUID
	disableHardwareStatusCheck bool //nolint:unused // will be used in the future
}

func NewHandler(
	dog watchdog.Iface,
	pusher *push.Pusher,
	instanceUUID uuid.UUID,
	ttl time.Duration,
	cull time.Duration,
	releaseChannel config.ReleaseChannel,
	disableHardwareStatusCheck bool,
	systemSnapshotManager *fsm.SnapshotManager,
	configManager config.ConfigManager,
	logger *zap.SugaredLogger,
	topicBrowserCommunicator *topicbrowser.TopicBrowserCommunicator,
) *Handler {
	s := &Handler{}
	s.subscriberRegistry = subscribers.NewRegistry(cull, ttl)
	s.dog = dog
	s.pusher = pusher
	s.instanceUUID = instanceUUID
	s.systemSnapshotManager = systemSnapshotManager
	s.configManager = configManager
	s.topicBrowserCommunicator = topicBrowserCommunicator
	s.logger = logger
	s.StatusCollector = generator.NewStatusCollector(
		dog,
		systemSnapshotManager,
		configManager,
		logger,
		topicBrowserCommunicator,
	)

	return s
}

func (s *Handler) StartNotifier() {
	go s.notifySubscribers()
}

func (s *Handler) AddOrRefreshSubscriber(identifier string, bootstrapped bool) {
	s.subscriberRegistry.AddOrRefresh(identifier, bootstrapped)
	s.dog.SetHasSubscribers(true)
}

func (s *Handler) GetSubscribers() []string {
	subscribers := s.subscriberRegistry.List()
	s.dog.SetHasSubscribers(len(subscribers) > 0)
	return subscribers
}

func (s *Handler) notifySubscribers() {
	watcherUUID := s.dog.RegisterHeartbeat("notifySubscribers", 0, 600, true)
	var timer = time.NewTicker(time.Second)
	defer timer.Stop()

	for range timer.C {
		s.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_OK)
		s.notify()
		// The ticker will automatically fire at 1 second intervals
		// This prevents large message queues from building up
	}
}

func (s *Handler) notify() {
	s.dog.SetHasSubscribers(s.subscriberRegistry.Length() > 0)
	if s.subscriberRegistry.Length() == 0 {
		return
	}

	// Update Topic Browser cache before generating status messages (Phase 2 architectural improvement)
	// This consolidates the cache update logic into the single notification ticker
	err := s.StatusCollector.UpdateTopicBrowserCache()
	if err != nil {
		s.logger.Warnf("Failed to update topic browser cache: %v", err)
		// Continue with status generation even if cache update fails
	}

	ctx, cncl := tools.Get1SecondContext()
	defer cncl()

	notified := 0
	baseStatusMessage := s.StatusCollector.GenerateStatusMessage(ctx, true)

	s.subscriberRegistry.ForEach(func(email string, bootstrapped bool) {
		// Generate personalized status message based on bootstrap state
		statusMessage := baseStatusMessage
		if !bootstrapped {
			// If the subscriber is not bootstrapped, we need to generate a new status message
			statusMessage = s.StatusCollector.GenerateStatusMessage(ctx, false)
		}

		if ctx.Err() != nil {
			// It is expected that the first 1-2 times this might fail, due to the systems starting up
			s.logger.Warnf("Failed to generate status message: %s", ctx.Err().Error())
			return
		}
		if statusMessage == nil {
			s.logger.Warnf("Failed to generate status message")
			return
		}

		message, err := encoding.EncodeMessageFromUMHInstanceToUser(models.UMHMessageContent{
			MessageType: models.Status,
			Payload:     statusMessage,
		})
		if err != nil {
			s.logger.Warnf("Failed to encrypt message for subscriber %s", email)
			return
		}

		s.pusher.Push(models.UMHMessage{
			Content:      message,
			Email:        email,
			InstanceUUID: s.instanceUUID,
		})

		// Mark subscriber as bootstrapped after first message
		if !bootstrapped {
			s.subscriberRegistry.SetBootstrapped(email, true)
			s.logger.Debugf("Subscriber %s has been bootstrapped", email)
		}

		notified++
	})

	// Mark data as sent for tracking purposes
	if notified > 0 && s.topicBrowserCommunicator != nil {
		s.topicBrowserCommunicator.MarkDataAsSent(time.Now())
	}
}
