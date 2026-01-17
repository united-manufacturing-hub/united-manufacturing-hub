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

// TODO: move to ginkgo/gomega

package factory_test

import (
	"context"
	"sync"
	"testing"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
)

// mockWorker is a minimal Worker implementation for testing.
type mockWorker struct {
	identity fsmv2.Identity
}

func (m *mockWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	return nil, nil
}

func (m *mockWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
	return config.DesiredState{}, nil
}

func (m *mockWorker) GetInitialState() fsmv2.State[any, any] {
	return nil
}

func TestRegisterWorkerType(t *testing.T) {
	// Reset registry before test
	factory.ResetRegistry()

	tests := []struct {
		name        string
		workerType  string
		wantErr     bool
		errContains string
	}{
		{
			name:       "register new worker type",
			workerType: "mqtt_client",
			wantErr:    false,
		},
		{
			name:        "register duplicate worker type",
			workerType:  "mqtt_client",
			wantErr:     true,
			errContains: "already registered",
		},
		{
			name:        "register empty worker type",
			workerType:  "",
			wantErr:     true,
			errContains: "empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factoryFunc := func(id fsmv2.Identity, _ *zap.SugaredLogger, _ fsmv2.StateReader) fsmv2.Worker {
				return &mockWorker{identity: id}
			}

			err := factory.RegisterFactoryByType(tt.workerType, factoryFunc)

			if tt.wantErr {
				if err == nil {
					t.Errorf("RegisterWorkerType() expected error containing %q, got nil", tt.errContains)
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("RegisterWorkerType() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("RegisterWorkerType() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestNewWorker(t *testing.T) {
	// Reset registry before test
	factory.ResetRegistry()

	// Register a worker type for testing
	err := factory.RegisterFactoryByType("test_worker", func(id fsmv2.Identity, _ *zap.SugaredLogger, _ fsmv2.StateReader) fsmv2.Worker {
		return &mockWorker{identity: id}
	})
	if err != nil {
		t.Fatalf("failed to register test worker: %v", err)
	}

	tests := []struct {
		name        string
		workerType  string
		identity    fsmv2.Identity
		wantErr     bool
		errContains string
	}{
		{
			name:       "create registered worker type",
			workerType: "test_worker",
			identity: fsmv2.Identity{
				ID:         "test-123",
				Name:       "Test Worker",
				WorkerType: "test_worker",
			},
			wantErr: false,
		},
		{
			name:        "create unknown worker type",
			workerType:  "unknown_worker",
			identity:    fsmv2.Identity{ID: "test-456", Name: "Unknown", WorkerType: "unknown_worker"},
			wantErr:     true,
			errContains: "unknown worker type",
		},
		{
			name:        "create with empty worker type",
			workerType:  "",
			identity:    fsmv2.Identity{ID: "test-789", Name: "Empty", WorkerType: ""},
			wantErr:     true,
			errContains: "empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker, err := factory.NewWorkerByType(tt.workerType, tt.identity, zap.NewNop().Sugar(), nil)

			if tt.wantErr {
				if err == nil {
					t.Errorf("NewWorker() expected error containing %q, got nil", tt.errContains)
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("NewWorker() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("NewWorker() unexpected error = %v", err)
				}

				if worker == nil {
					t.Error("NewWorker() returned nil worker")
				}
				// Verify worker has correct identity
				if mock, ok := worker.(*mockWorker); ok {
					if mock.identity.ID != tt.identity.ID {
						t.Errorf("NewWorker() identity.ID = %v, want %v", mock.identity.ID, tt.identity.ID)
					}
				}
			}
		})
	}
}

func TestConcurrentRegistration(t *testing.T) {
	// Reset registry before test
	factory.ResetRegistry()

	const numGoroutines = 10

	const numWorkerTypes = 5

	var wg sync.WaitGroup

	errors := make(chan error, numGoroutines*numWorkerTypes)

	// Launch multiple goroutines trying to register worker types concurrently
	for i := range numGoroutines {
		wg.Add(1)

		go func(goroutineID int) {
			defer wg.Done()

			for j := range numWorkerTypes {
				workerType := "worker_" + string(rune('A'+j))

				err := factory.RegisterFactoryByType(workerType, func(id fsmv2.Identity, _ *zap.SugaredLogger, _ fsmv2.StateReader) fsmv2.Worker {
					return &mockWorker{identity: id}
				})
				if err != nil {
					errors <- err
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Count errors - we expect exactly (numGoroutines-1) * numWorkerTypes duplicate registration errors
	// because only the first registration of each worker type should succeed
	errorCount := 0
	for range errors {
		errorCount++
	}

	expectedErrors := (numGoroutines - 1) * numWorkerTypes
	if errorCount != expectedErrors {
		t.Errorf("Concurrent registration: got %d errors, want %d (indicates missing mutex protection)", errorCount, expectedErrors)
	}
}

func TestConcurrentCreation(t *testing.T) {
	// Reset registry before test
	factory.ResetRegistry()

	// Register worker types
	for i := range 3 {
		workerType := "concurrent_worker_" + string(rune('A'+i))

		err := factory.RegisterFactoryByType(workerType, func(id fsmv2.Identity, _ *zap.SugaredLogger, _ fsmv2.StateReader) fsmv2.Worker {
			return &mockWorker{identity: id}
		})
		if err != nil {
			t.Fatalf("failed to register worker type: %v", err)
		}
	}

	const numGoroutines = 20

	var wg sync.WaitGroup

	errors := make(chan error, numGoroutines)
	workers := make(chan fsmv2.Worker, numGoroutines)

	// Launch multiple goroutines creating workers concurrently
	for i := range numGoroutines {
		wg.Add(1)

		go func(goroutineID int) {
			defer wg.Done()

			workerType := "concurrent_worker_" + string(rune('A'+(goroutineID%3)))
			identity := fsmv2.Identity{
				ID:         "worker-" + string(rune('0'+goroutineID)),
				Name:       "Concurrent Worker",
				WorkerType: workerType,
			}

			worker, err := factory.NewWorkerByType(workerType, identity, zap.NewNop().Sugar(), nil)
			if err != nil {
				errors <- err
			} else {
				workers <- worker
			}
		}(i)
	}

	wg.Wait()
	close(errors)
	close(workers)

	// All creations should succeed
	errorCount := 0
	for range errors {
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("Concurrent creation: got %d errors, want 0 (indicates race condition)", errorCount)
	}

	// Verify we created the expected number of workers
	workerCount := 0
	for range workers {
		workerCount++
	}

	if workerCount != numGoroutines {
		t.Errorf("Concurrent creation: got %d workers, want %d", workerCount, numGoroutines)
	}
}

// Helper function to check if error message contains substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}

func TestListRegisteredTypes_EmptyRegistry(t *testing.T) {
	// Reset registry before test
	factory.ResetRegistry()

	types := factory.ListRegisteredTypes()

	if types == nil {
		t.Error("ListRegisteredTypes() returned nil, want empty slice")
	}

	if len(types) != 0 {
		t.Errorf("ListRegisteredTypes() returned %d types, want 0", len(types))
	}
}

func TestListRegisteredTypes_SingleRegistration(t *testing.T) {
	// Reset registry before test
	factory.ResetRegistry()

	// Register one worker type
	err := factory.RegisterFactoryByType("mqtt_client", func(id fsmv2.Identity, _ *zap.SugaredLogger, _ fsmv2.StateReader) fsmv2.Worker {
		return &mockWorker{identity: id}
	})
	if err != nil {
		t.Fatalf("failed to register worker type: %v", err)
	}

	types := factory.ListRegisteredTypes()

	if len(types) != 1 {
		t.Errorf("ListRegisteredTypes() returned %d types, want 1", len(types))
	}

	if types[0] != "mqtt_client" {
		t.Errorf("ListRegisteredTypes() returned %q, want %q", types[0], "mqtt_client")
	}
}

func TestListRegisteredTypes_MultipleRegistrations(t *testing.T) {
	// Reset registry before test
	factory.ResetRegistry()

	// Register multiple worker types
	workerTypes := []string{"mqtt_client", "modbus_server", "opcua_client"}

	for _, wt := range workerTypes {
		err := factory.RegisterFactoryByType(wt, func(id fsmv2.Identity, _ *zap.SugaredLogger, _ fsmv2.StateReader) fsmv2.Worker {
			return &mockWorker{identity: id}
		})
		if err != nil {
			t.Fatalf("failed to register worker type %q: %v", wt, err)
		}
	}

	types := factory.ListRegisteredTypes()

	if len(types) != len(workerTypes) {
		t.Errorf("ListRegisteredTypes() returned %d types, want %d", len(types), len(workerTypes))
	}

	// Convert slice to map for easier comparison (order not guaranteed)
	typeMap := make(map[string]bool)
	for _, t := range types {
		typeMap[t] = true
	}

	for _, wt := range workerTypes {
		if !typeMap[wt] {
			t.Errorf("ListRegisteredTypes() missing type %q", wt)
		}
	}
}

func TestListRegisteredTypes_ReturnsSliceCopy(t *testing.T) {
	// Reset registry before test
	factory.ResetRegistry()

	// Register a worker type
	err := factory.RegisterFactoryByType("test_worker", func(id fsmv2.Identity, _ *zap.SugaredLogger, _ fsmv2.StateReader) fsmv2.Worker {
		return &mockWorker{identity: id}
	})
	if err != nil {
		t.Fatalf("failed to register worker type: %v", err)
	}

	// Get the list
	types1 := factory.ListRegisteredTypes()

	// Modify the returned slice
	if len(types1) > 0 {
		types1[0] = "modified"
	}

	// Get the list again
	types2 := factory.ListRegisteredTypes()

	// Verify the returned slice is a copy (not modified)
	if types2[0] == "modified" {
		t.Error("ListRegisteredTypes() returned slice is shared with internal registry")
	}

	if types2[0] != "test_worker" {
		t.Errorf("ListRegisteredTypes() second call returned %q, want %q", types2[0], "test_worker")
	}
}

func TestListRegisteredTypes_ConcurrentCalls(t *testing.T) {
	// Reset registry before test
	factory.ResetRegistry()

	// Register worker types
	for i := range 5 {
		workerType := "worker_" + string(rune('A'+i))

		err := factory.RegisterFactoryByType(workerType, func(id fsmv2.Identity, _ *zap.SugaredLogger, _ fsmv2.StateReader) fsmv2.Worker {
			return &mockWorker{identity: id}
		})
		if err != nil {
			t.Fatalf("failed to register worker type: %v", err)
		}
	}

	const numGoroutines = 20

	var wg sync.WaitGroup

	results := make(chan []string, numGoroutines)
	errors := make(chan error, numGoroutines)

	// Launch multiple goroutines calling ListRegisteredTypes concurrently
	for i := range numGoroutines {
		wg.Add(1)

		go func(goroutineID int) {
			defer wg.Done()

			types := factory.ListRegisteredTypes()
			results <- types
		}(i)
	}

	wg.Wait()
	close(results)
	close(errors)

	// Verify all calls returned 5 types
	for result := range results {
		if len(result) != 5 {
			t.Errorf("ListRegisteredTypes() concurrent call returned %d types, want 5", len(result))
		}
	}
}

func TestRegisterWorkerAndSupervisorFactory(t *testing.T) {
	// Reset registry before test
	factory.ResetRegistry()

	tests := []struct {
		name          string
		setupRegistry func() // Optional: pre-register conflicting entries
		wantErr       bool
		errContains   string
	}{
		{
			name:        "register both worker and supervisor successfully",
			wantErr:     false,
			errContains: "",
		},
		{
			name: "fail when worker already registered",
			setupRegistry: func() {
				_ = factory.RegisterFactoryByType("test_worker", func(id fsmv2.Identity, _ *zap.SugaredLogger, _ fsmv2.StateReader) fsmv2.Worker {
					return &mockWorker{identity: id}
				})
			},
			wantErr:     true,
			errContains: "failed to register worker factory",
		},
		{
			name: "fail when supervisor already registered and rollback worker",
			setupRegistry: func() {
				_ = factory.RegisterSupervisorFactoryByType("test_worker", func(cfg interface{}) interface{} {
					return nil
				})
			},
			wantErr:     true,
			errContains: "failed to register supervisor factory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory.ResetRegistry()

			if tt.setupRegistry != nil {
				tt.setupRegistry()
			}

			err := factory.RegisterWorkerAndSupervisorFactoryByType(
				"test_worker",
				func(id fsmv2.Identity, _ *zap.SugaredLogger, _ fsmv2.StateReader) fsmv2.Worker {
					return &mockWorker{identity: id}
				},
				func(cfg interface{}) interface{} {
					return nil
				},
			)

			if tt.wantErr {
				if err == nil {
					t.Errorf("RegisterWorkerAndSupervisorFactory() expected error containing %q, got nil", tt.errContains)
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("RegisterWorkerAndSupervisorFactory() error = %v, want error containing %q", err, tt.errContains)
				}

				// If supervisor registration failed, verify worker was rolled back
				if contains(err.Error(), "failed to register supervisor factory") {
					types := factory.ListRegisteredTypes()
					for _, typ := range types {
						if typ == "test_worker" {
							t.Errorf("Worker factory was not rolled back after supervisor registration failure")
						}
					}
				}
			} else {
				if err != nil {
					t.Errorf("RegisterWorkerAndSupervisorFactory() unexpected error = %v", err)
				}

				// Verify both worker and supervisor were registered
				types := factory.ListRegisteredTypes()
				foundWorker := false

				for _, typ := range types {
					if typ == "test_worker" {
						foundWorker = true

						break
					}
				}

				if !foundWorker {
					t.Error("Worker factory was not registered")
				}

				supervisorTypes := factory.ListSupervisorTypes()
				foundSupervisor := false

				for _, typ := range supervisorTypes {
					if typ == "test_worker" {
						foundSupervisor = true

						break
					}
				}

				if !foundSupervisor {
					t.Error("Supervisor factory was not registered")
				}
			}
		})
	}
}

func TestValidateRegistryConsistency(t *testing.T) {
	tests := []struct {
		name                string
		setupRegistry       func()
		wantWorkerOnly      []string
		wantSupervisorOnly  []string
	}{
		{
			name: "empty registries",
			setupRegistry: func() {
				factory.ResetRegistry()
			},
			wantWorkerOnly:     []string{},
			wantSupervisorOnly: []string{},
		},
		{
			name: "consistent registries",
			setupRegistry: func() {
				factory.ResetRegistry()
				_ = factory.RegisterFactoryByType("worker_a", func(id fsmv2.Identity, _ *zap.SugaredLogger, _ fsmv2.StateReader) fsmv2.Worker {
					return &mockWorker{identity: id}
				})
				_ = factory.RegisterSupervisorFactoryByType("worker_a", func(cfg interface{}) interface{} {
					return nil
				})
			},
			wantWorkerOnly:     []string{},
			wantSupervisorOnly: []string{},
		},
		{
			name: "worker registered but not supervisor",
			setupRegistry: func() {
				factory.ResetRegistry()
				_ = factory.RegisterFactoryByType("orphan_worker", func(id fsmv2.Identity, _ *zap.SugaredLogger, _ fsmv2.StateReader) fsmv2.Worker {
					return &mockWorker{identity: id}
				})
			},
			wantWorkerOnly:     []string{"orphan_worker"},
			wantSupervisorOnly: []string{},
		},
		{
			name: "supervisor registered but not worker",
			setupRegistry: func() {
				factory.ResetRegistry()
				_ = factory.RegisterSupervisorFactoryByType("orphan_supervisor", func(cfg interface{}) interface{} {
					return nil
				})
			},
			wantWorkerOnly:     []string{},
			wantSupervisorOnly: []string{"orphan_supervisor"},
		},
		{
			name: "mixed inconsistencies",
			setupRegistry: func() {
				factory.ResetRegistry()
				// Both registered
				_ = factory.RegisterFactoryByType("consistent", func(id fsmv2.Identity, _ *zap.SugaredLogger, _ fsmv2.StateReader) fsmv2.Worker {
					return &mockWorker{identity: id}
				})
				_ = factory.RegisterSupervisorFactoryByType("consistent", func(cfg interface{}) interface{} {
					return nil
				})
				// Worker only
				_ = factory.RegisterFactoryByType("worker_only", func(id fsmv2.Identity, _ *zap.SugaredLogger, _ fsmv2.StateReader) fsmv2.Worker {
					return &mockWorker{identity: id}
				})
				// Supervisor only
				_ = factory.RegisterSupervisorFactoryByType("supervisor_only", func(cfg interface{}) interface{} {
					return nil
				})
			},
			wantWorkerOnly:     []string{"worker_only"},
			wantSupervisorOnly: []string{"supervisor_only"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupRegistry()

			workerOnly, supervisorOnly := factory.ValidateRegistryConsistency()

			// Check worker-only types
			if len(workerOnly) != len(tt.wantWorkerOnly) {
				t.Errorf("ValidateRegistryConsistency() workerOnly count = %d, want %d", len(workerOnly), len(tt.wantWorkerOnly))
			}

			workerOnlyMap := make(map[string]bool)
			for _, w := range workerOnly {
				workerOnlyMap[w] = true
			}

			for _, want := range tt.wantWorkerOnly {
				if !workerOnlyMap[want] {
					t.Errorf("ValidateRegistryConsistency() missing worker-only type %q", want)
				}
			}

			// Check supervisor-only types
			if len(supervisorOnly) != len(tt.wantSupervisorOnly) {
				t.Errorf("ValidateRegistryConsistency() supervisorOnly count = %d, want %d", len(supervisorOnly), len(tt.wantSupervisorOnly))
			}

			supervisorOnlyMap := make(map[string]bool)
			for _, s := range supervisorOnly {
				supervisorOnlyMap[s] = true
			}

			for _, want := range tt.wantSupervisorOnly {
				if !supervisorOnlyMap[want] {
					t.Errorf("ValidateRegistryConsistency() missing supervisor-only type %q", want)
				}
			}
		})
	}
}

func TestListSupervisorTypes(t *testing.T) {
	tests := []struct {
		name          string
		setupRegistry func()
		wantTypes     []string
	}{
		{
			name: "empty registry",
			setupRegistry: func() {
				factory.ResetRegistry()
			},
			wantTypes: []string{},
		},
		{
			name: "single supervisor",
			setupRegistry: func() {
				factory.ResetRegistry()
				_ = factory.RegisterSupervisorFactoryByType("supervisor_a", func(cfg interface{}) interface{} {
					return nil
				})
			},
			wantTypes: []string{"supervisor_a"},
		},
		{
			name: "multiple supervisors",
			setupRegistry: func() {
				factory.ResetRegistry()
				_ = factory.RegisterSupervisorFactoryByType("supervisor_a", func(cfg interface{}) interface{} {
					return nil
				})
				_ = factory.RegisterSupervisorFactoryByType("supervisor_b", func(cfg interface{}) interface{} {
					return nil
				})
				_ = factory.RegisterSupervisorFactoryByType("supervisor_c", func(cfg interface{}) interface{} {
					return nil
				})
			},
			wantTypes: []string{"supervisor_a", "supervisor_b", "supervisor_c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupRegistry()

			types := factory.ListSupervisorTypes()

			if len(types) != len(tt.wantTypes) {
				t.Errorf("ListSupervisorTypes() returned %d types, want %d", len(types), len(tt.wantTypes))
			}

			typeMap := make(map[string]bool)
			for _, typ := range types {
				typeMap[typ] = true
			}

			for _, want := range tt.wantTypes {
				if !typeMap[want] {
					t.Errorf("ListSupervisorTypes() missing type %q", want)
				}
			}
		})
	}
}
