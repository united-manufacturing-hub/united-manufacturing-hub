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

package generator

import (
	"errors"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	topicbrowserfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// GenerateTopicBrowserFromCommunicator generates topic browser data from the communicator
// This function uses the new TopicBrowserCommunicator to get subscriber data.
func GenerateTopicBrowserFromCommunicator(
	communicator *topicbrowser.TopicBrowserCommunicator,
	isBootstrapped bool,
	logger *zap.SugaredLogger,
	inst *fsm.FSMInstanceSnapshot,
) *models.TopicBrowser {
	if communicator == nil {
		logger.Error("Topic browser communicator is nil")

		return &models.TopicBrowser{
			Health: &models.Health{
				Message:       "Topic browser communicator not initialized",
				ObservedState: "error",
				DesiredState:  "running",
				Category:      models.Degraded,
			},
		}
	}

	// Get subscriber data from the communicator
	subscriberData, err := communicator.GetSubscriberData(isBootstrapped)
	if err != nil {
		logger.Errorf("Failed to get subscriber data from topic browser communicator: %v", err)

		return &models.TopicBrowser{
			Health: &models.Health{
				Message:       "Failed to get topic browser data",
				ObservedState: "error",
				DesiredState:  "running",
				Category:      models.Degraded,
			},
		}
	}

	// generate health from instance
	var health *models.Health
	if inst != nil {
		health, err = buildTopicBrowserAsDfc(*inst, logger)
		if err != nil {
			logger.Errorf("Failed to build topic browser as DFC: %v", err)
		}
	} else {
		health = &models.Health{
			Message:       "Topic browser operational",
			ObservedState: "running",
			DesiredState:  "running",
			Category:      models.Active,
		}
	}

	// Convert subscriber data to models.TopicBrowser
	return &models.TopicBrowser{
		Health:     health,
		TopicCount: subscriberData.TopicCount,
		UnsBundles: subscriberData.UnsBundles,
	}
}

func buildTopicBrowserAsDfc(
	instance fsm.FSMInstanceSnapshot,
	logger *zap.SugaredLogger,
) (*models.Health, error) {
	observed, ok := instance.LastObservedState.(*topicbrowserfsm.ObservedStateSnapshot)
	if !ok {
		return nil, errors.New("last observed state is not a topic browser observed state snapshot")
	}

	serviceInfo := observed.ServiceInfo

	healthCat := models.Neutral

	switch serviceInfo.BenthosFSMState {
	case topicbrowserfsm.OperationalStateActive:
		healthCat = models.Active
	case topicbrowserfsm.OperationalStateDegradedBenthos,
		topicbrowserfsm.OperationalStateDegradedRedpanda:
		healthCat = models.Degraded
	case topicbrowserfsm.OperationalStateIdle,
		topicbrowserfsm.OperationalStateStarting,
		topicbrowserfsm.OperationalStateStopping:
		healthCat = models.Neutral
	}

	return &models.Health{
		Message:       serviceInfo.StatusReason,
		ObservedState: instance.CurrentState,
		DesiredState:  "active",
		Category:      healthCat,
	}, nil
}
