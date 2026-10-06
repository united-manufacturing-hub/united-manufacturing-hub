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

// Specs for the fsmv2 nmap wiring inside the connection service: ServiceExists
// consults the fsmv2 manager, and ForceRemoveConnection drops the desired config.

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	fsmv2nmap "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

var _ = Describe("fsmv2 nmap wiring", func() {
	var (
		ctx          context.Context
		cancel       context.CancelFunc
		mockServices *serviceregistry.Registry
	)

	BeforeEach(func() {
		ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(60*time.Second))
		mockServices = serviceregistry.NewMockRegistry()
	})

	AfterEach(func() {
		cancel()

		fsmv2client.SetClient(nil)
	})

	Describe("ServiceExists", func() {
		It("returns false for an unknown connection", func() {
			svc := NewDefaultConnectionService("flag-on-noinstance")
			// ServiceExists consults the fsmv2 manager (GetInstance), so an unknown
			// connection reports false without panicking.
			var exists bool

			Expect(func() {
				exists = svc.ServiceExists(ctx, mockServices.GetFileSystem(), "no-such-connection")
			}).NotTo(Panic())
			Expect(exists).To(BeFalse())
		})

		It("returns true once the fsmv2 manager holds the instance", func() {
			// Stage a global fsmv2 client so a Reconcile can materialise the worker
			// instance (mirrors the harness in pkg/fsmv2/nmap/manager_test.go).
			writer := dynamicchildren.NewWriter()
			reader := &stubStateReader{
				obs: &fsmv2.Observation[simple.Status[fsmv2nmap.NmapStatus]]{
					CollectedAt: time.Now().Add(-100 * time.Millisecond),
					Status: simple.Status[fsmv2nmap.NmapStatus]{
						Result: fsmv2nmap.NmapStatus{PortState: "open", IsRunning: true, Port: 502},
					},
				},
			}
			fsmv2client.SetClient(fsmv2client.NewFSMv2Client(writer, reader))

			connName := "flag-on-live"
			svc := NewDefaultConnectionService(connName)
			cfg := &connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: "192.0.2.10",
					Port:   502,
				},
			}

			// Before any reconcile the manager has no instance.
			Expect(svc.ServiceExists(ctx, mockServices.GetFileSystem(), connName)).To(BeFalse())

			Expect(svc.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, connName)).To(Succeed())
			Expect(svc.StartConnection(ctx, mockServices.GetFileSystem(), connName)).To(Succeed())

			// Drive the connection manager (delegates to the fsmv2 manager's
			// Reconcile) until the worker instance appears.
			tick := uint64(1)
			for range 10 {
				_, _ = svc.ReconcileManager(ctx, mockServices, fsm.SystemSnapshot{Tick: tick, SnapshotTime: time.Now()})
				tick++
			}

			Expect(svc.ServiceExists(ctx, mockServices.GetFileSystem(), connName)).To(BeTrue(),
				"once the fsmv2 manager holds the instance, ServiceExists must report it")
		})
	})

	Describe("ForceRemoveConnection", func() {
		It("removes the config from the reconcile set so the worker despawns", func() {
			connName := "flag-on-forceremove"
			svc := NewDefaultConnectionService(connName)

			cfg := &connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{Target: "192.0.2.20", Port: 502},
			}
			Expect(svc.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, connName)).To(Succeed())
			Expect(svc.nmapConfigs).NotTo(BeEmpty())

			Expect(svc.ForceRemoveConnection(ctx, mockServices.GetFileSystem(), connName)).To(Succeed())

			// The desired config must be gone, else the next reconcile keeps the
			// worker alive despite the force-remove.
			for _, v := range svc.nmapConfigs {
				Expect(v.Name).NotTo(Equal(svc.getNmapName(connName)))
			}
		})
	})
})

// stubStateReader is a deps.StateReader that returns a fixed observation for
// every ref. It mirrors stubManagerReader in pkg/fsmv2/nmap/manager_test.go.
type stubStateReader struct {
	obs *fsmv2.Observation[simple.Status[fsmv2nmap.NmapStatus]]
	err error
}

func (s *stubStateReader) LoadObservedTyped(_ context.Context, _, _ string, result any) error {
	if s.err != nil {
		return s.err
	}

	if s.obs == nil {
		return nil
	}

	out, ok := result.(*fsmv2.Observation[simple.Status[fsmv2nmap.NmapStatus]])
	if !ok {
		return nil
	}

	*out = *s.obs

	return nil
}
