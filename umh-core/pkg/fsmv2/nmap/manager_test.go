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

package fsmv2nmap

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
)

// -----------------------------------------------------------------------------
// ASSUMED API (drives GREEN — implement EXACTLY this):
//
//	func NewFsmv2NmapManager(managerName string) *adapter.WorkerManager[config.NmapConfig, simple.Status[NmapStatus]]
//
// Its internal WorkerManagerSpec must set:
//   - WorkerType     = WorkerType ("nmap")
//   - ExtractConfigs = snapshot.CurrentConfig.Internal.Nmap   (config.go:42 Internal, :85 Nmap)
//   - NameOf         = cfg.Name (promoted from FSMInstanceConfig)
//   - IsEnabled      = cfg.DesiredFSMState != "stopped"
//   - MapFresh(cfg, s simple.Status[NmapStatus]) string
//       maps s.Result.PortState -> the fsmv1 operational-state string
//       (Fresh+healthy leaf only; degraded/stale/bootstrap are framework-owned).
//   - MapObserved(cfg, s simple.Status[NmapStatus]) publicfsm.ObservedState
//       builds a nmapfsm.NmapObservedState from s.Result (ObservedNmapServiceConfig
//       from cfg; ServiceInfo.NmapStatus.LastScan.PortResult from the status).
//
// The stored status type is simple.Status[NmapStatus] (the verdict wrapper), so
// the mappers read s.Result and the adapter reads the degraded verdict off the
// wrapper via its HealthReporter interface.
// -----------------------------------------------------------------------------

// stubManagerReader is a deps.StateReader that returns a fixed
// Observation[simple.Status[NmapStatus]] for every ref, or an error. It mirrors
// the stubReader harness in adapter/instance_test.go but for the nmap stored
// status type.
type stubManagerReader struct {
	obs *fsmv2.Observation[simple.Status[NmapStatus]]
	err error
}

func (s *stubManagerReader) LoadObservedTyped(_ context.Context, _, _ string, result any) error {
	if s.err != nil {
		return s.err
	}

	if s.obs == nil {
		return nil
	}

	out, ok := result.(*fsmv2.Observation[simple.Status[NmapStatus]])
	if !ok {
		return nil
	}

	*out = *s.obs

	return nil
}

