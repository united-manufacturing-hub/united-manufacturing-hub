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

package example_child

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func TestNewChildWorker(t *testing.T) {
	logger := zap.NewNop().Sugar()
	mockPool := NewMockConnectionPool()

	worker := NewChildWorker("test-id", "test-child", mockPool, logger)

	if worker == nil {
		t.Fatal("NewChildWorker returned nil")
	}

	if worker.GetInitialState() == nil {
		t.Error("GetInitialState() returned nil")
	}
}

func TestChildWorker_CollectObservedState(t *testing.T) {
	logger := zap.NewNop().Sugar()
	mockPool := NewMockConnectionPool()

	worker := NewChildWorker("test-id", "test-child", mockPool, logger)

	observed, err := worker.CollectObservedState(context.Background())
	if err != nil {
		t.Fatalf("CollectObservedState() error = %v", err)
	}

	if observed == nil {
		t.Fatal("CollectObservedState() returned nil")
	}
}

func TestChildWorker_DeriveDesiredState(t *testing.T) {
	logger := zap.NewNop().Sugar()
	mockPool := NewMockConnectionPool()

	worker := NewChildWorker("test-id", "test-child", mockPool, logger)

	desired, err := worker.DeriveDesiredState(nil)
	if err != nil {
		t.Fatalf("DeriveDesiredState() error = %v", err)
	}

	if desired.State != "connected" {
		t.Errorf("DeriveDesiredState() state = %v, want connected", desired.State)
	}
}
