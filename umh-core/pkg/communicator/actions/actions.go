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
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/safejson"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	deps "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	fsmv2sentry "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
)

// communicatorHierarchyPath is the Sentry hierarchy path for action-handler
// logs. Uses dot separator: ParseHierarchyPath splits FSMv1 paths on "."
// (not "/"). Mirrors configManagerHierarchyPath in pkg/config/manager.go,
// the canonical convention for non-FSMv2 packages wired to FSMLogger.
const communicatorHierarchyPath = "fsmv1.Communicator"

// communicatorSentryHook is the package-level SentryHook shared across all
// action goroutines. Set inside initCommunicatorFSMLogger on first use.
// StopCommunicatorSentryHook nil-checks before calling Stop, so reading
// here without synchronization is safe (sync.OnceValue establishes the
// happens-before for the first read; subsequent reads see the same pointer).
var communicatorSentryHook *fsmv2sentry.SentryHook

// communicatorFSMLoggerFn lazily builds the wrapped FSMLogger exactly once.
// Build-once is required: SentryHook.Wrap mutates h.Core
// (pkg/fsmv2/sentry/hook.go:62), so rebuilding on every call would race
// under concurrent action goroutines. Mirrors the singleton precedent at
// pkg/config/manager.go:179-182 (FileConfigManager).
var communicatorFSMLoggerFn = sync.OnceValue(initCommunicatorFSMLogger)

func initCommunicatorFSMLogger() deps.FSMLogger {
	base := logger.For(logger.ComponentCommunicator)
	communicatorSentryHook = fsmv2sentry.NewSentryHook(5 * time.Minute)
	wrapped := base.Desugar().WithOptions(zap.WrapCore(communicatorSentryHook.Wrap)).Sugar()

	return deps.NewFSMLogger(wrapped)
}

// communicatorFSMLogger returns the FSMLogger whose underlying zap core is
// wrapped with the package-level SentryHook. First call constructs the hook
// and the wrapped logger; subsequent calls return the same instance.
func communicatorFSMLogger() deps.FSMLogger {
	return communicatorFSMLoggerFn()
}

// StopCommunicatorSentryHook releases the package-level SentryHook's
// debouncer goroutine. Call from cmd/main.go shutdown alongside
// fsmv2Hook.Stop(). Safe to call when the hook was never initialized.
func StopCommunicatorSentryHook() {
	if communicatorSentryHook != nil {
		communicatorSentryHook.Stop()
	}
}

// Action is the interface that all action types must implement.
// It defines the core lifecycle methods for parsing, validating, and executing actions.
type Action interface {
	// Parse parses the ActionMessagePayload into the corresponding action type.
	// It should extract and validate all required fields from the raw payload.
	Parse(interface{}) error

	// Validate validates the action payload, returns an error if something is wrong.
	// This should perform deeper validation than Parse, checking business rules and constraints.
	Validate() error

	// Execute executes the action, returns the result as an interface and an error if something went wrong.
	// It must send ActionConfirmed and ActionExecuting messages for progress updates.
	// It must send ActionFinishedWithFailure messages if an error occurs.
	// It must not send the final successful action reply, as it is done by the caller.
	Execute() (interface{}, map[string]interface{}, error)

	// getUserEmail returns the user email of the action
	getUserEmail() string

	// getUuid returns the UUID of the action
	getUuid() uuid.UUID
}

