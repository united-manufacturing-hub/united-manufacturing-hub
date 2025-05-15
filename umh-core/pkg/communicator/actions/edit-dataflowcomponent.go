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

// Package actions contains implementations of the Action interface that mutate the
// UMH configuration or otherwise change the system state.
//
// -----------------------------------------------------------------------------
// BUSINESS CONTEXT
// -----------------------------------------------------------------------------
// A Data‑Flow Component (DFC) in UMH is a Benthos pipeline that lives inside the
// FSM runtime. Editing a DFC therefore means **writing a new desired
// configuration** and then **waiting until the FSM reports that the running
// instance has reached the
//   - state "active" **and**
//   - the *observed* configuration matches the *desired* one.
//
// If the component fails to come up within `constants.DataflowComponentWaitForActiveTimeout`
// the action **rolls back** to the previous configuration (unless the caller set
// `ignoreHealthCheck`).
//
// Why two UUIDs?
//   * `oldComponentUUID` – the UUID that already exists in the system before the
//     edit and is taken from the incoming request payload.
//   * `newComponentUUID` – a *deterministic* UUID derived from the **name** *after*
//     the edit (this can change when the user renames the component).  It is the
//     key under which the rewritten config will be stored.
//
// Runtime state observation
// -------------------------
// The caller passes a pointer `*fsm.SystemSnapshot` that is owned and mutated by
// a different goroutine inside the FSM event loop.  The action never locks that
// pointer itself; instead it takes a *copy* under a read‑lock (`GetSystemSnapshot`).
// This means the polling loop in `waitForComponentToBeActive` always sees a
// consistent snapshot without blocking the FSM writer.
//
// Rollback strategy
// -----------------
// On timeout, `waitForComponentToBeActive` writes `oldConfig` back through the
// same `AtomicEditDataflowcomponent` API that performed the edit.  It uses
// `newComponentUUID` as key because – after a rename – the *existing* component
// in the configuration store now sits under the new ID.
//
// -----------------------------------------------------------------------------
// The concrete flow of an EditDataflowComponentAction
// -----------------------------------------------------------------------------
//   1. **Parse** – extract name, type, UUID, payload and flags.  Generate
//      `newComponentUUID` from the (possibly new) name.
//   2. **Validate** – structural sanity checks and YAML parsing.
//   3. **Execute**
//        a.     Send ActionConfirmed.
//        b.     Translate the custom payload into a Benthos service config.
//        c.     Write the new DFC config via `configManager.AtomicEditDataflowcomponent`.
//        d.     Poll `systemSnapshot` until the instance reports `state==active`
//               **and** `observedConfig == desiredConfig`.
//        e.     If the poll times out → rollback (unless `ignoreHealthCheck`).
//
// All public methods below have Go‑doc comments that repeat these key aspects in
// the exact location where a future maintainer will look for them.
// -----------------------------------------------------------------------------

package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// EditDataflowComponentAction implements the Action interface for editing an
// existing Data‑Flow Component.  The struct only contains *immutable* data that
// is required across the lifetime of a single action execution.
//
// NOTE: When adding new fields, decide explicitly whether the value is written
// once during construction/parse (→ field) or transient in Execute (→ local var).
// Keeping the struct immutable avoids subtle race conditions because callers are
// not expected to share EditDataflowComponentAction instances between goroutines.
// -----------------------------------------------------------------------------

type EditDataflowComponentAction struct {
	userEmail string // e‑mail of the human that triggered the action (used for replies)

	actionUUID   uuid.UUID // unique ID of *this* action instance
	instanceUUID uuid.UUID // ID of the UMH instance this action operates on

	outboundChannel chan *models.UMHMessage // channel used to send progress events back to the UI

	configManager config.ConfigManager // abstraction over the central configuration store

	// Parsed request payload (only populated after Parse)
	payload  models.CDFCPayload
	name     string // human‑readable component name (may change during an edit)
	metaType string // "custom" for now – future‑proofing for other component kinds

	// ─── UUID choreography ────────────────────────────────────────────────────
	oldComponentUUID uuid.UUID // UUID of the pre‑existing component (taken from the request)
	newComponentUUID uuid.UUID // deterministic UUID derived from the *new* name

	// ─── Runtime observation & synchronisation ───────────────────────────────
	systemSnapshotManager *fsm.SnapshotManager // manager that holds the latest system snapshot

	ignoreHealthCheck bool // if true -> no rollback on timeout
	actionLogger      *zap.SugaredLogger

	// Caches for rollback: we hold the freshly generated config and the old one
	dfc       config.DataFlowComponentConfig // desired (new) config
	oldConfig config.DataFlowComponentConfig // backup of the original config
}

