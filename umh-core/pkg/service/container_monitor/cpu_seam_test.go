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

package container_monitor_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// workerVerdictMessage is cpuhealth.ComposeMessage's output for a degraded
// throttling verdict — the value the CPU worker stages into CPUStatus.Message
// on a degraded tick. It is sourced from ComposeMessage, never invented: a real
// message-format regression moves the staged value and the assertion together,
// so a drift cannot sail the suite.
var workerVerdictMessage = cpuhealth.ComposeMessage(
	cpuhealth.Verdict{
		State:  cpuhealth.StateDegraded,
		Causes: []cpuhealth.Cause{{Kind: cpuhealth.CauseKindThrottling, Value: 0.5}},
	},
	cpuhealth.Details{ThrottleRatio: 0.5},
)

// workerHealthyDetails is one healthy tick's Details on the no-limit budget
// dashboard. Specs stage it as the observation's own Details and compose
// workerHealthyMessage from it, so the sentence and the numbers beside it can
// never drift apart. The usage members are set because a box healthy enough to
// be judged has measured its usage; without them the wire would carry no usage
// at all.
var workerHealthyDetails = cpuhealth.Details{
	CapacityCores:          8,
	LimitApplies:           false,
	HostBusyRingActive:     true,
	HostBusyCoresAvailable: true,
	AvgHostBusyCores:       2,
	ReserveCores:           1,
	AvgUsageCores:          1.5,
	UsageRingActive:        true,
	LogicalCpus:            8,
}

// workerHealthyMessage is cpuhealth.ComposeMessage's output for a healthy
// verdict, sourced from the same Details.
var workerHealthyMessage = cpuhealth.ComposeMessage(
	cpuhealth.Verdict{State: cpuhealth.StateHealthy},
	workerHealthyDetails,
)

// supervisorNotRunningMessage mirrors the diagnostic warning readWorkerCPUHealth
// emits when no fsmv2 client is published. The warn-once spec asserts message
// content, not just a count.
const supervisorNotRunningMessage = "The CPU health check cannot run because the fsmv2 supervisor is not running, so CPU is not measured"

// useFSMv2CPUEnv is the environment variable that used to switch CPU status
// back to the 70% CPU usage rule. Nothing reads it any more; the
// [use-fsmv2-cpu-false] spec below pins that a customer who still sets it
// keeps the CPU health check.
const useFSMv2CPUEnv = "USE_FSMV2_CPU"

// cpuStubStateReader is the cpuStubReader harness pattern from
// fsmv2/fsmv2client/freshness_test.go: a deps.StateReader that serves a
// staged fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]] verbatim, letting
// the test drive a REAL fsmv2 client's read side without a storage engine. The
// framework wraps the worker's CPUStatus in simple.Status — the developer
// result plus the Degraded/Reason verdict — and the seam reads that wrapper, so
// the stub stages the wrapper, never the bare result.
type cpuStubStateReader struct {
	obs *fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]
	err error
}

func (s *cpuStubStateReader) LoadObservedTyped(_ context.Context, _, _ string, result interface{}) error {
	if s.err != nil {
		return s.err
	}

	if s.obs == nil {
		return errors.New("cpuStubStateReader: no staged observation")
	}

	out, ok := result.(*fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]])
	if !ok {
		return errors.New("cpuStubStateReader: result is not *fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]")
	}

	*out = *s.obs

	return nil
}

// LoadDesiredTyped reports that no desired state is saved.
func (*cpuStubStateReader) LoadDesiredTyped(_ context.Context, _, _ string, _ any) error {
	return persistence.ErrNotFound
}

