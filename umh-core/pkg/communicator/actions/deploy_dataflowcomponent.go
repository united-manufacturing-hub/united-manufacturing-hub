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
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

type DeployDataFlowComponentAction struct {
	userEmail       string
	actionUUID      uuid.UUID
	instanceUUID    uuid.UUID
	outboundChannel chan *models.UMHMessage
	configManager   config.ConfigManager
	systemSnapshot  *fsm.SystemSnapshot
	name            string
	payload         models.CDFCPayload
}

// exposed for testing purposes
func NewDeployDataFlowComponentAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager, systemSnapshot *fsm.SystemSnapshot) *DeployDataFlowComponentAction {
	return &DeployDataFlowComponentAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   configManager,
		systemSnapshot:  systemSnapshot,
	}
}

func (a *DeployDataFlowComponentAction) Parse(payload interface{}) error {
	zap.S().Debug("Parsing DeployDataFlowComponent action payload")

	parsedPayload, err := ParseActionPayload[models.DeployCustomDataFlowComponentPayload](payload)
	if err != nil {
		return err
	}

	a.name = parsedPayload.Name
	a.payload = parsedPayload.Payload

	return nil
}

func (a *DeployDataFlowComponentAction) Validate() error {
	return nil
}

func (a *DeployDataFlowComponentAction) Execute() (interface{}, map[string]interface{}, error) {
	zap.S().Info("Executing DeployDataFlowComponent action")

	// Send confirmation that action is starting
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed, "Starting DeployDataFlowComponent action", a.outboundChannel, models.DeployDataFlowComponent)

	// convert the action payload to a config.DataFlowComponentConfig
	pipeline := make(map[string]interface{})
	for k, v := range a.payload.Pipeline {
		pipeline[fmt.Sprintf("%d", k)] = v
	}
	input := make(map[string]interface{})
	input["type"] = a.payload.Input.Type
	input["data"] = a.payload.Input.Data
	output := make(map[string]interface{})
	output["type"] = a.payload.Output.Type
	output["data"] = a.payload.Output.Data

	serviceConfig := benthosserviceconfig.BenthosServiceConfig{
		Pipeline:    pipeline,
		Input:       input,
		Output:      output,
		MetricsPort: 9090,
		LogLevel:    "debug",
	}
	dfcConfig := config.DataFlowComponentConfig{
		Name:          a.name,
		DesiredState:  "running",
		ServiceConfig: serviceConfig,
	}

	// Add the data flow component to the config
	err := a.configManager.AtomicAddDataFlowComponent(context.Background(), dfcConfig)
	if err != nil {
		return nil, nil, err
	}

	// observe the system snapshot to check if the data flow component is deployed
	err = a.validateDeployment()
	if err != nil {
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, "Data flow component not deployed", a.outboundChannel, models.DeployDataFlowComponent)
		return nil, nil, err
	}

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedSuccessfull, "Data flow component deployed successfully", a.outboundChannel, models.DeployDataFlowComponent)

	return nil, nil, nil
}

func (a *DeployDataFlowComponentAction) validateDeployment() error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	timeout := time.After(10 * time.Second)

	for {
		select {
		case <-ticker.C:
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, "Validating data flow component deployment...", a.outboundChannel, models.DeployDataFlowComponent)
			// check if the data flow component is deployed
			managers := a.systemSnapshot.Managers
			for _, manager := range managers {
				if manager.GetName() == "dataflowcomponent" {
					instances := manager.GetInstances()
					for _, instance := range instances {
						if instance.ID == a.name {
							zap.S().Info("Data flow component deployed")
							//check state
							if instance.CurrentState == "running" {
								return nil
							}
						}
					}
				}
			}
		case <-timeout:
			return fmt.Errorf("data flow component not deployed after 10 seconds")
		}
	}
}

func (a *DeployDataFlowComponentAction) getUserEmail() string {
	return a.userEmail
}

func (a *DeployDataFlowComponentAction) getUuid() uuid.UUID {
	return a.actionUUID
}