// HandleActionMessage is the main entry point for processing action messages.
// It identifies the action type, creates the appropriate action implementation,
// and processes it through the Parse->Validate->Execute flow.
//
// After execution, it handles sending the success reply if the action completed successfully.
// Error handling for each step is done within this function.
func HandleActionMessage(instanceUUID uuid.UUID, payload models.ActionMessagePayload, sender string, outboundChannel chan *models.UMHMessage, releaseChannel config.ReleaseChannel, dog watchdog.Iface, traceID uuid.UUID, systemSnapshotManager *fsm.SnapshotManager, configManager config.ConfigManager) {
	log := logger.For(logger.ComponentCommunicator)
	fsmLogger := communicatorFSMLogger()

	// Panic recovery. Converts any uncaught panic in this goroutine into a
	// Sentry event, a counter increment, and a non-blocking failure reply so
	// the user-visible action is marked failed and the umh-core process keeps
	// running. Adapted from pkg/fsmv2/supervisor's tick-recovery pattern
	// (ENG-4289). See ENG-4959 for the regression that motivated wiring this
	// guard into the Communicator goroutine tree.
	defer func() {
		if r := recover(); r != nil {
			recoverActionPanic(r, instanceUUID, sender, payload, fsmLogger, outboundChannel)
		}
	}()

	// Start a new transaction for this action
	log.Debugf("Handling action message: Type: %s, Payload: %v", payload.ActionType, payload.ActionPayload)

	action := newActionFromPayloadFn(instanceUUID, payload, sender, outboundChannel, systemSnapshotManager, configManager, log, fsmLogger)
	if action == nil {
		log.Errorf("Unknown action type: %s", payload.ActionType)
		fsmLogger.SentryWarn(
			deps.FeatureFSMv1Communicator,
			communicatorHierarchyPath,
			"unknown action type",
			deps.String("action_type", string(payload.ActionType)),
			deps.String("action_uuid", payload.ActionUUID.String()),
		)
		SendActionReply(instanceUUID, sender, payload.ActionUUID, models.ActionFinishedWithFailure, "Unknown action type", outboundChannel, payload.ActionType)

		return
	}

	// Check if payload contains a DFC with a state field and is an EditProtocolConverter action.
	// If yes, log any errors via Sentry.
	// TODO: This can be removed once the "Write flows" feature is fully implemented
	isLogToSentry := payload.ActionType == models.EditProtocolConverter && payloadHasDFCState(payload.ActionPayload)

	SendActionReply(instanceUUID, sender, payload.ActionUUID, models.ActionExecuting, "Parsing action payload", outboundChannel, payload.ActionType)
	// Parse the action payload
	err := action.Parse(payload.ActionPayload)
	if err != nil {
		// If parsing fails, send a structured error reply using SendActionReplyV2 with ErrRetryParseFailed
		// this will allow the UI to retry the action
		SendActionReplyV2(instanceUUID, sender, payload.ActionUUID, models.ActionFinishedWithFailure, "Failed to parse action payload: "+err.Error(), models.ErrParseFailed, nil, outboundChannel, payload.ActionType, nil)
		log.Errorf("Error parsing action payload: %s", err)
		if isLogToSentry {
			fsmLogger.SentryError(deps.FeatureDisableReadFlows, "", err, "protocol_converter_parse_failed")
		}

		return
	}

	SendActionReply(instanceUUID, sender, payload.ActionUUID, models.ActionExecuting, "Validating action payload", outboundChannel, payload.ActionType)
	// Validate the action payload
	err = action.Validate()
	if err != nil {
		// If validation fails, send a structured error reply using SendActionReplyV2 with ErrEditValidationFailed
		SendActionReplyV2(instanceUUID, sender, payload.ActionUUID, models.ActionFinishedWithFailure, "Failed to validate action payload: "+err.Error(), models.ErrValidationFailed, nil, outboundChannel, payload.ActionType, nil)
		log.Errorf("Error validating action payload: %s", err)
		if isLogToSentry {
			fsmLogger.SentryError(deps.FeatureDisableReadFlows, "", err, "protocol_converter_validate_failed")
		}

		return
	}

	SendActionReply(instanceUUID, sender, payload.ActionUUID, models.ActionExecuting, "Executing action", outboundChannel, payload.ActionType)
	// Execute the action
	result, metadata, err := action.Execute()
	if err != nil {
		log.Errorf("Error executing action: %s", err)
		if isLogToSentry {
			fsmLogger.SentryError(deps.FeatureDisableReadFlows, "", err, "protocol_converter_execute_failed")
		}

		return
	}

	// Normally, the action.go logs the execution result that is sent back to the frontend. For the get-logs action,
	// this introduced a problem: in the frontend, we auto-refresh the logs of the companion. The get-logs action
	// reply is then the whole current log of the companion. If we log this action reply, we double the amount of
	// log lines on every get-logs call. This exponential growth of logs leads after a short time to problems.
	// To avoid this, we do not log the get-logs result.
	// This behavior is signaled to the frontend via the "log-logs-suppression" supported feature flag.
	if payload.ActionType != models.GetLogs {
		log.Debugf("Action executed, sending reply: %v", result)
	}

	SendActionReplyWithAdditionalContext(instanceUUID, sender, payload.ActionUUID, models.ActionFinishedSuccessfull, result, outboundChannel, payload.ActionType, metadata)
}

