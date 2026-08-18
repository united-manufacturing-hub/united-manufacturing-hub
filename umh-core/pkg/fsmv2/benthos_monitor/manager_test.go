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

package fsmv2benthosmonitor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosmonitorfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2benthosmonitor "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"

	"gopkg.in/yaml.v3"
)

// stubManagerReader is a deps.StateReader that returns a fixed
// Observation[simple.Status[BenthosMonitorStatus]] for every ref, mirroring the
// stubReader harness (adapter/instance_test.go) for the benthos status type.
type stubManagerReader struct {
	obs *fsmv2.Observation[simple.Status[fsmv2benthosmonitor.BenthosMonitorStatus]]
	err error
}

func (s *stubManagerReader) LoadObservedTyped(_ context.Context, _, _ string, result any) error {
	if s.err != nil {
		return s.err
	}

	if s.obs == nil {
		return nil
	}

	out, ok := result.(*fsmv2.Observation[simple.Status[fsmv2benthosmonitor.BenthosMonitorStatus]])
	if !ok {
		return nil
	}

	*out = *s.obs

	return nil
}

var _ = Describe("benthos_monitor registration and adapter vocabulary", func() {
	const name = "benthos-1"

	// benthosConfig builds an enabled config.BenthosMonitorConfig named `name`.
	benthosConfig := func(desired string) config.BenthosMonitorConfig {
		return config.BenthosMonitorConfig{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            name,
				DesiredFSMState: desired,
			},
			MetricsPort: 4195,
		}
	}

	// snapshotWith puts the given configs at the real snapshot path the manager's
	// ExtractConfigs reads.
	snapshotWith := func(cfgs ...config.BenthosMonitorConfig) publicfsm.SystemSnapshot {
		return publicfsm.SystemSnapshot{
			CurrentConfig: config.FullConfig{
				Internal: config.InternalConfig{BenthosMonitor: cfgs},
			},
		}
	}

	// degradedFresh stamps CollectedAt inside fsmv2client.GetFresh's maxAge window,
	// so the observation reads Fresh, and carries a degraded verdict.
	degradedFresh := func() *fsmv2.Observation[simple.Status[fsmv2benthosmonitor.BenthosMonitorStatus]] {
		return &fsmv2.Observation[simple.Status[fsmv2benthosmonitor.BenthosMonitorStatus]]{
			CollectedAt: time.Now().Add(-100 * time.Millisecond),
			Status: simple.Status[fsmv2benthosmonitor.BenthosMonitorStatus]{
				Result:   fsmv2benthosmonitor.BenthosMonitorStatus{},
				Degraded: true,
				Reason:   "poll error: x",
			},
		}
	}

	// freshHealthy is degradedFresh with no degraded verdict, so mapFresh maps it to
	// benthos_monitor's own "active" word.
	freshHealthy := func() *fsmv2.Observation[simple.Status[fsmv2benthosmonitor.BenthosMonitorStatus]] {
		return &fsmv2.Observation[simple.Status[fsmv2benthosmonitor.BenthosMonitorStatus]]{
			CollectedAt: time.Now().Add(-100 * time.Millisecond),
			Status: simple.Status[fsmv2benthosmonitor.BenthosMonitorStatus]{
				Result: fsmv2benthosmonitor.BenthosMonitorStatus{},
			},
		}
	}

	AfterEach(func() {
		fsmv2client.SetClient(nil)
	})

	It("registers the worker and drives benthos_monitor's own state words through the adapter", func() {
		// The worker registers itself on import: a "benthos_monitor" worker type
		// exists with a positive observation interval, and WorkerType is exactly
		// the canonical string.
		Expect(fsmv2.LookupInitialState(fsmv2benthosmonitor.WorkerType)).NotTo(BeNil())
		interval, ok := fsmv2.ObservationIntervalFor(fsmv2benthosmonitor.WorkerType)
		Expect(ok).To(BeTrue())
		Expect(interval).To(BeNumerically(">", 0))
		Expect(fsmv2benthosmonitor.WorkerType).To(Equal("benthos_monitor"))

		// One registered client, held across the reads; only the reader's
		// observation/error changes. The ref stays registered, so the first read
		// takes the NeverObserved bootstrap exit rather than Unregistered. The two
		// later reads are classified by freshness. fsmv2client.Freshness
		// (fsmv2client/fsmv2client.go) defines both words: Unregistered means the
		// ref was never Upserted, NeverObserved means Upserted with nothing stored
		// yet.
		reader := &stubManagerReader{err: persistence.ErrNotFound}
		writer := dynamicchildren.NewWriter()
		fsmv2client.SetClient(fsmv2client.NewFSMv2Client(writer, reader))

		mgr := fsmv2benthosmonitor.NewFsmv2BenthosMonitorManager("test")
		err, _ := mgr.Reconcile(context.Background(), snapshotWith(benthosConfig("active")), nil)
		Expect(err).NotTo(HaveOccurred())

		// BOOTSTRAP: no observation yet (NeverObserved) => the Starting word this
		// worker declared, benthosmonitorfsm.OperationalStateStarting
		// ("benthos_monitoring_starting"). The adapter supplies no fallback word of
		// its own: adapter.StateVocabulary (adapter/manager.go) requires a
		// worker to declare all four.
		state, gerr := mgr.GetCurrentFSMState(name)
		Expect(gerr).NotTo(HaveOccurred())
		Expect(state).To(Equal(benthosmonitorfsm.OperationalStateStarting))

		// DEGRADED verdict on a Fresh observation => the Degraded word this worker
		// declared, benthosmonitorfsm.OperationalStateDegraded ("degraded"). The four
		// declared words must be pairwise distinct, so this word cannot double as any
		// other exit: adapter.StateVocabulary says why (adapter/manager.go) and
		// adapter.NewWorkerManager panics otherwise (same file).
		reader.err = nil
		reader.obs = degradedFresh()

		state, gerr = mgr.GetCurrentFSMState(name)
		Expect(gerr).NotTo(HaveOccurred())
		Expect(state).To(Equal(benthosmonitorfsm.OperationalStateDegraded))

		// FRESH + healthy verdict => the mapFresh leaf, which returns
		// benthosmonitorfsm.OperationalStateActive ("active"), the same word this
		// worker declares as adapter.StateVocabulary.DesiredRunning.
		reader.obs = freshHealthy()

		state, gerr = mgr.GetCurrentFSMState(name)
		Expect(gerr).NotTo(HaveOccurred())
		Expect(state).To(Equal(benthosmonitorfsm.OperationalStateActive))

		// EMPTY desired state on a config => the declared DesiredRunning word,
		// benthosmonitorfsm.OperationalStateActive ("active"). Why it has to be that
		// word rather than a generic "running" is on the States field in
		// NewFsmv2BenthosMonitorManager (manager.go).
		mgr2 := fsmv2benthosmonitor.NewFsmv2BenthosMonitorManager("test")
		err, _ = mgr2.Reconcile(context.Background(), snapshotWith(benthosConfig("")), nil)
		Expect(err).NotTo(HaveOccurred())

		inst, iok := mgr2.GetInstance(name)
		Expect(iok).To(BeTrue())
		Expect(inst.GetDesiredFSMState()).To(Equal(benthosmonitorfsm.OperationalStateActive))
	})

	It("round-trips Name, DesiredFSMState, and MetricsPort through the child-spec YAML pipeline", func() {
		// cfgFor's godoc (manager.go) explains why this round-trip needs
		// yaml-tag keys.
		reader := &stubManagerReader{err: persistence.ErrNotFound}
		writer := dynamicchildren.NewWriter()
		fsmv2client.SetClient(fsmv2client.NewFSMv2Client(writer, reader))

		mgr := fsmv2benthosmonitor.NewFsmv2BenthosMonitorManager("test")
		err, _ := mgr.Reconcile(context.Background(), snapshotWith(benthosConfig("active")), nil)
		Expect(err).NotTo(HaveOccurred())

		spec, ok := writer.Registry().Lookup(dynamicchildren.Ref{
			WorkerType: fsmv2benthosmonitor.WorkerType,
			Name:       name,
		})
		Expect(ok).To(BeTrue(), "enabled worker ref should be Upserted")

		var back config.BenthosMonitorConfig
		Expect(yaml.Unmarshal([]byte(spec.UserSpec.Config), &back)).To(Succeed())
		Expect(back.Name).To(Equal(name))
		Expect(back.DesiredFSMState).To(Equal("active"))
		Expect(back.MetricsPort).To(Equal(uint16(4195)))
	})
})
