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

package snapshot

import (
	"testing"
	"time"
)

func TestExamplechildObservedState_GetTimestamp(t *testing.T) {
	now := time.Now()
	observed := ExamplechildObservedState{
		CollectedAt: now,
	}

	if observed.GetTimestamp() != now {
		t.Errorf("GetTimestamp() = %v, want %v", observed.GetTimestamp(), now)
	}
}

func TestExamplechildObservedState_GetObservedDesiredState(t *testing.T) {
	observed := ExamplechildObservedState{
		CollectedAt: time.Now(),
		State:       "running_connected",
	}

	desired := observed.GetObservedDesiredState()
	if desired == nil {
		t.Fatal("GetObservedDesiredState() returned nil")
	}
}

func TestExamplechildDesiredState_ShutdownRequested(t *testing.T) {
	tests := []struct {
		name     string
		shutdown bool
		want     bool
	}{
		{"not requested", false, false},
		{"requested", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desired := &ExamplechildDesiredState{}
			desired.SetShutdownRequested(tt.shutdown)

			if got := desired.IsShutdownRequested(); got != tt.want {
				t.Errorf("IsShutdownRequested() = %v, want %v", got, tt.want)
			}
		})
	}
}