// Test seam. Production never reassigns. Tests that swap this must not
// call t.Parallel() — the read in HandleActionMessage is unsynchronized.
var newActionFromPayloadFn = newActionFromPayload

// Test seam. Same t.Parallel() constraint as newActionFromPayloadFn.
var recordActionPanicFn = metrics.RecordActionPanic

// recoverActionPanic is the body of HandleActionMessage's defer-recover,
// split out so tests can drive it directly. The outer recover catches a
// panic in the action handler itself; the inner guard catches a panic
// inside this recovery path (mirrors fsmv2 supervisor's tick-recover).
func recoverActionPanic(
	r any,
	instanceUUID uuid.UUID,
	userEmail string,
	payload models.ActionMessagePayload,
	fsmLogger deps.FSMLogger,
	outboundChannel chan *models.UMHMessage,
) {
	defer func() {
		if r2 := recover(); r2 != nil {
			// Best-effort Sentry breadcrumb when the recovery path itself
			// panics (Sentry logger, metric increment, or reply generation
			// — not the original action panic, which the OUTER recover
			// already captured). Pattern mirrors pkg/fsmv2/supervisor's
			// tick-recover secondary guard. The innermost recover ensures
			// a panic inside this last-resort Sentry call does not
			// propagate; stderr remains the absolute fallback.
			func() {
				defer func() { _ = recover() }()

				if fsmLogger != nil {
					fsmLogger.SentryError(
						deps.FeatureFSMv1Communicator,
						communicatorHierarchyPath,
						fmt.Errorf("action handler double panic: primary=%v secondary=%v", r, r2),
						"action_handler_double_panic",
						deps.String("action_type", string(payload.ActionType)),
						deps.String("action_uuid", payload.ActionUUID.String()),
					)
				}
			}()

			fmt.Fprintf(os.Stderr,
				"action_handler_double_panic: action=%s uuid=%s primary=%v secondary=%v\n",
				payload.ActionType, payload.ActionUUID.String(), r, r2)
		}
	}()

	panicType, panicErr := classifyActionPanic(r)

	// Sentry runs before the metric increment because Sentry carries the
	// most diagnostic value for incident response. The double-panic guard
	// (above) re-attempts Sentry if either subsequent step itself panics,
	// so the primary panic always reaches the dashboard even if metrics
	// or reply generation fails.
	if fsmLogger != nil {
		fsmLogger.SentryError(
			deps.FeatureFSMv1Communicator,
			communicatorHierarchyPath,
			fmt.Errorf("action handler panic: %w", panicErr),
			"action_handler_panic",
			deps.String("action_type", string(payload.ActionType)),
			deps.String("action_uuid", payload.ActionUUID.String()),
			deps.String("panic_type", panicType),
			// debug.Stack() captures the panicking goroutine's stack: the
			// frames between the panic site and this deferred recover are
			// still on the stack (unwind has not happened yet), so Sentry
			// receives the full panic-site trace.
			deps.String("stack_trace", string(debug.Stack())),
		)
	}

	recordActionPanicFn(string(payload.ActionType), panicType)

	replyMsg, err := generateUMHMessage(
		instanceUUID, userEmail, models.ActionReply,
		models.ActionReplyMessagePayload{
			ActionUUID:       payload.ActionUUID,
			ActionReplyState: models.ActionFinishedWithFailure,
			ActionReplyPayload: fmt.Sprintf(
				"Internal error: %s action failed unexpectedly (action %s). UMH engineering has been notified.",
				payload.ActionType, payload.ActionUUID.String()),
		})
	if err != nil {
		// Reply generation can fail on JSON marshal errors. Without a Sentry
		// breadcrumb here the customer's action appears stuck (no failure
		// reply ever arrives) and engineering only sees the panic event, not
		// the follow-on failure. Stderr line kept as last-resort fallback.
		if fsmLogger != nil {
			fsmLogger.SentryError(
				deps.FeatureFSMv1Communicator,
				communicatorHierarchyPath,
				err,
				"action_handler_panic_reply_generation_failed",
				deps.String("action_type", string(payload.ActionType)),
				deps.String("action_uuid", payload.ActionUUID.String()),
			)
		}

		fmt.Fprintf(os.Stderr,
			"action_handler_panic_reply_generation_failed: action=%s uuid=%s err=%v\n",
			payload.ActionType, payload.ActionUUID.String(), err)

		return
	}

	// Non-blocking send. SendActionReply uses an unconditional channel
	// write (sendActionReplyInternal further down this file) that would
	// block forever if outboundChannel is full. The recovery goroutine
	// must not block; Sentry and the metric have already captured the
	// panic, so the reply is a UX nicety.
	select {
	case outboundChannel <- &replyMsg:
	default:
		// Structured log matches the pattern used elsewhere when the outbound
		// channel saturates (push/push.go, fsmv2_adapter/legacy_bridge.go).
		// Stderr fallback kept so the line remains grep-friendly during
		// instance-offline triage even if zap is misconfigured.
		logger.For(logger.ComponentCommunicator).Warnw(
			"action_handler_panic_reply_dropped",
			"action_type", string(payload.ActionType),
			"action_uuid", payload.ActionUUID.String(),
		)

		fmt.Fprintf(os.Stderr,
			"action_handler_panic_reply_dropped: outbound channel full, action=%s uuid=%s\n",
			payload.ActionType, payload.ActionUUID.String())
	}
}