// NewEditDataflowComponentAction returns an *un‑parsed* action instance.  The
// method exists mainly to support dependency injection in unit tests – caller
// still needs to invoke Parse & Validate before Execute.
func NewEditDataflowComponentAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager, systemSnapshotManager *fsm.SnapshotManager) *EditDataflowComponentAction {
	return &EditDataflowComponentAction{
		userEmail:             userEmail,
		actionUUID:            actionUUID,
		instanceUUID:          instanceUUID,
		outboundChannel:       outboundChannel,
		configManager:         configManager,
		actionLogger:          logger.For(logger.ComponentCommunicator),
		systemSnapshotManager: systemSnapshotManager,
	}
}

// Parse implements the Action interface.
//
// It extracts the business fields from the raw JSON payload – *without* doing
// deep semantic validation.  Responsibility is split deliberately so that unit
// tests can cover each phase independently.
//
// The function also computes `newComponentUUID` because the UUID is purely a
// function of the name and therefore already known at this stage (even before
// the heavy YAML parsing starts).
func (a *EditDataflowComponentAction) Parse(payload interface{}) error {
	//First parse the top level structure
	type TopLevelPayload struct {
		Name string `json:"name"`
		Meta struct {
			Type string `json:"type"`
		} `json:"meta"`
		Payload           interface{} `json:"payload"`
		UUID              string      `json:"uuid"`
		IgnoreHealthCheck bool        `json:"ignoreHealthCheck"`
	}

	// Parse the top level payload
	var topLevel TopLevelPayload
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %v", err)
	}

	if err := json.Unmarshal(payloadBytes, &topLevel); err != nil {
		return fmt.Errorf("failed to unmarshal top level payload: %v", err)
	}

	a.name = topLevel.Name
	if a.name == "" {
		return errors.New("missing required field Name")
	}

	//set the new component UUID by the name
	a.newComponentUUID = dataflowcomponentserviceconfig.GenerateUUIDFromName(a.name)

	a.oldComponentUUID, err = uuid.Parse(topLevel.UUID)
	if err != nil {
		return fmt.Errorf("invalid UUID format: %v", err)
	}

	//set the new component UUID by the name
	a.newComponentUUID = dataflowcomponentserviceconfig.GenerateUUIDFromName(a.name)

	// Store the meta type
	a.metaType = topLevel.Meta.Type
	if a.metaType == "" {
		return errors.New("missing required field Meta.Type")
	}

	a.ignoreHealthCheck = topLevel.IgnoreHealthCheck

	// Handle different component types
	switch a.metaType {
	case "custom":
		payload, err := parseCustomDataFlowComponent(topLevel.Payload)
		if err != nil {
			return err
		}
		a.payload = payload
	case "protocolConverter", "dataBridge", "streamProcessor":
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, "component type not supported", a.outboundChannel, models.EditDataFlowComponent)
		return fmt.Errorf("component type %s not yet supported", a.metaType)
	default:
		return fmt.Errorf("unsupported component type: %s", a.metaType)
	}

	a.actionLogger.Debugf("Parsed EditDataFlowComponent action payload: name=%s, type=%s, UUID=%s", a.name, a.metaType, a.oldComponentUUID)
	return nil
}

