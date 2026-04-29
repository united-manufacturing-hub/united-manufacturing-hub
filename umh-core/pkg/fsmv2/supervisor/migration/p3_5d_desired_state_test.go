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

package migration_test

import (
	"encoding/json"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/migration"
)

func TestMigrateP3_5dDesiredState(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		wantChanged     bool
		wantShutdown    bool
		wantStateAbsent bool
		wantErr         bool
	}{
		{
			name:            "state stopped promotes ShutdownRequested",
			input:           `{"state":"stopped","ShutdownRequested":false,"extra":"value"}`,
			wantChanged:     true,
			wantShutdown:    true,
			wantStateAbsent: true,
		},
		{
			name:            "state stopped overrides ShutdownRequested already true",
			input:           `{"state":"stopped","ShutdownRequested":true}`,
			wantChanged:     true,
			wantShutdown:    true,
			wantStateAbsent: true,
		},
		{
			name:            "state running is no-op",
			input:           `{"state":"running","ShutdownRequested":false}`,
			wantChanged:     false,
			wantShutdown:    false,
			wantStateAbsent: false,
		},
		{
			name:            "no state field is no-op",
			input:           `{"ShutdownRequested":false}`,
			wantChanged:     false,
			wantShutdown:    false,
			wantStateAbsent: false,
		},
		{
			name:        "empty input is no-op",
			input:       "",
			wantChanged: false,
		},
		{
			name:    "malformed JSON returns error",
			input:   `{not-valid-json`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var inputBytes []byte
			if tt.input != "" {
				inputBytes = []byte(tt.input)
			}

			migrated, changed, err := migration.MigrateP3_5dDesiredState(inputBytes)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if changed != tt.wantChanged {
				t.Errorf("changed = %v, want %v", changed, tt.wantChanged)
			}

			if !tt.wantChanged {
				if migrated != nil {
					t.Errorf("expected nil migrated bytes when unchanged, got %q", migrated)
				}
				return
			}

			// Verify the migrated document.
			var doc map[string]interface{}
			if err := json.Unmarshal(migrated, &doc); err != nil {
				t.Fatalf("migrated bytes are invalid JSON: %v", err)
			}

			shutdownVal, ok := doc["ShutdownRequested"]
			if !ok {
				t.Error("ShutdownRequested missing from migrated doc")
			} else if shutdownVal != tt.wantShutdown {
				t.Errorf("ShutdownRequested = %v, want %v", shutdownVal, tt.wantShutdown)
			}

			if tt.wantStateAbsent {
				if _, hasState := doc["state"]; hasState {
					t.Error("migrated doc still contains 'state' key, expected removal")
				}
			}
		})
	}
}

func TestMigrateP3_5dDesiredState_PreservesExtraFields(t *testing.T) {
	input := `{"state":"stopped","ShutdownRequested":false,"ChildrenSpecs":[{"name":"child-1"}],"custom":42}`

	migrated, changed, err := migration.MigrateP3_5dDesiredState([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}

	var doc map[string]interface{}
	if err := json.Unmarshal(migrated, &doc); err != nil {
		t.Fatalf("migrated bytes are invalid JSON: %v", err)
	}

	// "state" key must be gone.
	if _, ok := doc["state"]; ok {
		t.Error("'state' key still present after migration")
	}

	// ShutdownRequested must be true.
	if doc["ShutdownRequested"] != true {
		t.Errorf("ShutdownRequested = %v, want true", doc["ShutdownRequested"])
	}

	// Other fields must be preserved.
	if doc["custom"] != float64(42) {
		t.Errorf("custom field lost: got %v", doc["custom"])
	}
	if _, ok := doc["ChildrenSpecs"]; !ok {
		t.Error("ChildrenSpecs field lost")
	}
}