// classifyActionPanic maps a recovered panic to the panic_type metric
// label and an error for Sentry. Labels match panicutil.PanicType* in
// fsmv2 so dashboards group action and worker panics together. Do not
// add richer categories — cardinality stays bounded, richer info goes
// to Sentry.
func classifyActionPanic(r any) (panicType string, err error) {
	switch v := r.(type) {
	case error:
		return "error_panic", v
	case string:
		return "string_panic", errors.New(v)
	default:
		// %T avoids invoking String() — a re-panic from a pathological
		// Stringer (ENG-4289 family) would defeat the recovery here.
		return "unknown_panic", fmt.Errorf("%T (string repr suppressed)", v)
	}
}

// newActionFromPayload constructs the concrete Action implementation for the
// given action type. It returns nil for unknown action types.
//
// This is the single point where action structs are built from their
// dependencies. Every action whose struct exposes an fsmLogger field must
// receive fsmLogger here, otherwise it will be a zero-value (nil) interface
// and any call to its SentryWarn/SentryError methods will panic at runtime.
// See ENG-4959 for the PR #2546 regression that motivated this contract.
func newActionFromPayload(
	instanceUUID uuid.UUID,
	payload models.ActionMessagePayload,
	sender string,
	outboundChannel chan *models.UMHMessage,
	systemSnapshotManager *fsm.SnapshotManager,
	configManager config.ConfigManager,
	log *zap.SugaredLogger,
	fsmLogger deps.FSMLogger,
) Action {
	switch payload.ActionType {
	case models.EditInstance:
		return &EditInstanceAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			configManager:         configManager,
			actionLogger:          log,
			systemSnapshotManager: systemSnapshotManager,
		}
	case models.DeployDataFlowComponent:
		return &DeployDataflowComponentAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			configManager:         configManager,
			systemSnapshotManager: systemSnapshotManager,
			actionLogger:          log,
		}

	case models.DeleteDataFlowComponent:
		return &DeleteDataflowComponentAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			configManager:         configManager,
			systemSnapshotManager: systemSnapshotManager,
			actionLogger:          log,
		}

	case models.GetDataFlowComponent:
		return &GetDataFlowComponentAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			configManager:         configManager,
			systemSnapshotManager: systemSnapshotManager,
			actionLogger:          log,
		}
	case models.EditDataFlowComponent:
		return &EditDataflowComponentAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			configManager:         configManager,
			actionLogger:          log,
			systemSnapshotManager: systemSnapshotManager,
		}
	case models.GetLogs:
		return &GetLogsAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			configManager:         configManager,
			actionLogger:          log,
			systemSnapshotManager: systemSnapshotManager,
		}
	case models.GetConfigFile:
		return &GetConfigFileAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			systemSnapshotManager: systemSnapshotManager,
			configManager:         configManager,
			actionLogger:          log,
		}
	case models.SetConfigFile:
		return &SetConfigFileAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			systemSnapshotManager: systemSnapshotManager,
			configManager:         configManager,
			actionLogger:          log,
		}

	case models.GetDataFlowComponentMetrics: //nolint:staticcheck // Deprecated but kept for back compat
		return &GetDataflowcomponentMetricsAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			actionLogger:          log,
			systemSnapshotManager: systemSnapshotManager,
		}

	case models.DeployProtocolConverter:
		return &DeployProtocolConverterAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			configManager:         configManager,
			actionLogger:          log,
			fsmLogger:             fsmLogger,
			systemSnapshotManager: systemSnapshotManager,
		}
	case models.EditProtocolConverter:
		return &EditProtocolConverterAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			configManager:         configManager,
			systemSnapshotManager: systemSnapshotManager,
			actionLogger:          log,
			fsmLogger:             fsmLogger,
		}
	case models.SaveProtocolConverter:
		return &SaveProtocolConverterAction{
			userEmail:       sender,
			actionUUID:      payload.ActionUUID,
			instanceUUID:    instanceUUID,
			outboundChannel: outboundChannel,
			configManager:   configManager,
			actionLogger:    log,
			fsmLogger:       fsmLogger,
		}
	case models.GetProtocolConverter:
		return NewGetProtocolConverterAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, configManager, systemSnapshotManager)

	case models.GetMetrics:
		return NewGetMetricsAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, systemSnapshotManager, log)
	case models.DeleteProtocolConverter:
		return NewDeleteProtocolConverterAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, configManager, systemSnapshotManager)
	case models.AddDataModel:
		return NewAddDataModelAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, configManager)
	case models.DeleteDataModel:
		return NewDeleteDataModelAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, configManager)
	case models.EditDataModel:
		return NewEditDataModelAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, configManager)
	case models.GetDataModel:
		return NewGetDataModelAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, configManager)
	case models.DeleteStreamProcessor:
		return NewDeleteStreamProcessorAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, configManager, systemSnapshotManager)
	case models.EditStreamProcessor:
		return NewEditStreamProcessorAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, configManager, systemSnapshotManager)
	case models.DeployStreamProcessor:
		return NewDeployStreamProcessorAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, configManager, systemSnapshotManager)
	case models.GetStreamProcessor:
		return NewGetStreamProcessorAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, configManager, systemSnapshotManager)
	}

	return nil
}

