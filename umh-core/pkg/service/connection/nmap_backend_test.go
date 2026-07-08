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

// RED-phase (TDD) specs for the NMAP_BACKEND=fsmv2 wiring inside the
// connection service. These describe NOT-YET-BUILT behavior and MUST fail to
// compile/run until GREEN implements the assumed production symbols:
//
//   - constants.NmapBackendFSMv2          == "fsmv2"
//   - (*ConnectionService).UsesFsmv2Backend() bool
//     Reports whether NewDefaultConnectionService selected the fsmv2-backed
//     nmap manager (env NMAP_BACKEND == constants.NmapBackendFSMv2).
//
// When the flag is on, NewDefaultConnectionService must:
//   - build the fsmv2 manager via fsmv2nmap.NewFsmv2NmapManager,
//   - set usesFsmv2Backend = true,
//   - leave the S6 nmapService nil, and
//   - make ServiceExists fsmv2-aware (GetInstance, NOT an S6 probe).
//
// When the flag is unset/"fsmv1", every path is byte-identical to today.

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	fsmv2nmap "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

var _ = Describe("NMAP_BACKEND flag wiring", func() {
	var (
		ctx          context.Context
		cancel       context.CancelFunc
		mockServices *serviceregistry.Registry
	)

	BeforeEach(func() {
		ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(60*time.Second))
		mockServices = serviceregistry.NewMockRegistry()
		// Ensure a clean slate: no leaked env from other specs.
		_ = os.Unsetenv("NMAP_BACKEND")
	})

	AfterEach(func() {
		cancel()

		_ = os.Unsetenv("NMAP_BACKEND")

		fsmv2client.SetClient(nil)
	})

	Describe("backend selection", func() {
		It("selects the fsmv2 backend when NMAP_BACKEND=fsmv2", func() {
			_ = os.Setenv("NMAP_BACKEND", constants.NmapBackendFSMv2)

			defer func() { _ = os.Unsetenv("NMAP_BACKEND") }()

			svc := NewDefaultConnectionService("flag-on-conn")

			Expect(svc.UsesFsmv2Backend()).To(BeTrue(),
				"NMAP_BACKEND=fsmv2 must select the fsmv2-backed nmap manager")
		})

		It("keeps the fsmv1 backend when NMAP_BACKEND is unset (FF-off default)", func() {
			// env intentionally unset in BeforeEach
			svc := NewDefaultConnectionService("flag-off-conn")

			Expect(svc.UsesFsmv2Backend()).To(BeFalse(),
				"unset NMAP_BACKEND must keep the existing S6/fsmv1 nmap path")
		})

		It("keeps the fsmv1 backend for any non-fsmv2 value", func() {
			_ = os.Setenv("NMAP_BACKEND", "fsmv1")

			defer func() { _ = os.Unsetenv("NMAP_BACKEND") }()

			svc := NewDefaultConnectionService("flag-explicit-off-conn")

			Expect(svc.UsesFsmv2Backend()).To(BeFalse())
		})
	})

	Describe("ServiceExists is fsmv2-aware when the flag is on", func() {
		It("returns false for an unknown connection without probing S6 (no nil-nmapService panic)", func() {
			_ = os.Setenv("NMAP_BACKEND", constants.NmapBackendFSMv2)

			defer func() { _ = os.Unsetenv("NMAP_BACKEND") }()

			svc := NewDefaultConnectionService("flag-on-noinstance")
			Expect(svc.UsesFsmv2Backend()).To(BeTrue())

			// With the fsmv2 backend the S6 nmapService is left nil. ServiceExists
			// must consult the fsmv2 manager (GetInstance) and NOT touch S6, so
			// this must return false rather than panic on a nil nmapService.
			var exists bool

			Expect(func() {
				exists = svc.ServiceExists(ctx, mockServices.GetFileSystem(), "no-such-connection")
			}).NotTo(Panic())
			Expect(exists).To(BeFalse())
		})

		It("returns true once the fsmv2 manager holds the instance", func() {
			_ = os.Setenv("NMAP_BACKEND", constants.NmapBackendFSMv2)

			defer func() { _ = os.Unsetenv("NMAP_BACKEND") }()

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
			Expect(svc.UsesFsmv2Backend()).To(BeTrue())

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

	Describe("ForceRemoveConnection drops the desired config when the flag is on", func() {
		It("removes the config from the reconcile set so the worker despawns", func() {
			_ = os.Setenv("NMAP_BACKEND", constants.NmapBackendFSMv2)

			defer func() { _ = os.Unsetenv("NMAP_BACKEND") }()

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
