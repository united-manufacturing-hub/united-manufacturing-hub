package factory_test

import (
	"context"
	"sync"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
)

// mockWorker is a minimal Worker implementation for testing.
type mockWorker struct {
	identity fsmv2.Identity
}

func (m *mockWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	return nil, nil
}

func (m *mockWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	return nil, nil
}

func (m *mockWorker) GetInitialState() fsmv2.State {
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
			factoryFunc := func(id fsmv2.Identity) fsmv2.Worker {
				return &mockWorker{identity: id}
			}

			err := factory.RegisterWorkerType(tt.workerType, factoryFunc)

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
	err := factory.RegisterWorkerType("test_worker", func(id fsmv2.Identity) fsmv2.Worker {
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
			worker, err := factory.NewWorker(tt.workerType, tt.identity)

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

				err := factory.RegisterWorkerType(workerType, func(id fsmv2.Identity) fsmv2.Worker {
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

		err := factory.RegisterWorkerType(workerType, func(id fsmv2.Identity) fsmv2.Worker {
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

			worker, err := factory.NewWorker(workerType, identity)
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