// SendActionReply sends an action reply with the given state and payload.
// It is a convenience wrapper around SendActionReplyWithAdditionalContext that doesn't include additional context.
// It returns false if an error occurred during message generation or sending.
func SendActionReply(instanceUUID uuid.UUID, userEmail string, actionUUID uuid.UUID, arstate models.ActionReplyState, payload interface{}, outboundChannel chan *models.UMHMessage, action models.ActionType) bool {
	// TODO: The 'action' parameter will be used in the future for action-specific logic or logging
	return SendActionReplyWithAdditionalContext(instanceUUID, userEmail, actionUUID, arstate, payload, outboundChannel, action, nil)
}

// SendActionReplyWithAdditionalContext sends an action reply with added context metadata.
// It is used for all user-facing communication about action progress and results.
// The actionContext parameter allows passing additional structured data with the reply.
//
// This is the primary method for sending action status messages to users, and is
// used for confirmation, progress updates, success, and failure notifications.
func SendActionReplyWithAdditionalContext(instanceUUID uuid.UUID, userEmail string, actionUUID uuid.UUID, arstate models.ActionReplyState, payload interface{}, outboundChannel chan *models.UMHMessage, action models.ActionType, actionContext map[string]interface{}) bool {
	// TODO: The 'action' parameter will be used in the future for action-specific logic or logging
	err := sendActionReplyInternal(instanceUUID, userEmail, actionUUID, arstate, payload, outboundChannel, actionContext)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, logger.For(logger.ComponentCommunicator), "Error generating action reply: %w", err)

		return false
	}

	return true
}

