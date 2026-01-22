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

package example_child_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	example_child "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild"
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

func (m *MockConnectionPool) Acquire() (example_child.Connection, error) {
	if m.acquireErr != nil {
		return nil, m.acquireErr
	}

	conn := &MockConnection{ID: "test-conn-1", Active: true}
	m.connections[conn.ID] = conn

	return conn, nil
}

func (m *MockConnectionPool) Release(conn example_child.Connection) error {
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

func (m *MockConnectionPool) HealthCheck(conn example_child.Connection) error {
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

var _ = Describe("ExamplechildDependencies", func() {
	var (
		logger   *zap.SugaredLogger
		mockPool *MockConnectionPool
		identity fsmv2.Identity
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
		mockPool = NewMockConnectionPool()
		identity = fsmv2.Identity{ID: "test-id", WorkerType: "child"}
	})

	Describe("NewExamplechildDependencies", func() {
		It("should create dependencies with valid logger and pool", func() {
			deps := example_child.NewExamplechildDependencies(mockPool, logger, nil, identity)

			Expect(deps).NotTo(BeNil())
			Expect(deps.GetLogger()).NotTo(BeNil())
		})
	})

	Describe("GetConnectionPool", func() {
		It("should return the original connection pool", func() {
			deps := example_child.NewExamplechildDependencies(mockPool, logger, nil, identity)

			pool := deps.GetConnectionPool()
			Expect(pool).NotTo(BeNil())

			mockPoolTyped, ok := pool.(*MockConnectionPool)
			Expect(ok).To(BeTrue())
			Expect(mockPoolTyped).To(Equal(mockPool))
		})
	})
})