// Validate implements the Action interface.
//
// After `Parse` succeeded, Validate performs all expensive checks such as YAML
// unmarshalling.  That keeps error messages well‑structured: syntax/shape errors
// surface here, whereas runtime failures show up in Execute.
func (a *EditDataflowComponentAction) Validate() error {
	// Validate UUID was properly parsed
	if a.oldComponentUUID == uuid.Nil {
		return errors.New("component UUID is missing or invalid")
	}

	// Validate name and metatype were properly parsed
	if a.name == "" {
		return errors.New("missing required field Name")
	}

	if a.metaType == "" {
		return errors.New("missing required field Meta.Type")
	}

	// For custom type, validate the payload structure
	if a.metaType == "custom" {
		// Validate input fields
		if a.payload.Inputs.Type == "" {
			return errors.New("missing required field inputs.type")
		}
		if a.payload.Inputs.Data == "" {
			return errors.New("missing required field inputs.data")
		}

		// Validate output fields
		if a.payload.Outputs.Type == "" {
			return errors.New("missing required field outputs.type")
		}
		if a.payload.Outputs.Data == "" {
			return errors.New("missing required field outputs.data")
		}

		// Validate pipeline
		if len(a.payload.Pipeline) == 0 {
			return errors.New("missing required field pipeline.processors")
		}

		// Validate YAML in all components
		var temp map[string]interface{}

		// Validate Input YAML
		if err := yaml.Unmarshal([]byte(a.payload.Inputs.Data), &temp); err != nil {
			return fmt.Errorf("inputs.data is not valid YAML: %v", err)
		}

		// Validate Output YAML
		if err := yaml.Unmarshal([]byte(a.payload.Outputs.Data), &temp); err != nil {
			return fmt.Errorf("outputs.data is not valid YAML: %v", err)
		}

		// Validate pipeline processor YAML and fields
		for key, proc := range a.payload.Pipeline {
			if proc.Type == "" {
				return fmt.Errorf("missing required field pipeline.processors.%s.type", key)
			}
			if proc.Data == "" {
				return fmt.Errorf("missing required field pipeline.processors.%s.data", key)
			}

			// Check processor YAML
			if err := yaml.Unmarshal([]byte(proc.Data), &temp); err != nil {
				return fmt.Errorf("pipeline.processors.%s.data is not valid YAML: %v", key, err)
			}
		}

		// Validate inject data
		if a.payload.Inject.Type != "" && a.payload.Inject.Data != "" {
			if err := yaml.Unmarshal([]byte(a.payload.Inject.Data), &temp); err != nil {
				return fmt.Errorf("inject.data is not valid YAML: %v", err)
			}
		}
	}

	return nil
}

