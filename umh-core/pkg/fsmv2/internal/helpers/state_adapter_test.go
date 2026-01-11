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

package helpers

import (
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

// Test types for the state adapter tests.
type testObserved struct {
	Value     string
	Timestamp time.Time
}

func (t testObserved) GetObservedDesiredState() fsmv2.DesiredState {
	return &testDesired{}
}

func (t testObserved) GetTimestamp() time.Time {
	return t.Timestamp
}

type testDesired struct {
	shutdown bool
}

func (t *testDesired) IsShutdownRequested() bool {
	return t.shutdown
}

type wrongObserved struct {
	Value string
}

func (w wrongObserved) GetObservedDesiredState() fsmv2.DesiredState {
	return &testDesired{}
}

func (w wrongObserved) GetTimestamp() time.Time {
	return time.Now()
}

type wrongDesired struct {
	value int
}

func (w *wrongDesired) IsShutdownRequested() bool {
	return false
}

func TestConvertSnapshot_SuccessfulConversion(t *testing.T) {
	// Arrange
	identity := fsmv2.Identity{
		ID:         "test-id",
		Name:       "test-name",
		WorkerType: "test-worker",
	}
	observed := testObserved{
		Value:     "observed-value",
		Timestamp: time.Now(),
	}
	desired := &testDesired{shutdown: true}

	rawSnapshot := fsmv2.Snapshot{
		Identity: identity,
		Observed: observed,
		Desired:  desired,
	}

	// Act
	typedSnap := ConvertSnapshot[testObserved, *testDesired](rawSnapshot)

	// Assert
	if typedSnap.Identity.ID != identity.ID {
		t.Errorf("Expected Identity.ID %s, got %s", identity.ID, typedSnap.Identity.ID)
	}

	if typedSnap.Identity.Name != identity.Name {
		t.Errorf("Expected Identity.Name %s, got %s", identity.Name, typedSnap.Identity.Name)
	}

	if typedSnap.Identity.WorkerType != identity.WorkerType {
		t.Errorf("Expected Identity.WorkerType %s, got %s", identity.WorkerType, typedSnap.Identity.WorkerType)
	}

	if typedSnap.Observed.Value != observed.Value {
		t.Errorf("Expected Observed.Value %s, got %s", observed.Value, typedSnap.Observed.Value)
	}

	if !typedSnap.Desired.IsShutdownRequested() {
		t.Error("Expected Desired.IsShutdownRequested() to be true")
	}
}

func TestConvertSnapshot_ValueTypeObserved(t *testing.T) {
	// Arrange - test with value type (non-pointer) for observed
	identity := fsmv2.Identity{
		ID:         "test-id",
		Name:       "test-name",
		WorkerType: "test-worker",
	}
	observed := testObserved{
		Value:     "value-type-test",
		Timestamp: time.Now(),
	}
	desired := &testDesired{shutdown: false}

	rawSnapshot := fsmv2.Snapshot{
		Identity: identity,
		Observed: observed,
		Desired:  desired,
	}

	// Act
	typedSnap := ConvertSnapshot[testObserved, *testDesired](rawSnapshot)

	// Assert
	if typedSnap.Observed.Value != "value-type-test" {
		t.Errorf("Expected Observed.Value 'value-type-test', got %s", typedSnap.Observed.Value)
	}
}

func TestConvertSnapshot_PointerTypeDesired(t *testing.T) {
	// Arrange - test with pointer type for desired
	identity := fsmv2.Identity{
		ID:         "test-id",
		Name:       "test-name",
		WorkerType: "test-worker",
	}
	observed := testObserved{
		Value:     "test",
		Timestamp: time.Now(),
	}
	desired := &testDesired{shutdown: true}

	rawSnapshot := fsmv2.Snapshot{
		Identity: identity,
		Observed: observed,
		Desired:  desired,
	}

	// Act
	typedSnap := ConvertSnapshot[testObserved, *testDesired](rawSnapshot)

	// Assert
	if typedSnap.Desired == nil {
		t.Error("Expected Desired to not be nil")
	}

	if !typedSnap.Desired.IsShutdownRequested() {
		t.Error("Expected Desired.IsShutdownRequested() to be true")
	}
}

func TestConvertSnapshot_PanicOnWrongObservedType(t *testing.T) {
	// Arrange
	identity := fsmv2.Identity{
		ID:         "test-id",
		Name:       "test-name",
		WorkerType: "test-worker",
	}
	observed := wrongObserved{Value: "wrong"}
	desired := &testDesired{shutdown: false}

	rawSnapshot := fsmv2.Snapshot{
		Identity: identity,
		Observed: observed,
		Desired:  desired,
	}

	// Act & Assert
	defer func() {
		r := recover()
		if r == nil {
			t.Error("Expected panic for wrong observed type")

			return
		}

		panicMsg, ok := r.(string)
		if !ok {
			t.Errorf("Expected panic message to be string, got %T", r)

			return
		}

		if panicMsg == "" {
			t.Error("Expected non-empty panic message")
		}
		// Check for descriptive message
		if !containsSubstring(panicMsg, "Observed") {
			t.Errorf("Expected panic message to mention 'Observed', got: %s", panicMsg)
		}
	}()

	ConvertSnapshot[testObserved, *testDesired](rawSnapshot)
}

func TestConvertSnapshot_PanicOnWrongDesiredType(t *testing.T) {
	// Arrange
	identity := fsmv2.Identity{
		ID:         "test-id",
		Name:       "test-name",
		WorkerType: "test-worker",
	}
	observed := testObserved{Value: "test", Timestamp: time.Now()}
	desired := &wrongDesired{value: 42}

	rawSnapshot := fsmv2.Snapshot{
		Identity: identity,
		Observed: observed,
		Desired:  desired,
	}

	// Act & Assert
	defer func() {
		r := recover()
		if r == nil {
			t.Error("Expected panic for wrong desired type")

			return
		}

		panicMsg, ok := r.(string)
		if !ok {
			t.Errorf("Expected panic message to be string, got %T", r)

			return
		}

		if panicMsg == "" {
			t.Error("Expected non-empty panic message")
		}
		// Check for descriptive message
		if !containsSubstring(panicMsg, "Desired") {
			t.Errorf("Expected panic message to mention 'Desired', got: %s", panicMsg)
		}
	}()

	ConvertSnapshot[testObserved, *testDesired](rawSnapshot)
}

func TestConvertSnapshot_PanicOnNonSnapshotInput(t *testing.T) {
	// Arrange - pass something that's not a Snapshot
	notASnapshot := "not a snapshot"

	// Act & Assert
	defer func() {
		r := recover()
		if r == nil {
			t.Error("Expected panic for non-Snapshot input")

			return
		}

		panicMsg, ok := r.(string)
		if !ok {
			t.Errorf("Expected panic message to be string, got %T", r)

			return
		}

		if !containsSubstring(panicMsg, "Snapshot") {
			t.Errorf("Expected panic message to mention 'Snapshot', got: %s", panicMsg)
		}
	}()

	ConvertSnapshot[testObserved, *testDesired](notASnapshot)
}

func TestConvertSnapshot_NilObserved(t *testing.T) {
	// Arrange
	identity := fsmv2.Identity{
		ID:         "test-id",
		Name:       "test-name",
		WorkerType: "test-worker",
	}
	desired := &testDesired{shutdown: false}

	rawSnapshot := fsmv2.Snapshot{
		Identity: identity,
		Observed: nil,
		Desired:  desired,
	}

	// Act & Assert
	defer func() {
		r := recover()
		if r == nil {
			t.Error("Expected panic for nil observed")

			return
		}

		panicMsg, ok := r.(string)
		if !ok {
			t.Errorf("Expected panic message to be string, got %T", r)

			return
		}

		if !containsSubstring(panicMsg, "Observed") {
			t.Errorf("Expected panic message to mention 'Observed', got: %s", panicMsg)
		}
	}()

	ConvertSnapshot[testObserved, *testDesired](rawSnapshot)
}

func TestConvertSnapshot_NilDesired(t *testing.T) {
	// Arrange
	identity := fsmv2.Identity{
		ID:         "test-id",
		Name:       "test-name",
		WorkerType: "test-worker",
	}
	observed := testObserved{Value: "test", Timestamp: time.Now()}

	rawSnapshot := fsmv2.Snapshot{
		Identity: identity,
		Observed: observed,
		Desired:  nil,
	}

	// Act & Assert
	defer func() {
		r := recover()
		if r == nil {
			t.Error("Expected panic for nil desired")

			return
		}

		panicMsg, ok := r.(string)
		if !ok {
			t.Errorf("Expected panic message to be string, got %T", r)

			return
		}

		if !containsSubstring(panicMsg, "Desired") {
			t.Errorf("Expected panic message to mention 'Desired', got: %s", panicMsg)
		}
	}()

	ConvertSnapshot[testObserved, *testDesired](rawSnapshot)
}

func TestTypedSnapshot_DirectFieldAccess(t *testing.T) {
	// Arrange
	identity := fsmv2.Identity{
		ID:         "test-id",
		Name:       "test-name",
		WorkerType: "test-worker",
	}
	observed := testObserved{
		Value:     "direct-access-test",
		Timestamp: time.Now(),
	}
	desired := &testDesired{shutdown: true}

	typedSnap := TypedSnapshot[testObserved, *testDesired]{
		Identity: identity,
		Observed: observed,
		Desired:  desired,
	}

	// Assert - verify direct field access works correctly
	if typedSnap.Identity.ID != "test-id" {
		t.Errorf("Expected Identity.ID 'test-id', got %s", typedSnap.Identity.ID)
	}

	if typedSnap.Observed.Value != "direct-access-test" {
		t.Errorf("Expected Observed.Value 'direct-access-test', got %s", typedSnap.Observed.Value)
	}

	if !typedSnap.Desired.IsShutdownRequested() {
		t.Error("Expected Desired.IsShutdownRequested() to be true")
	}
}

// containsSubstring is a helper function to check if a string contains a substring.
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}