var _ = Describe("NewFsmv2NmapManager", func() {
	const target = "192.0.2.1"

	const port uint16 = 502

	const name = "probe-1"

	// ref is the ref the manager derives internally: {WorkerType:"nmap", Name}.
	ref := dynamicchildren.Ref{WorkerType: WorkerType, Name: name}

	// nmapConfig builds an enabled config.NmapConfig named `name`.
	nmapConfig := func(desired string) config.NmapConfig {
		return config.NmapConfig{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            name,
				DesiredFSMState: desired,
			},
			NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
				Target: target,
				Port:   port,
			},
		}
	}

	// snapshotWith puts the given configs at the real snapshot path the manager
	// reads: CurrentConfig.Internal.Nmap.
	snapshotWith := func(cfgs ...config.NmapConfig) publicfsm.SystemSnapshot {
		return publicfsm.SystemSnapshot{
			CurrentConfig: config.FullConfig{
				Internal: config.InternalConfig{Nmap: cfgs},
			},
		}
	}

	// freshStatus wraps an NmapStatus in a Fresh, non-degraded observation.
	freshStatus := func(st NmapStatus) *fsmv2.Observation[simple.Status[NmapStatus]] {
		return &fsmv2.Observation[simple.Status[NmapStatus]]{
			CollectedAt: time.Now().Add(-100 * time.Millisecond),
			Status:      simple.Status[NmapStatus]{Result: st},
		}
	}

	// stageClient publishes a global client whose store returns obs/err for the
	// managed ref. The manager Upserts the ref itself during Reconcile.
	stageClient := func(obs *fsmv2.Observation[simple.Status[NmapStatus]], err error) *dynamicchildren.Writer {
		writer := dynamicchildren.NewWriter()
		fsmv2client.SetClient(fsmv2client.NewFSMv2Client(writer, &stubManagerReader{obs: obs, err: err}))

		return writer
	}

	AfterEach(func() {
		fsmv2client.SetClient(nil)
	})

	It("maps a Fresh open observation to the fsmv1 open operational state", func() {
		stageClient(freshStatus(NmapStatus{PortState: "open", IsRunning: true, Port: port, LatencyMs: 3}), nil)

		mgr := NewFsmv2NmapManager("test")

		err, _ := mgr.Reconcile(context.Background(), snapshotWith(nmapConfig("running")), nil)
		Expect(err).NotTo(HaveOccurred())

		state, err := mgr.GetCurrentFSMState(name)
		Expect(err).NotTo(HaveOccurred())
		Expect(state).To(Equal(nmapfsm.OperationalStateOpen))
	})

	It("returns the degraded operational state for a degraded verdict", func() {
		obs := &fsmv2.Observation[simple.Status[NmapStatus]]{
			CollectedAt: time.Now().Add(-100 * time.Millisecond),
			Status:      simple.Status[NmapStatus]{Degraded: true, Reason: "poll error: x"},
		}
		stageClient(obs, nil)

		mgr := NewFsmv2NmapManager("test")

		err, _ := mgr.Reconcile(context.Background(), snapshotWith(nmapConfig("running")), nil)
		Expect(err).NotTo(HaveOccurred())

		state, err := mgr.GetCurrentFSMState(name)
		Expect(err).NotTo(HaveOccurred())
		Expect(state).To(Equal(nmapfsm.OperationalStateDegraded))
	})

	It("builds a NmapObservedState reflecting the status and config", func() {
		stageClient(freshStatus(NmapStatus{PortState: "open", IsRunning: true, Port: port, LatencyMs: 7}), nil)

		mgr := NewFsmv2NmapManager("test")

		err, _ := mgr.Reconcile(context.Background(), snapshotWith(nmapConfig("running")), nil)
		Expect(err).NotTo(HaveOccurred())

		observed, err := mgr.GetLastObservedState(name)
		Expect(err).NotTo(HaveOccurred())

		nmapObserved, ok := observed.(nmapfsm.NmapObservedState)
		Expect(ok).To(BeTrue(), "expected a nmapfsm.NmapObservedState")

		Expect(nmapObserved.ObservedNmapServiceConfig.Target).To(Equal(target))
		Expect(nmapObserved.ObservedNmapServiceConfig.Port).To(Equal(port))

		Expect(nmapObserved.ServiceInfo.NmapStatus.IsRunning).To(BeTrue())
		Expect(nmapObserved.ServiceInfo.NmapStatus.LastScan).NotTo(BeNil())
		Expect(nmapObserved.ServiceInfo.NmapStatus.LastScan.PortResult.State).To(Equal("open"))
		Expect(nmapObserved.ServiceInfo.NmapStatus.LastScan.PortResult.Port).To(Equal(port))
	})

	It("extracts configs from the snapshot and Upserts the enabled worker's ref", func() {
		writer := stageClient(freshStatus(NmapStatus{PortState: "open", IsRunning: true, Port: port}), nil)

		mgr := NewFsmv2NmapManager("test")

		err, changed := mgr.Reconcile(context.Background(), snapshotWith(nmapConfig("running")), nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())

		_, ok := mgr.GetInstance(name)
		Expect(ok).To(BeTrue(), "instance should appear under its name")

		Expect(writer.Registry().Contains(ref)).To(BeTrue(), "enabled worker ref should be Upserted")
	})

	It("keeps a stopped worker in the instance map but does not Upsert its ref", func() {
		writer := stageClient(freshStatus(NmapStatus{PortState: "open"}), nil)

		mgr := NewFsmv2NmapManager("test")

		err, _ := mgr.Reconcile(context.Background(), snapshotWith(nmapConfig("stopped")), nil)
		Expect(err).NotTo(HaveOccurred())

		_, ok := mgr.GetInstance(name)
		Expect(ok).To(BeTrue(), "disabled worker stays resident in the instance map")

		Expect(writer.Registry().Contains(ref)).To(BeFalse(), "disabled worker ref must not be Upserted")
	})

	It("cfgFor round-trips target/port through the child-spec YAML pipeline", func() {
		// The Upsert payload is YAML-marshalled into UserSpec.Config and the worker
		// YAML-unmarshals it back into an NmapConfig. cfgFor must therefore emit
		// yaml-tag keys; the adapter's default JSON CfgFor would drop target/port
		// (NmapConfig carries no json tags).
		payload, err := cfgFor(nmapConfig("running"))
		Expect(err).NotTo(HaveOccurred())

		data, err := yaml.Marshal(payload)
		Expect(err).NotTo(HaveOccurred())

		var back config.NmapConfig
		Expect(yaml.Unmarshal(data, &back)).To(Succeed())
		Expect(back.NmapServiceConfig.Target).To(Equal(target))
		Expect(back.NmapServiceConfig.Port).To(Equal(port))
		Expect(back.Name).To(Equal(name))
	})
})