// sendActionReplyInternal is the internal implementation for SendActionReply.
// It handles the actual process of creating and sending UMH messages.
// This function is only meant to be called within the actions.go file!
// Use SendActionReply instead (or SendActionReplyWithAdditionalContext if you need to pass additional context).
func sendActionReplyInternal(instanceUUID uuid.UUID, userEmail string, actionUUID uuid.UUID, arstate models.ActionReplyState, payload interface{}, outboundChannel chan *models.UMHMessage, actionContext map[string]interface{}) error {
	var (
		err        error
		umhMessage models.UMHMessage
	)

	if actionContext == nil {
		umhMessage, err = generateUMHMessage(instanceUUID, userEmail, models.ActionReply, models.ActionReplyMessagePayload{
			ActionUUID:         actionUUID,
			ActionReplyState:   arstate,
			ActionReplyPayload: payload,
		})
	} else {
		umhMessage, err = generateUMHMessage(instanceUUID, userEmail, models.ActionReply, models.ActionReplyMessagePayload{
			ActionUUID:         actionUUID,
			ActionReplyState:   arstate,
			ActionReplyPayload: payload,
			ActionContext:      actionContext,
		})
	}

	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, logger.For(logger.ComponentCommunicator), "Error generating umh message: %v", err)

		return err
	}

	outboundChannel <- &umhMessage

	return nil
}

// generateUMHMessage creates a UMHMessage with the specified parameters.
// It handles the encryption of message content before adding it to the UMHMessage.
//
// There is no check for matching message type and payload, so ensure the payload
// is compatible with the message type. The content is encrypted using the
// encoding package before being added to the message.
func generateUMHMessage(instanceUUID uuid.UUID, userEmail string, messageType models.MessageType, payload any) (umhMessage models.UMHMessage, err error) {
	messageContent := models.UMHMessageContent{
		MessageType: messageType,
		Payload:     payload,
	}

	encryptedContent, err := encoding.EncodeMessageFromUMHInstanceToUser(messageContent)
	if err != nil {
		return
	}

	umhMessage = models.UMHMessage{
		Email:        userEmail,
		Content:      encryptedContent,
		InstanceUUID: instanceUUID,
	}

	return
}

// ParseActionPayload is a generic helper function that converts raw payload data into a typed struct.
// It handles the conversion from interface{} -> map -> JSON -> typed struct safely.
//
// This function is particularly useful for parsing nested structures within action payloads,
// and provides consistent error handling for payload parsing.
//
// Example usage:
//
//	myPayload, err := ParseActionPayload[MyCustomStruct](actionPayload)
func ParseActionPayload[T any](actionPayload interface{}) (T, error) {
	var payload T

	rawMap, ok := actionPayload.(map[string]interface{})
	if !ok {
		return payload, fmt.Errorf("could not assert ActionPayload to map[string]interface{}. Actual type: %T, Value: %v", actionPayload, actionPayload)
	}

	// Marshal the raw payload into JSON bytes
	jsonData, err := safejson.Marshal(rawMap)
	if err != nil {
		return payload, fmt.Errorf("error marshaling raw payload: %w", err)
	}

	// Unmarshal the JSON bytes into the specified type
	err = safejson.Unmarshal(jsonData, &payload)
	if err != nil {
		return payload, fmt.Errorf("error unmarshaling into target type: %w", err)
	}

	return payload, nil
}

