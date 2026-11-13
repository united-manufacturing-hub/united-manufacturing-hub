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

package connection

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	fsmv2nmap "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

const (
	targetHost = "localhost"
	targetPort = 502
)

var _ = Describe("ConnectionService", func() {
	var (
		service        *ConnectionService
		ctx            context.Context
		tick           uint64
		connectionName string
		cancelFunc     context.CancelFunc
		mockServices   *serviceregistry.Registry
	)

	BeforeEach(func() {
		ctx, cancelFunc = context.WithDeadline(context.Background(), time.Now().Add(500*time.Second))
		tick = 1
		connectionName = "test-connection"

		// The service drives the real fsmv2 nmap manager. The manager reads scan
		// results through the global fsmv2 client, which each test stages with a
		// stubbed state reader.
		service = NewDefaultConnectionService(connectionName)
		mockServices = serviceregistry.NewMockRegistry()
	})

	AfterEach(func() {
		cancelFunc()
		fsmv2client.SetClient(nil)
	})

	Describe("AddConnection", func() {
		var (
			cfg *connectionserviceconfig.ConnectionServiceConfig
		)

		BeforeEach(func() {
			// Create a basic config for testing
			cfg = &connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: targetHost,
					Port:   targetPort,
				},
			}
		})

		It("should add a new connection to the nmap manager", func() {
			err := service.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, connectionName)
			Expect(err).NotTo(HaveOccurred())

			// Verify that a Nmap config was added to the service
			Expect(service.nmapConfigs).To(HaveLen(1))

			// Verify the name follows the expected pattern
			nmapName := service.getNmapName(connectionName)
			Expect(service.nmapConfigs[0].Name).To(Equal(nmapName))

			// Verify the desired state is set correctly
			Expect(service.nmapConfigs[0].DesiredFSMState).To(Equal(fsmv2nmap.OperationalStateStopped))
		})

		It("should return error when connection already exists", func() {
			err := service.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, connectionName)
			Expect(err).NotTo(HaveOccurred())

			// Try to add it again
			err = service.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, connectionName)

			Expect(err).To(MatchError(ErrServiceAlreadyExists))
		})

		It("should set up the connection for reconciliation with the nmap manager", func() {
			// Act
			err := service.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, connectionName)
			Expect(err).NotTo(HaveOccurred())

			stageFsmv2Client(nil)

			// Reconcile to ensure the component is passed to nmap manager
			_, reconciled := service.ReconcileManager(ctx, mockServices, fsm.SystemSnapshot{Tick: tick, SnapshotTime: time.Now()})

			// Assert
			Expect(reconciled).To(BeTrue())
			Expect(service.nmapConfigs).To(HaveLen(1))
		})
	})

	Describe("Status", func() {
		var (
			cfg    *connectionserviceconfig.ConnectionServiceConfig
			reader *stubStateReader
		)

		BeforeEach(func() {
			cfg = &connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: targetHost,
					Port:   targetPort,
				},
			}

			reader = stageFsmv2Client(nil)

			err := service.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, connectionName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should report status correctly for an existing component", func() {
			// A stopped connection reports the stopped nmap state.
			_, reconciled := service.ReconcileManager(ctx, mockServices, fsm.SystemSnapshot{Tick: tick, SnapshotTime: time.Now()})
			Expect(reconciled).To(BeTrue())

			status, err := service.Status(ctx, mockServices.GetFileSystem(), connectionName, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(status.NmapFSMState).To(Equal(fsmv2nmap.OperationalStateStopped))

			// Start it and let the worker report an open port.
			reader.obs = freshNmapObservation(fsmv2nmap.NmapStatus{
				Target:    targetHost,
				PortState: string(fsmv2nmap.PortStateOpen),
				Port:      targetPort,
				IsRunning: true,
				ScannedAt: time.Now(),
			})

			err = service.StartConnection(ctx, mockServices.GetFileSystem(), connectionName)
			Expect(err).NotTo(HaveOccurred())

			tick++
			_, reconciled = service.ReconcileManager(ctx, mockServices, fsm.SystemSnapshot{Tick: tick, SnapshotTime: time.Now()})
			Expect(reconciled).To(BeTrue())

			status, err = service.Status(ctx, mockServices.GetFileSystem(), connectionName, tick)

			Expect(err).NotTo(HaveOccurred())
			Expect(status.NmapFSMState).To(Equal(fsmv2nmap.OperationalStateOpen))
			Expect(status.NmapObservedState.ServiceInfo.NmapStatus.IsRunning).To(BeTrue())
			Expect(status.NmapObservedState.ServiceInfo.NmapStatus.LastScan.PortResult.State).To(Equal("open"))
			Expect(status.LastChange).To(Equal(tick))

			service.recentNmapStates[connectionName] = []string{"open", "closed", "open"}

			status, err = service.Status(ctx, mockServices.GetFileSystem(), connectionName, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(status.IsFlaky).To(BeTrue())
		})

		It("should return error for non-existent component", func() {
			// The connection is configured but never reconciled, so the nmap
			// manager holds no instance for it.
			Expect(service.ServiceExists(ctx, mockServices.GetFileSystem(), connectionName)).To(BeFalse())

			_, err := service.Status(ctx, mockServices.GetFileSystem(), connectionName, tick)

			Expect(err).To(MatchError(ErrServiceNotExist))
		})
	})

	Describe("UpdateConnection", func() {
		var (
			cfg        *connectionserviceconfig.ConnectionServiceConfig
			updatedCfg *connectionserviceconfig.ConnectionServiceConfig
		)

		BeforeEach(func() {
			// Initial config
			cfg = &connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: targetHost,
					Port:   targetPort,
				},
			}

			// Updated config with different settings
			updatedCfg = &connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: targetHost,
					Port:   444,
				},
			}

			// Add the component first
			err := service.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, connectionName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update an existing connection", func() {
			// Act - update the component
			err := service.UpdateConnectionInNmapManager(ctx, mockServices.GetFileSystem(), updatedCfg, connectionName)

			// Assert
			Expect(err).NotTo(HaveOccurred())

			// Verify the config was updated but the desired state was preserved
			nmapName := service.getNmapName(connectionName)

			var found bool

			for _, config := range service.nmapConfigs {
				if config.Name == nmapName {
					found = true

					Expect(config.DesiredFSMState).To(Equal(fsmv2nmap.OperationalStateStopped))
					// In a real test, we'd verify the ConnectionServiceConfig was updated as expected
					break
				}
			}

			Expect(found).To(BeTrue())
		})

		It("should return error when component doesn't exist", func() {
			// Act - try to update a non-existent component
			err := service.UpdateConnectionInNmapManager(ctx, mockServices.GetFileSystem(), updatedCfg, "non-existent")

			// Assert
			Expect(err).To(MatchError(ErrServiceNotExist))
		})
	})

	Describe("StartAndStopConnection", func() {
		var (
			cfg *connectionserviceconfig.ConnectionServiceConfig
		)

		BeforeEach(func() {
			// Create a basic config for testing
			cfg = &connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: targetHost,
					Port:   targetPort,
				},
			}

			// Add the component first
			err := service.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, connectionName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should start a connection by changing its desired state", func() {
			// First stop the component
			err := service.StopConnection(ctx, mockServices.GetFileSystem(), connectionName)
			Expect(err).NotTo(HaveOccurred())

			// Verify the desired state was changed to stopped
			nmapName := service.getNmapName(connectionName)

			var foundStopped bool

			for _, config := range service.nmapConfigs {
				if config.Name == nmapName {
					foundStopped = true

					Expect(config.DesiredFSMState).To(Equal(fsmv2nmap.OperationalStateStopped))

					break
				}
			}

			Expect(foundStopped).To(BeTrue())

			// Now start the component
			err = service.StartConnection(ctx, mockServices.GetFileSystem(), connectionName)
			Expect(err).NotTo(HaveOccurred())

			// Verify the desired state was changed to active
			var foundStarted bool

			for _, config := range service.nmapConfigs {
				if config.Name == nmapName {
					foundStarted = true

					Expect(config.DesiredFSMState).To(Equal(fsmv2nmap.OperationalStateOpen))

					break
				}
			}

			Expect(foundStarted).To(BeTrue())
		})

		It("should return error when trying to start/stop non-existent connection", func() {
			// Try to start a non-existent component
			err := service.StartConnection(ctx, mockServices.GetFileSystem(), "non-existent")
			Expect(err).To(MatchError(ErrServiceNotExist))

			// Try to stop a non-existent component
			err = service.StopConnection(ctx, mockServices.GetFileSystem(), "non-existent")
			Expect(err).To(MatchError(ErrServiceNotExist))
		})
	})

	Describe("RemoveConnection", func() {
		var (
			cfg *connectionserviceconfig.ConnectionServiceConfig
		)

		BeforeEach(func() {
			// Create a basic config for testing
			cfg = &connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: targetHost,
					Port:   targetPort,
				},
			}

			// Add the component first
			err := service.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, connectionName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should remove a connection from the nmap manager", func() {
			// Get the initial count
			initialCount := len(service.nmapConfigs)

			// Act - remove the component
			err := service.RemoveConnectionFromNmapManager(ctx, mockServices.GetFileSystem(), connectionName)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(service.nmapConfigs).To(HaveLen(initialCount - 1))

			// Verify the component is no longer in the list
			nmapName := service.getNmapName(connectionName)
			for _, config := range service.nmapConfigs {
				Expect(config.Name).NotTo(Equal(nmapName))
			}
		})

		// Note: removing a non-existent component should not result in an error
		// the remove action will be called multiple times until the component is gone it returns nil
	})

	Describe("ReconcileManager", func() {
		var cfg *connectionserviceconfig.ConnectionServiceConfig

		BeforeEach(func() {
			cfg = &connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: targetHost,
					Port:   targetPort,
				},
			}
		})

		It("should pass configs to the nmap manager for reconciliation", func() {
			stageFsmv2Client(nil)

			err := service.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, connectionName)
			Expect(err).NotTo(HaveOccurred())

			err, reconciled := service.ReconcileManager(ctx, mockServices, fsm.SystemSnapshot{Tick: tick, SnapshotTime: time.Now()})

			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(service.ServiceExists(ctx, mockServices.GetFileSystem(), connectionName)).To(BeTrue())
		})

		It("should retry a started connection until the fsmv2 runtime is ready", func() {
			// No global fsmv2 client: the runtime is not up yet.
			err := service.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, connectionName)
			Expect(err).NotTo(HaveOccurred())
			Expect(service.StartConnection(ctx, mockServices.GetFileSystem(), connectionName)).To(Succeed())

			err, reconciled := service.ReconcileManager(ctx, mockServices, fsm.SystemSnapshot{Tick: tick, SnapshotTime: time.Now()})
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeFalse())
			Expect(service.ServiceExists(ctx, mockServices.GetFileSystem(), connectionName)).To(BeFalse())

			stageFsmv2Client(nil)

			err, reconciled = service.ReconcileManager(ctx, mockServices, fsm.SystemSnapshot{Tick: tick + 1, SnapshotTime: time.Now()})
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(service.ServiceExists(ctx, mockServices.GetFileSystem(), connectionName)).To(BeTrue())
		})

		It("should return the context error when the context is cancelled", func() {
			cancelFunc()

			err, reconciled := service.ReconcileManager(ctx, mockServices, fsm.SystemSnapshot{Tick: tick, SnapshotTime: time.Now()})
			Expect(err).To(MatchError(context.Canceled))
			Expect(reconciled).To(BeFalse())
		})
	})
})

// stageFsmv2Client publishes a global fsmv2 client whose state reader returns
// obs for every worker. Tests change the returned reader's obs to simulate new
// scan results.
func stageFsmv2Client(obs *fsmv2.Observation[simple.Status[fsmv2nmap.NmapStatus]]) *stubStateReader {
	reader := &stubStateReader{obs: obs}
	fsmv2client.SetClient(fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), reader))

	return reader
}

// freshNmapObservation wraps st in a fresh, healthy observation.
func freshNmapObservation(st fsmv2nmap.NmapStatus) *fsmv2.Observation[simple.Status[fsmv2nmap.NmapStatus]] {
	return &fsmv2.Observation[simple.Status[fsmv2nmap.NmapStatus]]{
		CollectedAt: time.Now().Add(-100 * time.Millisecond),
		Status:      simple.Status[fsmv2nmap.NmapStatus]{Result: st},
	}
}