var _ = Describe("the CPU seam", func() {
	var (
		service      *container_monitor.ContainerMonitorService
		mockFS       *filesystem.MockFileSystem
		ctx          context.Context
		testDataPath string
	)

	BeforeEach(func() {
		mockFS = filesystem.NewMockFileSystem()
		ctx = context.Background()

		var err error
		testDataPath, err = os.MkdirTemp("", "container-monitor-cpu-seam")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		err := os.RemoveAll(testDataPath)
		Expect(err).NotTo(HaveOccurred())
	})

	// publishWorkerClientWithStub registers the single CPU monitor ref in a real
	// writer and publishes a REAL fsmv2 client (NewFSMv2Client + SetClient) whose
	// read side serves the staged stub verbatim — an observation, an
	// ErrNotFound, or a generic read error. DeferCleanup restores the process
	// globals, so a leak can never bleed into another spec.
	publishWorkerClientWithStub := func(stub *cpuStubStateReader) *fsmv2client.FSMv2Client {
		writer := dynamicchildren.NewWriter()
		Expect(writer.Upsert(fsmv2cpu.Ref, map[string]any{})).To(Succeed())

		client := fsmv2client.NewFSMv2Client(writer, stub)
		previous := fsmv2client.GetClient()
		fsmv2client.SetClient(client)
		DeferCleanup(func() { fsmv2client.SetClient(previous) })

		return client
	}

	publishWorkerClient := func(obs *fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]) *fsmv2client.FSMv2Client {
		return publishWorkerClientWithStub(&cpuStubStateReader{obs: obs})
	}

	// observeWarns installs a warn-level observer on the component logger and
	// returns the log sink, following the warn-once harness: logger.GetLogger()
	// first so the constructor's logger.For() cannot re-initialize over it.
	observeWarns := func() *observer.ObservedLogs {
		logger.GetLogger()
		core, logs := observer.New(zapcore.WarnLevel)
		restoreGlobals := zap.ReplaceGlobals(zap.New(core))
		DeferCleanup(restoreGlobals)

		return logs
	}

	Context("[worker]", func() {
		It("should degrade CPU health, carrying the worker's reason as the message, from a Fresh observation whose framework Degraded flag is set (a Poll error arrives with a nil error and a zero result verdict)", func() {
			const pollErrReason = "poll error: cgroup cpu.stat read failed"
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond), // Fresh: well inside the seam's 3s maxAge
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result:   fsmv2cpu.CPUStatus{}, // zero result verdict: the failed Poll preserved no verdict
					Degraded: true,
					Reason:   pollErrReason,
				},
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// The worker's "cannot measure" declaration must degrade the instance...
			Expect(status.CPUHealth).To(Equal(models.Degraded))
			// ...and, because OverallHealth reads CPU, it must degrade too. This
			// asserts that explicitly: memory and disk can only ADD Degraded,
			// never remove it.
			Expect(status.OverallHealth).To(Equal(models.Degraded))
			// ...and the framework reason must land where the protocol-converter
			// resource-limit check (BridgeMustWait) reads the block message.
			Expect(status.CPU.Health.Message).To(Equal(pollErrReason))
			Expect(status.CPU.Health.Category).To(Equal(models.Degraded))
			Expect(status.CPU.Health.ObservedState).To(Equal("degraded"))
			Expect(status.CPU.Health.DesiredState).To(Equal("active"))
		})

		It("should fill status.CPUHealth and status.CPU.Health from the worker's Fresh degraded verdict", func() {
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond), // Fresh: well inside the seam's 3s maxAge
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: cpuhealth.Verdict{State: cpuhealth.StateDegraded},
						Message: workerVerdictMessage,
					},
				},
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// The worker's verdict drives the service-level CPU health...
			Expect(status.CPUHealth).To(Equal(models.Degraded))
			// ...and, because OverallHealth reads CPU, it drives the overall
			// health too (memory and disk can only add to a degraded CPU
			// verdict, never pull it back to Active).
			Expect(status.OverallHealth).To(Equal(models.Degraded))
			// ...the worker's message lands where the protocol-converter
			// resource-limit check (BridgeMustWait) reads it...
			Expect(status.CPU.Health.Message).To(Equal(workerVerdictMessage))
			// ...the nested category follows the verdict...
			Expect(status.CPU.Health.Category).To(Equal(models.Degraded))
			// ...and the state vocabulary matches the other health records in
			// this package ("active"/"degraded"), not the worker's raw verdict.
			Expect(status.CPU.Health.ObservedState).To(Equal("degraded"))
			Expect(status.CPU.Health.DesiredState).To(Equal("active"))

			// A second tick still comes from the worker.
			status, err = service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(status.CPU.Health.Message).To(Equal(workerVerdictMessage))
		})

		It("should keep the real measured numbers on a Fresh degraded worker verdict (a busy box the worker assessed degraded carries its genuine usage, not a nil or 0 absent marker)", func() {
			// A measured degraded verdict as the worker really stores it: the
			// framework Degraded flag rides along (healthFromStatus degrades
			// the wrapper on every good degraded poll, and worker.go copies
			// that verdict into Status.Degraded), with the same composed
			// message in Reason and Result.
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond), // Fresh: inside the seam's 3s maxAge
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: cpuhealth.Verdict{
							State:  cpuhealth.StateDegraded,
							Causes: []cpuhealth.Cause{{Kind: cpuhealth.CauseKindThrottling, Value: 0.5}},
						},
						Details: cpuhealth.Details{
							AvgUsageCores:   3.5,
							UsageRingActive: true,
							LogicalCpus:     6,
							CapacityCores:   4,
							LimitApplies:    true,
							ThrottleRatio:   0.5,
							// A verdict cannot blame throttling unless the
							// throttle signal was readable, so a fixture that
							// omits this stages a state the worker never emits.
							ThrottleSignalReady: true,
						},
						Message: workerVerdictMessage,
					},
					Degraded: true,
					Reason:   workerVerdictMessage,
				},
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// The measured degraded verdict degrades the CPU health and carries
			// the worker's message...
			Expect(status.CPUHealth).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Category).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Message).To(Equal(workerVerdictMessage))
			// The numbers live in the evidence.
			Expect(status.CPU.CPUHealth).NotTo(BeNil())
			Expect(status.CPU.CPUHealth.AvgUsageCores).To(Equal(3.5))
			Expect(status.CPU.CPUHealth.LogicalCpus).To(Equal(6.0))
			Expect(status.CPU.CPUHealth.CapacityCores).To(Equal(4.0))
			Expect(status.CPU.CPUHealth.ThrottleRatio).To(Equal(0.5))
		})

		It("should fill status.CPU.Health from a Fresh healthy worker verdict with the Active category", func() {
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond),
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: cpuhealth.Verdict{State: cpuhealth.StateHealthy},
						Details: workerHealthyDetails,
						Message: workerHealthyMessage,
					},
				},
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// A real healthy verdict fills the nested health record.
			Expect(status.CPU.Health.Message).To(Equal(workerHealthyMessage))
			Expect(status.CPU.Health.Category).To(Equal(models.Active))
			Expect(status.CPU.Health.ObservedState).To(Equal("active"))
			Expect(status.CPU.Health.DesiredState).To(Equal("active"))
			// A healthy verdict carries its numbers in the evidence.
			Expect(status.CPU.CPUHealth.AvgUsageCores).To(Equal(1.5))
			Expect(status.CPU.CPUHealth.LogicalCpus).To(Equal(8.0))
		})

		It("should degrade with the worker's no-verdict reason when its Fresh observation carries no determination", func() {
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond),
				// Genuine "no determination": an empty result verdict and no
				// framework degraded declaration. A poll error is NOT this shape —
				// the worker sets Degraded with a reason for that. This is a
				// successful poll that produced no verdict, and an empty result
				// must not be read as healthy.
				Status: simple.Status[fsmv2cpu.CPUStatus]{Result: fsmv2cpu.CPUStatus{}},
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// A successful poll that produced no verdict is an absence, not a
			// healthy box, and with no second opinion left it degrades and says
			// which absence it was.
			Expect(status.CPUHealth).To(Equal(models.Degraded))
			Expect(status.OverallHealth).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Category).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Message).To(Equal("CPU worker produced no verdict for its last observation"))
			Expect(status.CPU.Health.ObservedState).To(Equal("degraded"))
			Expect(status.CPU.Health.DesiredState).To(Equal("active"))
			// Nothing measured, so no measurement ships.
			Expect(status.CPU.CPUHealth).To(BeNil())
		})
	})

	Context("[unmeasured-ships-no-numbers]", func() {
		It("should ship no usage numbers on a worker tick that could not measure", func() {
			// An unmeasurable tick: the worker stored a Degraded framework
			// verdict with an empty result (zero numeric measurements), because
			// it could not measure. This is the poll-error / unreadable-cgroup
			// shape, which arrives Fresh with a nil error.
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond), // Fresh: inside the seam's 3s maxAge
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result:   fsmv2cpu.CPUStatus{},
					Degraded: true,
					Reason:   "cpu.stat unreadable",
				},
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// The worker could not measure, so the tick reports the worker's
			// reason and no figures. cpuHealth is omitempty, so an unmeasured
			// tick omits it rather than shipping a zero a reader would take for
			// an idle box.
			Expect(status.CPU.Health.Category).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Message).To(Equal("cpu.stat unreadable"))
			Expect(status.CPU.CPUHealth).To(BeNil())

			data, err := json.Marshal(status.CPU)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).NotTo(ContainSubstring("cpuHealth"))
		})
	})

	Context("[cpuhealth-evidence]", func() {
		It("should fill status.CPU.CPUHealth from one worker tick's verdict and details, and omit the key entirely on a tick that measured nothing", func() {
			// The fills case stages what one measured tick stores: a degraded
			// verdict WITH attribution and causes — the console panel's whole
			// purpose, since without causes no row reddens — beside the Details
			// it judged, including a present p95 Reading so the optional-member
			// pass-through is asserted too, not just the always-float members. The
			// framework Degraded flag and Reason ride along too, exactly as
			// worker.go stores a measured degraded tick, so the staging is the
			// reachable pairing rather than one no writer produces.
			stagedVerdict := cpuhealth.Verdict{
				State:       cpuhealth.StateDegraded,
				Attribution: cpuhealth.AttributionHost,
				Causes: []cpuhealth.Cause{{
					Kind:        cpuhealth.CauseKindThrottling,
					Instrument:  "cpu.stat",
					Unit:        "ratio",
					Attribution: cpuhealth.AttributionHost,
					Value:       0.5,
				}},
			}
			stagedDetails := cpuhealth.Details{
				P95UsageCores:          diagnosis.Known(1.25),
				ThrottleRatio:          0.5,
				PressureAvg60:          0.1,
				StealP95:               0.2,
				AvgUsageFraction:       0.4,
				AvgUsageCores:          3.2,
				HostHeadroomCores:      -1.5,
				AvgHostBusyCores:       6.5,
				CapacityCores:          2,
				ReserveCores:           1,
				LogicalCpus:            8,
				HostCpus:               8,
				UsageRingActive:        true,
				HostBusyRingActive:     true,
				HostBusyCoresAvailable: true,
				LimitApplies:           true,
				PressureApplies:        true,
				HostHeadroomAvailable:  false,
				ThrottleSignalReady:    true,
				PressureSignalReady:    true,
				StealSignalReady:       false,
			}

			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond), // Fresh: inside the seam's 3s maxAge
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: stagedVerdict,
						Message: workerVerdictMessage,
						Details: stagedDetails,
					},
					Degraded: true,
					Reason:   workerVerdictMessage,
				},
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// The seam fills the wire record from that one tick: the staged
			// observation is the only source of these values, so equality asserts
			// that the verdict (state, attribution, causes) AND every Details
			// member travel together — not a re-derivation and not a subset.
			Expect(status.CPU.CPUHealth).NotTo(BeNil())
			Expect(*status.CPU.CPUHealth).To(Equal(models.CPUHealth{
				Verdict: stagedVerdict,
				Details: stagedDetails,
			}))

			// Wire contract: the measured tick ships the cpuHealth key.
			data, err := json.Marshal(status.CPU)
			Expect(err).NotTo(HaveOccurred())

			var raw map[string]interface{}
			Expect(json.Unmarshal(data, &raw)).To(Succeed())
			Expect(raw).To(HaveKey("cpuHealth"))

			// The omits case: a tick that measured nothing. The framework-
			// Degraded observation (a poll error arrives Fresh with a zero
			// result) fails closed to a Degraded models.Health on CPU.Health,
			// but there is no Details behind such a verdict — so cpuHealth
			// must be absent, never a fabricated empty or zero-filled one.
			// Absence of cpuHealth never means health: it means nothing was
			// measured, and Health keeps carrying the judgement either way.
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond), // Fresh: inside the seam's 3s maxAge
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result:   fsmv2cpu.CPUStatus{}, // zero result: the failed Poll preserved no verdict and no details
					Degraded: true,
					Reason:   "poll error: cgroup cpu.stat read failed",
				},
			})

			status, err = service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(status.CPU.CPUHealth).To(BeNil())

			dataNil, err := json.Marshal(status.CPU)
			Expect(err).NotTo(HaveOccurred())

			var rawNil map[string]interface{}
			Expect(json.Unmarshal(dataNil, &rawNil)).To(Succeed())
			Expect(rawNil).NotTo(HaveKey("cpuHealth"))
		})
	})

	Context("[cpuhealth-document]", func() {
		It("should publish one real tick end to end as a cpuHealth object holding verdict.state and every Details member inline", func() {
			// The tick is real, not staged output: a genuine engine and a
			// staged sample sequence, the harness pkg/cpuhealth's own specs
			// drive (decide_contract_test.go's fired-throttling scenario).
			// Sixty-six one-second samples fill every 60s window past its
			// floor and hold the throttle ratio at 0.06, so the final Decide
			// returns a degraded verdict with a throttling cause beside a
			// Details whose every member carries a measured value.
			engine, err := cpuhealth.NewEngine(4, 2.0)
			Expect(err).NotTo(HaveOccurred())
			env := diagnosis.NewEnvironment(cpuhealth.HasVirtualization, cpuhealth.HasLimit, cpuhealth.HasPressureStats)
			base := time.Now()

			var verdict cpuhealth.Verdict
			var details cpuhealth.Details
			for i := 0; i <= 65; i++ {
				verdict, details = cpuhealth.Decide(engine, cpuhealth.Sample{
					Timestamp:    base.Add(time.Duration(i) * time.Second),
					CpuScope:     cpuhealth.ScopeHost,
					Virtualized:  true,
					Pressure:     diagnosis.Known(0.1),
					PsiAvailable: true,
					Steal:        diagnosis.Known(0),
					HostBusy:     diagnosis.Known(0.5),
					UsageCores:   diagnosis.Known(0.2),
					NrPeriods:    diagnosis.Known(100 * float64(i)),
					NrThrottled:  diagnosis.Known(6 * float64(i)), // steady 0.06, above the 0.05 fire mark
					Quota:        diagnosis.Known(2.0),
					LogicalCpus:  diagnosis.Known(4),
					HostCpus:     diagnosis.Known(8),
				}, env)
			}

			// The sequence must have earned the degraded verdict the document
			// below asserts: one that failed to fire would leave this spec
			// asserting a healthy tick's layout and pass vacuously.
			Expect(verdict.State).To(Equal(cpuhealth.StateDegraded))
			Expect(verdict.Causes).NotTo(BeEmpty())
			_, p95Present := details.P95UsageCores.Get()
			Expect(p95Present).To(BeTrue(), "a full window's p95 must be present, so the document carries a number rather than a null")

			// Stage the observation exactly as the worker stores a measured
			// degraded tick: healthFromStatus wraps the poll's CPUStatus in
			// simple.Degraded with the composed message as the reason.
			message := cpuhealth.ComposeMessage(verdict, details)
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond), // Fresh: inside the seam's 3s maxAge
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: verdict,
						Message: message,
						Details: details,
					},
					Degraded: true,
					Reason:   message,
				},
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// The verdict and the Details arrive together, from this one tick.
			Expect(status.CPU.CPUHealth).NotTo(BeNil())
			Expect(*status.CPU.CPUHealth).To(Equal(models.CPUHealth{Verdict: verdict, Details: details}))

			// The document the console's adapter parses: the whole status.CPU
			// marshalled, with the key layout asserted here inline because no
			// committed fixture exists to compare against — a key renamed,
			// dropped or nested reads to the console's schema as
			// absent-and-invalid, which nothing downstream distinguishes from a
			// legitimately-absent optional.
			data, err := json.Marshal(status.CPU)
			Expect(err).NotTo(HaveOccurred())

			var raw map[string]interface{}
			Expect(json.Unmarshal(data, &raw)).To(Succeed())
			Expect(raw).To(HaveKey("cpuHealth"))

			health, ok := raw["cpuHealth"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "cpuHealth must be a JSON object")

			// verdict sits BESIDE the Details members and no "details" object
			// wraps them: the embedded shape the console's discriminated union
			// parses, not the nested one pkg/fsmv2/cpu.CPUStatus deliberately
			// uses.
			Expect(health).To(HaveKey("verdict"))
			Expect(health).NotTo(HaveKey("details"))

			detailKeys := []string{
				"p95UsageCores", "throttleRatio", "pressureAvg60", "stealP95",
				"avgUsageFraction", "avgUsageCores", "hostHeadroomCores",
				"avgHostBusyCores", "capacityCores", "reserveCores",
				"logicalCpus", "hostCpus", "usageRingActive",
				"hostBusyRingActive", "limitedVisibility",
				"hostBusyCoresAvailable", "limitApplies", "pressureApplies",
				"stealApplies", "hostHeadroomAvailable", "throttleSignalReady",
				"pressureSignalReady", "stealSignalReady",
			}
			Expect(health).To(HaveLen(len(detailKeys) + 1))
			for _, key := range detailKeys {
				Expect(health).To(HaveKey(key))
			}

			// The verdict object: the degraded state the tick earned, with the
			// attribution and causes the console forbids when healthy and
			// requires when degraded.
			verdictRaw, ok := health["verdict"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(verdictRaw["state"]).To(Equal("degraded"))
			Expect(verdictRaw).To(HaveKey("attribution"))
			causes, ok := verdictRaw["causes"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(causes).NotTo(BeEmpty())
			firstCause, ok := causes[0].(map[string]interface{})
			Expect(ok).To(BeTrue())
			for _, key := range []string{"kind", "instrument", "unit", "attribution", "value"} {
				Expect(firstCause).To(HaveKey(key))
			}
			Expect(firstCause["kind"]).To(Equal("throttling"))

			// The inline keys carry this tick's measured values, not just the
			// names: the numbers Decide produced on the final sample are the
			// numbers on the wire.
			Expect(health["throttleRatio"]).To(Equal(0.06))
			Expect(health["p95UsageCores"]).To(Equal(0.2))
			Expect(health["logicalCpus"]).To(Equal(float64(4)))
			Expect(health["hostCpus"]).To(Equal(float64(8)))
			Expect(health["capacityCores"]).To(Equal(float64(2)))
			Expect(health["usageRingActive"]).To(BeTrue())
			Expect(health["throttleSignalReady"]).To(BeTrue())
		})
	})

	Context("[freshness]", func() {
		It("should degrade CPU health when the worker's observation is stale, even though the verdict it carries is healthy", func() {
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				// Older than the seam's 3s maxAge: GetFresh maps this to
				// Stale even though the verdict it wraps is a healthy one.
				CollectedAt: time.Now().Add(-4 * time.Second),
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: cpuhealth.Verdict{State: cpuhealth.StateHealthy},
						Details: workerHealthyDetails,
						Message: workerHealthyMessage,
					},
				},
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// A stale observation is not a healthy one: the seam must fail
			// closed and drive the service-level CPU health to degraded...
			Expect(status.CPUHealth).To(Equal(models.Degraded))
			// ...and the degrade must say so in words, because the
			// protocol-converter resource-limit check (BridgeMustWait) reads
			// CPU.Health.Message as its block reason — the healthy verdict must
			// also not sail through.
			Expect(status.CPU.Health.Category).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Message).To(ContainSubstring("stale"))
			Expect(status.CPU.Health.Message).NotTo(Equal(workerHealthyMessage))
			Expect(status.CPU.Health.ObservedState).To(Equal("degraded"))
			Expect(status.CPU.Health.DesiredState).To(Equal("active"))
			// A stale observation is not a measurement, so nothing numeric ships
			// with it. The verdict it carried is withheld too.
			Expect(status.CPU.CPUHealth).To(BeNil())
		})

		It("should degrade CPU health when the worker is running but has never observed (the store returns ErrNotFound)", func() {
			publishWorkerClientWithStub(&cpuStubStateReader{err: persistence.ErrNotFound})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// NotFound is an absence of measurement, and an absent
			// measurement is not a healthy one: it must fail closed, and the
			// message must name the never-observed cause (the
			// protocol-converter resource-limit check reads it as the
			// bridge-block reason).
			Expect(status.CPUHealth).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Category).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Message).To(ContainSubstring("never observed"))
			Expect(status.CPU.Health.Message).NotTo(ContainSubstring("could not be read"))
			Expect(status.CPU.Health.ObservedState).To(Equal("degraded"))
			Expect(status.CPU.Health.DesiredState).To(Equal("active"))
		})

		It("should degrade CPU health when the worker's observation cannot be read (a generic store error maps Unknown to degraded, not a fall-through)", func() {
			publishWorkerClientWithStub(&cpuStubStateReader{err: errors.New("cpu store read failed")})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// GetFresh maps the read error to Unknown plus a verbatim error. A
			// switch over Freshness that omits the Unknown branch would leave
			// it Neutral and the box Active, so the assertion is Degraded
			// specifically. The message must also carry the read error
			// verbatim, so this branch cannot collapse into the
			// never-observed branch.
			Expect(status.CPUHealth).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Category).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Message).To(ContainSubstring("could not be read"))
			Expect(status.CPU.Health.Message).To(ContainSubstring("cpu store read failed"))
			Expect(status.CPU.Health.Message).NotTo(ContainSubstring("never observed"))
			Expect(status.CPU.Health.ObservedState).To(Equal("degraded"))
			Expect(status.CPU.Health.DesiredState).To(Equal("active"))
			// An unreadable observation is not a measurement, so no figures ship
			// with the diagnosis.
			Expect(status.CPU.CPUHealth).To(BeNil())
			data, err := json.Marshal(status.CPU)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).NotTo(ContainSubstring("cpuHealth"))
		})

		It("should degrade, naming the removal, when the CPU worker was removed", func() {
			// The staged verdict is Degraded, so the assertions below fail if
			// the seam uses the removed worker's last observation.
			deletedAt := time.Now().Add(-time.Second)
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond),
				DeletedAt:   &deletedAt,
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: cpuhealth.Verdict{State: cpuhealth.StateDegraded},
						Message: workerVerdictMessage,
					},
				},
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(status.CPUHealth).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Category).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Message).To(ContainSubstring("was removed"))
			Expect(status.CPU.Health.Message).NotTo(Equal(workerVerdictMessage))
		})
	})

	Context("[flicker]", func() {
		// flickerRun stages one observation age per GetStatus tick and returns
		// the sequence of CPUHealth categories the run produced. The observation
		// ALWAYS carries the same Fresh healthy verdict: only CollectedAt moves
		// between ticks, so a CPUHealth transition recorded here is a freshness
		// transition at the seam, never a verdict change.
		flickerRun := func(ages ...time.Duration) []models.HealthCategory {
			obs := &fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now(),
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: cpuhealth.Verdict{State: cpuhealth.StateHealthy},
						Details: workerHealthyDetails,
						Message: workerHealthyMessage,
					},
				},
			}
			publishWorkerClient(obs)

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			categories := make([]models.HealthCategory, 0, len(ages))
			for _, age := range ages {
				obs.CollectedAt = time.Now().Add(age)
				status, err := service.GetStatus(ctx)
				Expect(err).NotTo(HaveOccurred())
				categories = append(categories, status.CPUHealth)
			}

			return categories
		}

		// transitions counts the adjacent CPUHealth pairs that differ: the
		// flicker the gate counts. A-then-A and D-then-D are 0; A-then-D and
		// D-then-A are 1.
		transitions := func(categories []models.HealthCategory) int {
			count := 0
			for i := 1; i < len(categories); i++ {
				if categories[i] != categories[i-1] {
					count++
				}
			}

			return count
		}

		It("should not flicker CPUHealth across a single missed poll (a one-interval-old observation stays Fresh inside the seam's 3s maxAge)", func() {
			// The worker polls every 1s and the seam's maxAge is 3x that. A
			// single missed poll leaves the observation one interval (1s) old —
			// still inside maxAge, so GetFresh maps it Fresh and the worker keeps
			// judging. Alternating on-time and one-poll-behind
			// observations must therefore hold CPUHealth Active for the whole
			// run: zero transitions.
			categories := flickerRun(
				-500*time.Millisecond, // poll on time
				-1*time.Second,        // one poll behind: the single missed poll
				-500*time.Millisecond,
				-1*time.Second,
				-500*time.Millisecond,
				-1*time.Second,
			)

			Expect(transitions(categories)).To(Equal(0))
			for _, h := range categories {
				Expect(h).To(Equal(models.Active))
			}
		})

		It("should flip CPUHealth exactly once for an observation gap longer than maxAge, and hold the degraded state while the gap persists", func() {
			// A 4s-old observation is older than the seam's 3s maxAge: GetFresh
			// maps it Stale, the fail-closed verdict degrades CPUHealth, and a
			// second stale tick holds the gap degraded — one transition in, none
			// on the repeat.
			categories := flickerRun(
				-500*time.Millisecond, // Fresh: Active
				-4*time.Second,        // gap > maxAge: Degraded — the one transition
				-4*time.Second,        // gap persists: stays Degraded — no second transition
			)

			Expect(transitions(categories)).To(Equal(1))
			Expect(categories[0]).To(Equal(models.Active))
			Expect(categories[1]).To(Equal(models.Degraded))
			Expect(categories[2]).To(Equal(models.Degraded))
		})

		It("positive control: the same Fresh/behind alternation flips CPUHealth on every tick once the behind observations cross the freshness boundary", func() {
			// The seam reads maxAge as its package constant (3s), so this spec
			// pushes the behind observations past it (age 4s > 3s maxAge): the
			// alternation straddles the classification boundary and must flip
			// CPUHealth on every tick. A gate that reported zero here AND zero in
			// the missed-poll spec would be measuring nothing.
			categories := flickerRun(
				-500*time.Millisecond, // Fresh: Active
				-4*time.Second,        // beyond maxAge: Degraded
				-500*time.Millisecond, // Fresh again: Active
				-4*time.Second,        // Degraded again
				-500*time.Millisecond,
				-4*time.Second,
			)

			Expect(transitions(categories)).To(Equal(5))
		})
	})

	Context("[use-fsmv2-cpu-false]", func() {
		It("should take CPU health from the CPU health check when USE_FSMV2_CPU is false", func() {
			GinkgoT().Setenv(useFSMv2CPUEnv, "false")

			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond),
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: cpuhealth.Verdict{State: cpuhealth.StateHealthy},
						Details: workerHealthyDetails,
						Message: workerHealthyMessage,
					},
				},
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// The flag is ignored, so a host whose CPU health check says healthy
			// stays healthy: there is no 70% usage rule left to fall back to.
			Expect(status.CPUHealth).To(Equal(models.Active))
			Expect(status.CPU.Health.Message).To(Equal(workerHealthyMessage))
		})
	})

	Context("[warn-once]", func() {
		// noClient prepares the no-client state: a nil process client plus an
		// observer on the component logger.
		noClient := func() *observer.ObservedLogs {
			previous := fsmv2client.GetClient()
			fsmv2client.SetClient(nil)
			DeferCleanup(func() { fsmv2client.SetClient(previous) })

			return observeWarns()
		}

		It("should warn once, naming the supervisor that is not running, when no client is published", func() {
			logs := noClient()

			container_monitor.ResetFSMv2SupervisorNotRunningOnce()

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			// No worker is left to measure the box: the CPU record degrades
			// and carries the diagnosis, once, not per tick.
			for i := 0; i < 3; i++ {
				status, err := service.GetStatus(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(status.CPU).NotTo(BeNil())
				Expect(status.CPUHealth).To(Equal(models.Degraded))
				Expect(status.CPU.Health.Message).To(Equal(supervisorNotRunningMessage))
				// The state pair is asserted HERE and only here. Everywhere else
				// it is pinned through status.CPUHealth, which is derived from
				// Category; this is the one case where a hand-written Health could
				// carry a state that disagrees with its own category and nothing
				// would notice.
				Expect(status.CPU.Health.ObservedState).To(Equal("degraded"))
				Expect(status.CPU.Health.DesiredState).To(Equal("active"))
			}

			notRunningWarns := logs.Filter(func(entry observer.LoggedEntry) bool {
				return entry.LoggerName == logger.ComponentContainerMonitorService &&
					entry.Message == supervisorNotRunningMessage
			}).Len()
			Expect(notRunningWarns).To(Equal(1))
		})
	})
})