// SendActionReplyV2 sends an action reply with the given state and payload which is a map[string]interface{}
// SendActionReplyV2 should be used only for ActionFailure messages for backwards compatibility. This will be changed in the future.
// This function is preferred over SendActionReply as it is more flexible and allows for more complex payloads
// The return type is a bool and returns false if an error occurred.
func SendActionReplyV2(
	instanceUUID uuid.UUID,
	userEmail string,
	actionUUID uuid.UUID,
	arstate models.ActionReplyState,
	message string,
	errorCode string,
	payloadV2 map[string]interface{},
	outboundChannel chan *models.UMHMessage,
	action models.ActionType,
	actionContext map[string]interface{},
) bool {
	// TODO: The 'action' parameter will be used in the future for action-specific logic or logging
	return sendActionReplyWithAdditionalContextV2(instanceUUID, userEmail, actionUUID, arstate, message, errorCode, payloadV2, outboundChannel, action, actionContext)
}

func sendActionReplyWithAdditionalContextV2(
	instanceUUID uuid.UUID,
	userEmail string,
	actionUUID uuid.UUID,
	arstate models.ActionReplyState,
	message string,
	errorCode string,
	payloadV2 map[string]interface{},
	outboundChannel chan *models.UMHMessage,
	action models.ActionType,
	actionContext map[string]interface{},
) bool {
	// TODO: The 'action' parameter will be used in the future for action-specific logic or logging
	err := sendActionReplyInternalV2(instanceUUID, userEmail, actionUUID, arstate, message, errorCode, payloadV2, outboundChannel, actionContext)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, logger.For(logger.ComponentCommunicator), "Error generating action reply: %w", err)

		return false
	}

	return true
}

func sendActionReplyInternalV2(
	instanceUUID uuid.UUID,
	userEmail string,
	actionUUID uuid.UUID,
	arstate models.ActionReplyState,
	message string,
	errorCode string,
	payloadV2 map[string]interface{},
	outboundChannel chan *models.UMHMessage,
	actionContext map[string]interface{},
) error {
	payloadResponse := ConstructActionReplyV2Response(message, errorCode, arstate, actionUUID.String(), payloadV2, actionContext)

	umhMessageV2, err := generateUMHMessage(instanceUUID, userEmail, models.ActionReply, payloadResponse)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, logger.For(logger.ComponentCommunicator), "Error generating umh message: %w", err)

		return err
	}

	outboundChannel <- &umhMessageV2

	return nil
}

// ConstructActionReplyV2Response creates a new ActionReplyResponseSchemaJson.
func ConstructActionReplyV2Response(
	message string,
	errorCode string,
	actionReplyState models.ActionReplyState,
	actionUUID string,
	payloadV2 map[string]interface{},
	actionContext models.ActionReplyResponseSchemaJsonActionContext,
) models.ActionReplyResponseSchemaJson {
	//  For backwards compatibility, we need to support the old payload format
	//  This will be removed in the future
	var payload interface{}
	if message != "" {
		payload = message
	}

	// if payload is still nil, we use the first message from payloadV2
	if payload == nil {
		// Get first message from map regardless of key
		payload = GetFirstMessageFromMap(payloadV2)
	}

	return models.ActionReplyResponseSchemaJson{
		ActionContext:      actionContext,
		ActionReplyPayload: payload,
		ActionReplyPayloadV2: &models.ActionReplyResponseSchemaJsonActionReplyPayloadV2{
			Message:   message,
			ErrorCode: &errorCode,
			Payload:   payloadV2,
		},
		ActionReplyState: models.ActionReplyResponseSchemaJsonActionReplyState(actionReplyState),
		ActionUUID:       actionUUID,
	}
}

func GetFirstMessageFromMap(msg map[string]interface{}) interface{} {
	for _, v := range msg {
		return v
	}

	return nil
}

// payloadHasDFCState checks whether the action payload contains a "state" field
// inside either "readDFC" or "writeDFC".
// helper function for determining if a DFC state field should be logged to Sentry.
// TODO: Remove this function once the "Write flows" feature is fully implemented.
func payloadHasDFCState(actionPayload any) bool {
	m, ok := actionPayload.(map[string]any)
	if !ok {
		return false
	}
	for _, key := range []string{"readDFC", "writeDFC"} {
		if dfc, ok := m[key]; ok {
			if dfcMap, ok := dfc.(map[string]any); ok {
				if _, ok := dfcMap["state"]; ok {
					return true
				}
			}
		}
	}
	return false
}
