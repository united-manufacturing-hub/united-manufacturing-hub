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
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

// MockConnectionPool is a mock implementation of ConnectionPool for testing.
type MockConnectionPool struct {
	acquireErr     error
	releaseErr     error
	healthCheckErr error
	connections    map[string]*MockConnection
}

type MockConnection struct {
	ID     string
	Active bool
}

func NewMockConnectionPool() *MockConnectionPool {
	return &MockConnectionPool{
		connections: make(map[string]*MockConnection),
	}
}

func (m *MockConnectionPool) Acquire() (Connection, error) {
	if m.acquireErr != nil {
		return nil, m.acquireErr
	}

	conn := &MockConnection{ID: "test-conn-1", Active: true}
	m.connections[conn.ID] = conn

	return conn, nil
}

func (m *MockConnectionPool) Release(conn Connection) error {
	if m.releaseErr != nil {
		return m.releaseErr
	}

	if mockConn, ok := conn.(*MockConnection); ok {
		if c, exists := m.connections[mockConn.ID]; exists {
			c.Active = false

			delete(m.connections, mockConn.ID)
		}
	}

	return nil
}

func (m *MockConnectionPool) HealthCheck(conn Connection) error {
	if m.healthCheckErr != nil {
		return m.healthCheckErr
	}

	if mockConn, ok := conn.(*MockConnection); ok {
		if !mockConn.Active {
			return errors.New("connection inactive")
		}
	}

	return nil
}

func TestNewChildDependencies(t *testing.T) {
	logger := zap.NewNop().Sugar()
	mockPool := NewMockConnectionPool()
	identity := fsmv2.Identity{ID: "test-id", WorkerType: "child"}

	deps := NewChildDependencies(mockPool, logger, identity)

	if deps == nil {
		t.Fatal("NewChildDependencies returned nil")
	}

	if deps.GetLogger() == nil {
		t.Error("GetLogger() returned nil")
	}

	if deps.GetConnectionPool() == nil {
		t.Error("GetConnectionPool() returned nil")
	}

	if pool, ok := deps.GetConnectionPool().(*MockConnectionPool); !ok || pool != mockPool {
		t.Error("GetConnectionPool() returned wrong instance")
	}
}

func TestChildDependencies_GetConnectionPool(t *testing.T) {
	logger := zap.NewNop().Sugar()
	mockPool := NewMockConnectionPool()
	identity := fsmv2.Identity{ID: "test-id", WorkerType: "child"}

	deps := NewChildDependencies(mockPool, logger, identity)

	pool := deps.GetConnectionPool()
	if mockPoolTyped, ok := pool.(*MockConnectionPool); !ok || mockPoolTyped != mockPool {
		t.Error("GetConnectionPool() did not return the original pool")
	}
}
