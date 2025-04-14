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

	"github.com/google/uuid"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/generator"

	"sync"

	"github.com/united-manufacturing-hub/expiremap/v2/pkg/expiremap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

type Handler struct {
	subscribers                *expiremap.ExpireMap[string, string]
	dog                        watchdog.Iface
	pusher                     *push.Pusher
	instanceUUID               uuid.UUID
	StatusCollector            *generator.StatusCollectorType
	disableHardwareStatusCheck bool
	state                      *fsm.SystemSnapshot
	configManager              config.ConfigManager
}

func NewHandler(
	dog watchdog.Iface,
	pusher *push.Pusher,
	instanceUUID uuid.UUID,
	ttl time.Duration,
	cull time.Duration,
	releaseChannel config.ReleaseChannel,
	disableHardwareStatusCheck bool,
	state *fsm.SystemSnapshot,
	systemMu *sync.Mutex,
	configManager config.ConfigManager,
) *Handler {
	s := &Handler{}
	s.subscribers = expiremap.NewEx[string, string](cull, ttl)
	s.dog = dog
	s.pusher = pusher
	s.instanceUUID = instanceUUID
	s.state = state
	s.configManager = configManager
	s.StatusCollector = generator.NewStatusCollector(
		dog,
		state,
		systemMu,
		configManager,
	)

	return s
}

func (s *Handler) StartNotifier() {
	go s.notifySubscribers()
}

func (s *Handler) AddSubscriber(identifier string) {

	// zap.S().Debugf("Adding subscriber %s", identifier)
	s.subscribers.Set(identifier, identifier)
	s.dog.SetHasSubscribers(true)
}

func (s *Handler) GetSubscribers() []string {
	var subscribers []string
	s.subscribers.Range(func(key string, value string) bool {
		subscribers = append(subscribers, key)
		return true
	})
	s.dog.SetHasSubscribers(len(subscribers) > 0)
	return subscribers
}

func (s *Handler) notifySubscribers() {
	watcherUUID := s.dog.RegisterHeartbeat("notifySubscribers", 0, 600, true)
	var timer = time.NewTicker(time.Second)
	for {
		select {
		case <-timer.C:
			s.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_OK)
			s.notify()
			// Cycle the notification at the same speed as the pusher will push
			// This prevents large message queues from building up
			timer.Reset(1 * time.Second)
		}
	}
}

func (s *Handler) notify() {
	s.dog.SetHasSubscribers(s.subscribers.Length() > 0)
	if s.subscribers.Length() == 0 {
		//		zap.S().Debugf("No subscribers")
		return
	}

	ctx, cncl := tools.Get1SecondContext()
	statusMessage := s.StatusCollector.GenerateStatusMessage()
	if ctx.Err() != nil {
		// It is expected that the first 1-2 times this might fail, due to the systems starting up
		zap.S().Warnf("Failed to generate status message: %s", ctx.Err().Error())
		return
	}
	if statusMessage == nil {
		zap.S().Warnf("Failed to generate status message")
		return
	}

	notified := 0
	s.subscribers.Range(func(key string, value string) bool {

		message, err := encoding.EncodeMessageFromUMHInstanceToUser(models.UMHMessageContent{
			MessageType: models.Status,
			Payload:     statusMessage,
		})
		if err != nil {
			zap.S().Warnf("Failed to encrypt message for subscriber %s", key)
			return true
		}
		s.pusher.Push(models.UMHMessage{
			Content:      message,
			Email:        key,
			InstanceUUID: s.instanceUUID,
		})
		// zap.S().Debugf("Notified subscriber %s", key)
		notified++
		return true
	})
	zap.S().Debugf("Notified %d subscribers", notified)
	cncl()
}
