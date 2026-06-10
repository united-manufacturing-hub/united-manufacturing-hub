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
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// TestFSMLoggerWiring is the load-bearing red gate for ENG-4959.
//
// It pins the invariant that every action constructed by the production switch
// receives a non-nil fsmLogger. The PR #2546 regression omitted the fsmLogger
// field from the EditProtocolConverter and DeployProtocolConverter struct
// literals; any call into the resulting nil interface panics at runtime
// (SIGSEGV nil pointer dereference at offset 0x30 — the itab.fun method
// pointer).
//
// We assert at construction time via reflection because the bug manifests on
// the production switch path (newActionFromPayload), not via the
// New…Action(…) constructors that unit tests typically exercise — the
// constructors wire the field correctly. Reading the unexported field is
// permitted here because this test is in the same package.
func TestFSMLoggerWiring(t *testing.T) {
	log := logger.For(logger.ComponentCommunicator)
	fsmLogger := deps.NewFSMLogger(log)

	build := func(actionType models.ActionType) Action {
		return newActionFromPayload(
			uuid.New(),
			models.ActionMessagePayload{ActionType: actionType, ActionUUID: uuid.New()},
			"user@example.com",
			make(chan *models.UMHMessage, 1),
			nil,
			nil,
			log,
			fsmLogger,
		)
	}

	// T1.1 — EditProtocolConverter
	t.Run("EditProtocolConverter has non-nil fsmLogger", func(t *testing.T) {
		action := build(models.EditProtocolConverter)

		if action == nil {
			t.Fatalf("expected non-nil action for EditProtocolConverter")
		}

		assertFSMLoggerWired(t, action)
	})

	// T1.2 — DeployProtocolConverter
	t.Run("DeployProtocolConverter has non-nil fsmLogger", func(t *testing.T) {
		action := build(models.DeployProtocolConverter)

		if action == nil {
			t.Fatalf("expected non-nil action for DeployProtocolConverter")
		}

		assertFSMLoggerWired(t, action)
	})

	// T1.3 — forward-looking sweep: any future action that adds an fsmLogger
	// field must wire it too. Today, only EditProtocolConverter and
	// DeployProtocolConverter expose the field; the sweep catches regressions
	// the next time a PR adds the field to a new action type.
	t.Run("all action types with fsmLogger field receive a non-nil value", func(t *testing.T) {
		for _, at := range allKnownActionTypes() {
			t.Run(string(at), func(t *testing.T) {
				action := build(at)
				if action == nil {
					// Unknown / unsupported in this test catalog; skip.
					return
				}

				if !hasFSMLoggerField(action) {
					return
				}

				assertFSMLoggerWired(t, action)
			})
		}
	})
}

// hasFSMLoggerField reports whether the action struct exposes a field literally
// named "fsmLogger" (the field added by PR #2546).
func hasFSMLoggerField(action Action) bool {
	v := reflect.ValueOf(action)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return false
	}

	return v.FieldByName("fsmLogger").IsValid()
}

// assertFSMLoggerWired fails the test if the action's fsmLogger field is the
// zero-value nil interface.
func assertFSMLoggerWired(t *testing.T, action Action) {
	t.Helper()

	v := reflect.ValueOf(action)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	field := v.FieldByName("fsmLogger")
	if !field.IsValid() {
		t.Fatalf("action %T has no fsmLogger field", action)
	}

	if field.Kind() != reflect.Interface {
		t.Fatalf("action %T fsmLogger is not an interface (got %s)", action, field.Kind())
	}

	if field.IsNil() {
		t.Fatalf("action %T fsmLogger is nil — production switch construction omits wire-up (ENG-4959, PR #2546 regression)", action)
	}
}

// allKnownActionTypes enumerates the action types the switch handles. Listed
// explicitly because the constants are declared per-action across the models
// package and there is no central registry. Adding a new action type to the
// switch should also add it here so the sweep keeps catching regressions.
func allKnownActionTypes() []models.ActionType {
	return []models.ActionType{
		models.EditInstance,
		models.DeployDataFlowComponent,
		models.DeleteDataFlowComponent,
		models.GetDataFlowComponent,
		models.EditDataFlowComponent,
		models.GetLogs,
		models.GetConfigFile,
		models.SetConfigFile,
		models.GetDataFlowComponentMetrics, //nolint:staticcheck // Deprecated but kept for back compat
		models.DeployProtocolConverter,
		models.EditProtocolConverter,
		models.SaveProtocolConverter,
		models.GetProtocolConverter,
		models.GetMetrics,
		models.DeleteProtocolConverter,
		models.AddDataModel,
		models.DeleteDataModel,
		models.EditDataModel,
		models.GetDataModel,
		models.DeleteStreamProcessor,
		models.EditStreamProcessor,
		models.DeployStreamProcessor,
		models.GetStreamProcessor,
	}
}