// Execute implements the Action interface by performing the actual configuration
// mutation and, optionally, waiting for the runtime to pick it up.
//
// The method is intentionally *long* because splitting it would complicate the
// rollback logic – we need the full context to unwind safely.
func (a *EditDataflowComponentAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing EditDataflowComponent action")

	// Send confirmation that action is starting
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed, Label("edit", a.name)+"starting", a.outboundChannel, models.EditDataFlowComponent)

	// Parse the input and output configurations
	benthosInput := make(map[string]interface{})
	benthosOutput := make(map[string]interface{})
	benthosYamlInject := make(map[string]interface{})

	// First try to use the Input data
	err := yaml.Unmarshal([]byte(a.payload.Inputs.Data), &benthosInput)
	if err != nil {
		errMsg := Label("edit", a.name) + fmt.Sprintf("failed to parse input data: %s", err.Error())
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errMsg, a.outboundChannel, models.EditDataFlowComponent)
		return nil, nil, fmt.Errorf("%s", errMsg)
	}

	//parse the output data
	err = yaml.Unmarshal([]byte(a.payload.Outputs.Data), &benthosOutput)
	if err != nil {
		errMsg := Label("edit", a.name) + fmt.Sprintf("failed to parse output data: %s", err.Error())
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errMsg, a.outboundChannel, models.EditDataFlowComponent)
		return nil, nil, fmt.Errorf("%s", errMsg)
	}

	//parse the inject data
	if a.payload.Inject.Data != "" {
		err = yaml.Unmarshal([]byte(a.payload.Inject.Data), &benthosYamlInject)
		if err != nil {
			errMsg := Label("edit", a.name) + fmt.Sprintf("failed to parse inject data: %s", err.Error())
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errMsg, a.outboundChannel, models.EditDataFlowComponent)
			return nil, nil, fmt.Errorf("%s", errMsg)
		}
	}

	//parse the cache resources, rate limit resources and buffer from the inject data
	cacheResources, ok := benthosYamlInject["cache_resources"].([]interface{})
	if !ok {
		cacheResources = []interface{}{}
	}

	rateLimitResources, ok := benthosYamlInject["rate_limit_resources"].([]interface{})
	if !ok {
		rateLimitResources = []interface{}{}
	}

	buffer, ok := benthosYamlInject["buffer"].(map[string]interface{})
	if !ok {
		buffer = map[string]interface{}{}
	}

	benthosCacheResources := make([]map[string]interface{}, len(cacheResources))
	for i, resource := range cacheResources {
		resourceMap, ok := resource.(map[string]interface{})
		if !ok {
			return nil, nil, fmt.Errorf("cache resource %d is not a valid object", i)
		}
		benthosCacheResources[i] = resourceMap
	}

	benthosRateLimitResources := make([]map[string]interface{}, len(rateLimitResources))
	for i, resource := range rateLimitResources {
		resourceMap, ok := resource.(map[string]interface{})
		if !ok {
			return nil, nil, fmt.Errorf("rate limit resource %d is not a valid object", i)
		}
		benthosRateLimitResources[i] = resourceMap
	}

	benthosBuffer := make(map[string]interface{})
	for key, value := range buffer {
		benthosBuffer[key] = value
	}

	// Convert pipeline data to Benthos pipeline configuration
	benthosPipeline := map[string]interface{}{
		"processors": []interface{}{},
	}

	if len(a.payload.Pipeline) > 0 {
		// Convert each processor configuration in the pipeline
		processors := []interface{}{}

		// Check if we have numeric keys (0, 1, 2, ...) and use them to preserve order
		hasNumericKeys := CheckIfOrderedNumericKeys(a.payload.Pipeline)

		if hasNumericKeys {
			// Process in numeric order
			for i := range len(a.payload.Pipeline) {
				processorName := fmt.Sprintf("%d", i)

				processor := a.payload.Pipeline[processorName]
				var procConfig map[string]interface{}
				err := yaml.Unmarshal([]byte(processor.Data), &procConfig)
				if err != nil {
					errMsg := Label("edit", a.name) + fmt.Sprintf("failed to parse pipeline processor %s: %s", processorName, err.Error())
					SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errMsg, a.outboundChannel, models.EditDataFlowComponent)
					return nil, nil, fmt.Errorf("%s", errMsg)
				}

				// Add processor to the list
				processors = append(processors, procConfig)
			}
		}

		if !hasNumericKeys {
			// the frontend always sends numerous keys so this should never happen
			SendActionReply(a.instanceUUID, a.userEmail,
				a.actionUUID, models.ActionFinishedWithFailure, "At least one processor with a non-numerous key was found.",
				a.outboundChannel, models.EditDataFlowComponent)
			return nil, nil, fmt.Errorf("at least one processor with a non-numerous key was found")
		}

		benthosPipeline["processors"] = processors
	}

	// Create the Benthos service config
	benthosConfig := benthosserviceconfig.BenthosServiceConfig{
		Input:              benthosInput,
		Output:             benthosOutput,
		Pipeline:           benthosPipeline,
		CacheResources:     benthosCacheResources,
		RateLimitResources: benthosRateLimitResources,
		Buffer:             benthosBuffer,
	}

	// Normalize the config
	normalizedConfig := benthosserviceconfig.NormalizeBenthosConfig(benthosConfig)

	// Create the DataFlowComponentConfig
	dfc := config.DataFlowComponentConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            a.name,
			DesiredFSMState: "active",
		},
		DataFlowComponentServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
			BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
				Input:              normalizedConfig.Input,
				Pipeline:           normalizedConfig.Pipeline,
				Output:             normalizedConfig.Output,
				CacheResources:     normalizedConfig.CacheResources,
				RateLimitResources: normalizedConfig.RateLimitResources,
				Buffer:             normalizedConfig.Buffer,
			},
		},
	}

	a.dfc = dfc

	// Update the component in the configuration
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, Label("edit", a.name)+"updating configuration", a.outboundChannel, models.EditDataFlowComponent)
	a.oldConfig, err = a.configManager.AtomicEditDataflowcomponent(ctx, a.oldComponentUUID, dfc)
	if err != nil {
		errorMsg := Label("edit", a.name) + fmt.Sprintf("failed to edit dataflow component: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errorMsg, a.outboundChannel, models.EditDataFlowComponent)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// check against observedState as well
	if a.systemSnapshotManager != nil { // skipping this for the unit tests
		if a.ignoreHealthCheck {
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, Label("edit", a.name)+"configuration updated; but ignoring the health check", a.outboundChannel, models.EditDataFlowComponent)
		} else {
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, Label("edit", a.name)+"configuration updated; waiting to become active", a.outboundChannel, models.EditDataFlowComponent)
			err = a.waitForComponentToBeActive()
			if err != nil {
				errorMsg := Label("edit", a.name) + fmt.Sprintf("failed to wait for dataflow component to be active: %v", err)
				SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errorMsg, a.outboundChannel, models.EditDataFlowComponent)
				return nil, nil, fmt.Errorf("%s", errorMsg)
			}
		}
	}

	// return success message, but do not send it as this is done by the caller
	successMsg := Label("edit", a.name) + "success"

	return successMsg, nil, nil
}

