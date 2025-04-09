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

package actions

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

type GetDataFlowComponentAction struct {
	userEmail       string
	actionUUID      uuid.UUID
	instanceUUID    uuid.UUID
	outboundChannel chan *models.UMHMessage
	configManager   config.ConfigManager
	systemSnapshot  *fsm.SystemSnapshot
	uuids           []uuid.UUID
}

func (a *GetDataFlowComponentAction) Parse(payload interface{}) error {
	// Convert the payload to a map
	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		SendActionReply(a.actionUUID, a.getUserEmail(), a.getUuid(), models.ActionFinishedWithFailure, "invalid payload format, expected map", a.outboundChannel, models.GetDataFlowComponent)
		return errors.New("invalid payload format, expected map")
	}

	// Extract the uuids field
	uuids, ok := payloadMap["versionUUIDs"].([]uuid.UUID)
	if !ok {
		SendActionReply(a.actionUUID, a.getUserEmail(), a.getUuid(), models.ActionFinishedWithFailure, "invalid uuids format, expected array of UUIDs", a.outboundChannel, models.GetDataFlowComponent)
		return errors.New("invalid uuids format, expected array of UUIDs")
	}

	a.uuids = uuids

	return nil
}

// validation step is empty here
func (a *GetDataFlowComponentAction) Validate() error {
	return nil
}

func (a *GetDataFlowComponentAction) Execute() (interface{}, map[string]interface{}, error) {
	SendActionReply(a.actionUUID, a.getUserEmail(), a.getUuid(), models.ActionExecuting, "Checking the config to get the DataFlowComponent", a.outboundChannel, models.GetDataFlowComponent)

	// Get the config
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()
	curConfig, err := a.configManager.GetConfig(ctx, a.systemSnapshot.Tick)
	if err != nil {
		return nil, nil, err
	}

	// Get the DataFlowComponent
	dataFlowComponents := []config.DataFlowComponentConfig{}
	for _, component := range curConfig.DataFlow {
		if slices.Contains(a.uuids, dataflowcomponentconfig.GenerateUUIDFromName(component.Name)) {
			dataFlowComponents = append(dataFlowComponents, component)
		}
	}

	// build the response
	response := models.GetDataflowcomponentResponse{}
	for _, component := range dataFlowComponents {
		response[dataflowcomponentconfig.GenerateUUIDFromName(component.FSMInstanceConfig.Name).String()] = models.GetDataflowcomponentResponseContent{
			CreationTime: 0,
			Creator:      "",
			Meta: models.CommonDataFlowComponentMeta{
				Type: "custom",
			},
			Name:      component.FSMInstanceConfig.Name,
			ParentDFC: nil,
			Payload:   component.DataFlowComponentConfig,
		}
	}

	return response, nil, nil
}

func (a *GetDataFlowComponentAction) getUserEmail() string {
	return a.userEmail
}

func (a *GetDataFlowComponentAction) getUuid() uuid.UUID {
	return a.actionUUID
}
