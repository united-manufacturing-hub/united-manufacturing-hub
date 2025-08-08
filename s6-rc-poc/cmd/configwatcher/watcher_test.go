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

package configwatcher_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"s6-rc-poc/cmd/configwatcher"
)

func TestFileWatcher_ConfigChanges(t *testing.T) {
	// Create temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	// Initial config
	initialConfig := `services:
  - name: test-service
    desired_state: up
    executable: /bin/sleep
    parameters:
      0: "30"
`

	if err := os.WriteFile(configPath, []byte(initialConfig), 0o600); err != nil {
		t.Fatalf("Failed to write initial config: %v", err)
	}

	// Create watcher
	watcher := configwatcher.NewFileWatcher()
	defer func() { _ = watcher.Stop() }()

	if err := watcher.Start(configPath); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait for initial load and consume any initial events
	time.Sleep(100 * time.Millisecond)
	// Drain any initial events
	for {
		select {
		case <-watcher.Events():
			// consume event
		default:
			goto drained
		}
	}
drained:

	// Initial state is loaded (no need to verify internal state)

	// Test 1: Add a new service
	updatedConfig := `services:
  - name: test-service
    desired_state: up
    executable: /bin/sleep
    parameters:
      0: "30"
  - name: new-service
    desired_state: down
    executable: /bin/true
    parameters: {}
`

	if err := os.WriteFile(configPath, []byte(updatedConfig), 0o600); err != nil {
		t.Fatalf("Failed to write updated config: %v", err)
	}

	// Wait for file system event and processing
	time.Sleep(200 * time.Millisecond)

	// Check for Created event (might be first or second event)
	for range 3 { // Try up to 3 events
		select {
		case event := <-watcher.Events():
			if created, ok := event.(configwatcher.EventCreated); ok {
				if created.Name == "new-service" {
					if created.DesiredState != configwatcher.Down {
						t.Errorf("Expected Down state, got %v", created.DesiredState)
					}

					goto testStateChange
				}
			}
		case <-time.After(500 * time.Millisecond):
		}
	}

	t.Error("Expected EventCreated for 'new-service'")

	return

testStateChange:

	// Test 2: Change state of existing service
	stateChangeConfig := `services:
  - name: test-service
    desired_state: down
    executable: /bin/sleep
    parameters:
      0: "30"
  - name: new-service
    desired_state: down
    executable: /bin/true
    parameters: {}
`

	if err := os.WriteFile(configPath, []byte(stateChangeConfig), 0o600); err != nil {
		t.Fatalf("Failed to write state change config: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Check for StateChanged event
	for range 3 { // Try up to 3 events
		select {
		case event := <-watcher.Events():
			if stateChanged, ok := event.(configwatcher.EventStateChanged); ok {
				if stateChanged.Name == "test-service" {
					if stateChanged.DesiredState != configwatcher.Down {
						t.Errorf("Expected Down state, got %v", stateChanged.DesiredState)
					}

					goto testDelete
				}
			}
		case <-time.After(500 * time.Millisecond):
		}
	}

	t.Error("Expected EventStateChanged for 'test-service'")

	return

testDelete:

	// Test 3: Remove a service
	removeServiceConfig := `services:
  - name: test-service
    desired_state: down
    executable: /bin/sleep
    parameters:
      0: "30"
`

	if err := os.WriteFile(configPath, []byte(removeServiceConfig), 0o600); err != nil {
		t.Fatalf("Failed to write remove service config: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Check for Deleted event
	for range 3 { // Try up to 3 events
		select {
		case event := <-watcher.Events():
			if deleted, ok := event.(configwatcher.EventDeleted); ok {
				if deleted.Name == "new-service" {
					return // Test passed
				}
			}
		case <-time.After(500 * time.Millisecond):
		}
	}

	t.Error("Expected EventDeleted for 'new-service'")
}

func TestStringToState(t *testing.T) {
	tests := []struct {
		input    string
		expected configwatcher.State
	}{
		{"up", configwatcher.Up},
		{"down", configwatcher.Down},
		{"invalid", configwatcher.Down}, // defaults to Down
	}

	for _, test := range tests {
		result := configwatcher.StringToState(test.input)
		if result != test.expected {
			t.Errorf("stringToState(%q) = %v, expected %v", test.input, result, test.expected)
		}
	}
}

func TestEqualMaps(t *testing.T) {
	tests := []struct {
		name     string
		a, b     map[int]string
		expected bool
	}{
		{"equal maps", map[int]string{0: "arg1", 1: "arg2"}, map[int]string{0: "arg1", 1: "arg2"}, true},
		{"different values", map[int]string{0: "arg1"}, map[int]string{0: "arg2"}, false},
		{"different lengths", map[int]string{0: "arg1"}, map[int]string{0: "arg1", 1: "arg2"}, false},
		{"empty maps", map[int]string{}, map[int]string{}, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := configwatcher.EqualMaps(test.a, test.b)
			if result != test.expected {
				t.Errorf("equalMaps(%v, %v) = %v, expected %v", test.a, test.b, result, test.expected)
			}
		})
	}
}