// getUserEmail implements the Action interface by returning the user email associated with this action.
func (a *EditDataflowComponentAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface by returning the UUID of this action.
func (a *EditDataflowComponentAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// GetParsedPayload returns the parsed CDFCPayload - exposed primarily for testing purposes.
func (a *EditDataflowComponentAction) GetParsedPayload() models.CDFCPayload {
	return a.payload
}

// GetComponentUUID returns the UUID of the component being edited - exposed primarily for testing purposes.
func (a *EditDataflowComponentAction) GetComponentUUID() uuid.UUID {
	return a.oldComponentUUID
}

// waitForComponentToBeActive polls the live FSM state until either
//   - the component shows up **active** with the *expected* configuration or
//   - the timeout hits (→ rollback except when ignoreHealthCheck).
//
// Concurrency note: The method never writes to `systemSnapshot`; the FSM runtime
// is the single writer.  We only take read‑locks while **copying** the full
// snapshot to avoid holding the lock during YAML comparisons.
func (a *EditDataflowComponentAction) waitForComponentToBeActive() error {
	// checks the system snapshot
	// 1. waits for the component to appear in the system snapshot (relevant for changed name)
	// 2. waits for the component to be active
	// 3. waits for the component to have the correct config
	ticker := time.NewTicker(constants.ActionTickerTime)
	defer ticker.Stop()
	timeout := time.After(constants.DataflowComponentWaitForActiveTimeout)
	startTime := time.Now()
	timeoutDuration := constants.DataflowComponentWaitForActiveTimeout

	var logs []s6.LogEntry
	var lastLogs []s6.LogEntry

	for {
		select {
		case <-timeout:
			stateMessage := Label("edit", a.name) + "timeout reached. it did not become active in time. rolling back"
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, stateMessage, a.outboundChannel, models.EditDataFlowComponent)
			ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
			defer cancel()
			_, err := a.configManager.AtomicEditDataflowcomponent(ctx, a.newComponentUUID, a.oldConfig)
			if err != nil {

				a.actionLogger.Errorf("failed to roll back dataflow component %s: %v", a.name, err)
			}
			return fmt.Errorf("dataflow component %s was not active in time and was rolled back to the old config", a.name)

		case <-ticker.C:
			elapsed := time.Since(startTime)
			remaining := timeoutDuration - elapsed
			remainingSeconds := int(remaining.Seconds())

			// the snapshot manager holds the latest system snapshot which is asynchronously updated by the other goroutines
			// we need to get a deep copy of it to prevent race conditions
			systemSnapshot := a.systemSnapshotManager.GetDeepCopySnapshot()
			if dataflowcomponentManager, exists := systemSnapshot.Managers[constants.DataflowcomponentManagerName]; exists {
				instances := dataflowcomponentManager.GetInstances()
				found := false
				for _, instance := range instances {
					if dataflowcomponentserviceconfig.GenerateUUIDFromName(instance.ID) == a.newComponentUUID {
						found = true
						dfcSnapshot, ok := instance.LastObservedState.(*dataflowcomponent.DataflowComponentObservedStateSnapshot)
						if !ok {
							stateMessage := RemainingPrefixSec(remainingSeconds) + "waiting for state info"
							SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
								stateMessage, a.outboundChannel, models.EditDataFlowComponent)
							continue
						}
						// Verify that the Benthos instance has applied the desired configuration.
						// We compare the desired DataFlowComponentConfig with the *observed* Benthos
						// configuration contained in
						// dfcSnapshot.ServiceInfo.BenthosObservedState.ObservedBenthosServiceConfig.
						//
						// Do NOT use instance.LastObservedState.Config: the reconcile loop sets
						// that field to the desired config as soon as it writes to the store,
						// potentially some ticks before Benthos has actually restarted. Relying on
						// it here would let the action declare success while the container is
						// still starting up.
						//
						// ObservedBenthosServiceConfig and DataflowComponentServiceConfig differ in
						// type, so we convert the observed struct to the DFC representation before
						// running the comparison.

						if !CompareSnapshotWithDesiredConfig(dfcSnapshot, a.dfc) {
							stateMessage := RemainingPrefixSec(remainingSeconds) + "config not yet applied"
							SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
								stateMessage, a.outboundChannel, models.EditDataFlowComponent)
							continue
						}

						if instance.CurrentState != "active" && instance.CurrentState != "idle" {
							// currentStateReason contains more information on why the DFC is in its current state
							currentStateReason := dfcSnapshot.ServiceInfo.StatusReason

							stateMessage := RemainingPrefixSec(remainingSeconds) + currentStateReason
							SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
								stateMessage, a.outboundChannel, models.EditDataFlowComponent)
							// send the benthos logs to the user
							logs = dfcSnapshot.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosLogs
							// only send the logs that have not been sent yet
							if len(logs) > len(lastLogs) {
								lastLogs = SendLimitedLogs(logs, lastLogs, a.instanceUUID, a.userEmail, a.actionUUID, a.outboundChannel, models.EditDataFlowComponent, remainingSeconds)
							}

							continue
						} else {
							stateMessage := RemainingPrefixSec(remainingSeconds) + fmt.Sprintf("completed. is in state '%s' with correct configuration", instance.CurrentState)
							SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
								stateMessage, a.outboundChannel, models.EditDataFlowComponent)
							return nil
						}
					}
				}
				if !found {
					stateMessage := RemainingPrefixSec(remainingSeconds) + "waiting for it to appear in the config"
					SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
						stateMessage, a.outboundChannel, models.EditDataFlowComponent)
				}
			} else {
				stateMessage := RemainingPrefixSec(remainingSeconds) + "waiting for manager to initialise"
				SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
					stateMessage, a.outboundChannel, models.EditDataFlowComponent)
			}
		}
	}
}

func CompareSnapshotWithDesiredConfig(dfcSnapshot *dataflowcomponent.DataflowComponentObservedStateSnapshot, desiredConfig config.DataFlowComponentConfig) bool {
	if dfcSnapshot == nil || dfcSnapshot.ServiceInfo.BenthosObservedState.ObservedBenthosServiceConfig.Input == nil {
		return false
	}
	observedConfig := dfcSnapshot.ServiceInfo.BenthosObservedState.ObservedBenthosServiceConfig
	observedConfigInDfcConfig := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
		BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
			Input:              observedConfig.Input,
			Pipeline:           observedConfig.Pipeline,
			Output:             observedConfig.Output,
			CacheResources:     observedConfig.CacheResources,
			RateLimitResources: observedConfig.RateLimitResources,
			Buffer:             observedConfig.Buffer,
		},
	}
	return dataflowcomponentserviceconfig.NewComparator().ConfigsEqual(&observedConfigInDfcConfig, &desiredConfig.DataFlowComponentServiceConfig)
}