// The seam's two judgement functions, called directly. Everything below is
// reachable through GetStatus too, but only with a published fsmv2 client and a
// staged store.
var _ = Describe("the CPU seam's judgement, called without a client", func() {
	It("keeps the store's own error in the message of a failed read", func() {
		health, cpuHealth := container_monitor.JudgeWorkerCPUReadError(errors.New("boltdb: bucket not found"))

		Expect(health).NotTo(BeNil())
		Expect(health.Message).To(Equal("CPU worker observation could not be read: boltdb: bucket not found"))
		Expect(health.Category).To(Equal(models.Degraded))
		Expect(health.ObservedState).To(Equal(models.Degraded.String()))
		Expect(health.DesiredState).To(Equal(models.Active.String()))
		Expect(cpuHealth).To(BeNil(), "a failed read produced no measurement to attach")
	})

	DescribeTable("an observation it cannot trust",
		func(freshness fsmv2client.Freshness, expectedMessage string) {
			// A healthy verdict is staged on purpose: nothing here may read it,
			// so a build that fell through to the verdict would report Active
			// and fail this table rather than pass quietly.
			health, cpuHealth := container_monitor.JudgeWorkerCPU(healthyWorkerStatus(), freshness)

			Expect(health).NotTo(BeNil())
			Expect(health.Message).To(Equal(expectedMessage))
			Expect(health.Category).To(Equal(models.Degraded))
			Expect(cpuHealth).To(BeNil(), "no arm here judged a measurement")
		},
		Entry("stale", fsmv2client.Stale,
			"CPU worker observation is stale (older than "+fsmv2cpu.MaxObservationAge.String()+"); cannot trust the verdict it carries"),
		Entry("never observed", fsmv2client.NotFound,
			"CPU worker has never observed; no measurement to judge"),
		Entry("removed", fsmv2client.Deleted,
			"CPU worker was removed; no measurement to judge"),
		Entry("unknown freshness with no read error", fsmv2client.Unknown,
			"CPU worker observation could not be classified; no measurement to judge"),
	)

	It("passes a fresh healthy verdict through with its evidence attached", func() {
		// On the one verdict that did judge a measurement, the evidence travels
		// with it.
		health, cpuHealth := container_monitor.JudgeWorkerCPU(healthyWorkerStatus(), fsmv2client.Fresh)

		Expect(health).NotTo(BeNil())
		Expect(health.Message).To(Equal(workerHealthyMessage))
		Expect(health.Category).To(Equal(models.Active))
		Expect(cpuHealth).NotTo(BeNil())
		Expect(cpuHealth.Verdict.State).To(Equal(cpuhealth.StateHealthy))
		Expect(cpuHealth.Details).To(Equal(workerHealthyDetails))
	})
})

// healthyWorkerStatus is one healthy tick as the framework hands it to the
// seam: the developer's CPUStatus inside simple.Status, with Degraded unset.
func healthyWorkerStatus() simple.Status[fsmv2cpu.CPUStatus] {
	return simple.Status[fsmv2cpu.CPUStatus]{
		Result: fsmv2cpu.CPUStatus{
			Verdict: cpuhealth.Verdict{State: cpuhealth.StateHealthy},
			Details: workerHealthyDetails,
			Message: workerHealthyMessage,
		},
	}
}
