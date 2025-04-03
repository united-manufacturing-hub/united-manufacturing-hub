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

package connection_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/connection"
)

var _ = Describe("Connection FSM", func() {
	var (
		manager   *connection.ConnectionManager
		ctx       context.Context
		cancelCtx context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancelCtx = context.WithCancel(context.Background())
		manager = connection.NewConnectionManager("test")
	})

	AfterEach(func() {
		cancelCtx()
	})

	It("should create a connection instance", func() {
		conn, err := manager.CreateConnectionTest(ctx, "test-connection", "example.com", 80, "generic-asset", "")
		Expect(err).NotTo(HaveOccurred())
		Expect(conn).NotTo(BeNil())
		Expect(conn.Config.Name).To(Equal("test-connection"))
		Expect(conn.Config.Target).To(Equal("example.com"))
		Expect(conn.Config.Port).To(Equal(80))
	})

	It("should transition through states correctly", func() {
		conn, err := manager.CreateConnectionTest(ctx, "test-connection", "example.com", 80, "generic-asset", "")
		Expect(err).NotTo(HaveOccurred())

		// Instance should start in the stopped state
		Expect(conn.GetCurrentFSMState()).To(Equal(connection.OperationalStateStopped))

		// Reconcile the instance to move to the testing state
		var reconciled bool
		err, reconciled = conn.Reconcile(ctx, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(reconciled).To(BeTrue())

		// Allow time for transition to happen
		Eventually(func() string {
			return conn.GetCurrentFSMState()
		}, 2*time.Second).Should(Equal(connection.OperationalStateTesting))

		// Change desired state to stopped
		conn.Config.DesiredState = connection.OperationalStateStopped

		// Reconcile to stop the test
		err, reconciled = conn.Reconcile(ctx, 2)
		Expect(err).NotTo(HaveOccurred())
		Expect(reconciled).To(BeTrue())

		// Stopping might take time
		Eventually(func() string {
			return conn.GetCurrentFSMState()
		}, 2*time.Second).Should(Equal(connection.OperationalStateStopping))

		// Should eventually reach stopped state
		Eventually(func() string {
			return conn.GetCurrentFSMState()
		}, 2*time.Second).Should(Equal(connection.OperationalStateStopped))
	})

	It("should handle instance retrieval", func() {
		// Create an instance
		_, err := manager.CreateConnectionTest(ctx, "test-connection", "example.com", 80, "generic-asset", "")
		Expect(err).NotTo(HaveOccurred())

		// Get instance by name
		conn, exists := manager.GetConnectionByName(ctx, "test-connection")
		Expect(exists).To(BeTrue())
		Expect(conn).NotTo(BeNil())
		Expect(conn.GetCurrentFSMState()).To(Equal(connection.OperationalStateStopped))

		// Get all instances
		instances := manager.BaseFSMManager.GetInstances()
		Expect(instances).To(HaveLen(1))
	})

	It("should implement FSMInstance interface", func() {
		// Verify that Connection implements FSMInstance
		var _ fsm.FSMInstance = (*connection.Connection)(nil)
	})
})
