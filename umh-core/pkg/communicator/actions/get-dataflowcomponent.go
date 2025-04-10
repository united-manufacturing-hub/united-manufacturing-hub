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
	"slices"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

type GetDataFlowComponentAction struct {
	userEmail       string
	actionUUID      uuid.UUID
	instanceUUID    uuid.UUID
	outboundChannel chan *models.UMHMessage
	configManager   config.ConfigManager
	systemSnapshot  *fsm.SystemSnapshot
	payload         models.GetDataflowcomponentRequestSchemaJson
	actionLogger    *zap.SugaredLogger
}

func (a *GetDataFlowComponentAction) Parse(payload interface{}) (err error) {
	a.actionLogger.Info("Parsing the payload")
	a.payload, err = ParseActionPayload[models.GetDataflowcomponentRequestSchemaJson](payload)
	a.actionLogger.Info("Payload parsed, uuids: ", a.payload.VersionUUIDs)
	return err
}

// validation step is empty here
func (a *GetDataFlowComponentAction) Validate() error {
	return nil
}

func (a *GetDataFlowComponentAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing the action")
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, "getting the dataflowcomponent", a.outboundChannel, models.GetDataFlowComponent)

	// Get the config
	a.actionLogger.Info("Getting the config")
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()
	curConfig, err := a.configManager.GetConfig(ctx, a.systemSnapshot.Tick)
	if err != nil {
		return nil, nil, err
	}

	// Get the DataFlowComponent
	a.actionLogger.Info("Getting the DataFlowComponent")
	dataFlowComponents := []config.DataFlowComponentConfig{}
	for _, component := range curConfig.DataFlow {
		cur_uuid := dataflowcomponentconfig.GenerateUUIDFromName(component.Name).String()
		a.actionLogger.Info("Checking if ", cur_uuid, " is in ", a.payload.VersionUUIDs)
		if slices.Contains(a.payload.VersionUUIDs, cur_uuid) {
			a.actionLogger.Info("Adding ", component.Name, " to the response")
			dataFlowComponents = append(dataFlowComponents, component)
		}
	}

	// build the response
	a.actionLogger.Info("Building the response")
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

	// Send the success message
	//SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedSuccessfull, response, a.outboundChannel, models.GetDataFlowComponent)

	a.actionLogger.Info("Response built, returning, response: ", response)
	return "ewdwdwdwdwdwd", nil, nil
}

func (a *GetDataFlowComponentAction) getUserEmail() string {
	return a.userEmail
}

func (a *GetDataFlowComponentAction) getUuid() uuid.UUID {
	return a.actionUUID
}
