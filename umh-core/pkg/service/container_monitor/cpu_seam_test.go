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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// usefsmv2CPUEnv is the environment variable the seam reads: the seam is wired
// to the fsmv2 CPU worker at construction exactly when this flag is set.
const usefsmv2CPUEnv = "USE_FSMV2_CPU"

// workerVerdictMessage is cpuhealth.ComposeMessage's output for a degraded
// throttling verdict — the value the CPU worker stages into CPUStatus.Message
// on a degraded tick. It is sourced from ComposeMessage, never invented: a real
// message-format regression moves the staged value and the assertion together,
// so a drift cannot sail the suite. It discriminates the two paths because the
// legacy getCPUMetrics path can never emit it (legacy emits "CPU utilization
// normal|warning|critical" or "CPU throttled (...)").
var workerVerdictMessage = cpuhealth.ComposeMessage(
	cpuhealth.Verdict{
		State:  cpuhealth.StateDegraded,
		Causes: []cpuhealth.Cause{{Kind: cpuhealth.CauseKindThrottling, Value: 0.5}},
	},
	cpuhealth.Details{ThrottleRatio: 0.5},
)

// workerHealthyMessage is cpuhealth.ComposeMessage's output for a healthy
// verdict on the no-limit budget dashboard, staged from the same source and
// discriminating the two paths exactly like workerVerdictMessage.
var workerHealthyMessage = cpuhealth.ComposeMessage(
	cpuhealth.Verdict{State: cpuhealth.StateHealthy},
	cpuhealth.Details{
		CapacityCores:          8,
		LimitApplies:           false,
		HostBusyRingActive:     true,
		HostBusyCoresAvailable: true,
		AvgHostBusyCores:       2,
		ReserveCores:           1,
	},
)

// seamTransportOffWarning, seamCredentialsWarning, and seamStillStartingWarning
// mirror the three diagnostic warnings readWorkerCPUHealth emits when the flag
// is on but the fsmv2 supervisor cannot run or its client is not published yet.
// The warn-once specs pin message content, not just a count, because the warning
// must name the missing prerequisite.
const seamTransportOffWarning = "USE_FSMV2_CPU is enabled but USE_FSMV2_TRANSPORT is off, so the fsmv2 supervisor never runs and no CPU worker client is published; falling back to legacy CPU metrics"

const seamCredentialsWarning = "USE_FSMV2_CPU is enabled but API_URL or AUTH_TOKEN is unset, so the fsmv2 supervisor never runs and no CPU worker client is published; falling back to legacy CPU metrics"

const seamStillStartingWarning = "USE_FSMV2_CPU is enabled but no fsmv2 client is reachable yet (the fsmv2 supervisor may still be starting); falling back to legacy CPU metrics"

// cpuStubStateReader is the cpuStubReader harness pattern from
// pkg/fsmv2/fsmv2client/freshness_test.go: a deps.StateReader that serves a
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

var _ = Describe("the CPU seam (USE_FSMV2_CPU)", func() {
	var (
		service      *container_monitor.ContainerMonitorService
		mockFS       *filesystem.MockFileSystem
		ctx          context.Context
		testDataPath string
		envFlag      = usefsmv2CPUEnv
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

	// publishUnregisteredClient publishes a fsmv2 client exactly like
	// publishWorkerClient except that cpu.Ref is never Upserted into its writer,
	// so GetFresh maps the ref to Unregistered — the absence-of-worker row of
	// the seam table. The staged observation is served verbatim if a caller
	// ever does reach the reader, so a Spec can prove the worker was not
	// consulted at all.
	publishUnregisteredClient := func(obs *fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]) *fsmv2client.FSMv2Client {
		writer := dynamicchildren.NewWriter()

		client := fsmv2client.NewFSMv2Client(writer, &cpuStubStateReader{obs: obs})
		previous := fsmv2client.GetClient()
		fsmv2client.SetClient(client)
		DeferCleanup(func() { fsmv2client.SetClient(previous) })

		return client
	}

	// setFlag sets USE_FSMV2_CPU to value and restores the previous value (or
	// absence) when the spec ends.
	setFlag := func(value string) {
		previous, had := os.LookupEnv(envFlag)
		Expect(os.Setenv(envFlag, value)).To(Succeed())
		DeferCleanup(func() {
			if had {
				_ = os.Setenv(envFlag, previous)
			} else {
				_ = os.Unsetenv(envFlag)
			}
		})
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

	Context("[worker-on]", func() {
		It("should degrade CPU health, carrying the worker's reason as the message, from a Fresh observation whose framework Degraded flag is set (a Poll error arrives with a nil error and a zero result verdict)", func() {
			setFlag("true")
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

			// The worker's "cannot measure" declaration must degrade the
			// instance, not fall through to the legacy Active judgement...
			Expect(status.CPUHealth).To(Equal(models.Degraded))
			// ...and, as the CPU arm of OverallHealth, it must degrade the overall
			// health too. The rung's headline property is that a degraded worker
			// verdict drives OverallHealth, so it is pinned here explicitly: the
			// memory/disk arms can only ADD Degraded, never remove it.
			Expect(status.OverallHealth).To(Equal(models.Degraded))
			// ...and the framework reason must land where the protocol-converter
			// resource-limit check (IsResourceLimited) reads the block message.
			Expect(status.CPU.Health.Message).To(Equal(pollErrReason))
			Expect(status.CPU.Health.Category).To(Equal(models.Degraded))
			Expect(status.CPU.Health.ObservedState).To(Equal("degraded"))
			Expect(status.CPU.Health.DesiredState).To(Equal("active"))
		})

		It("should fill status.CPUHealth and status.CPU.Health from the worker's Fresh degraded verdict when USE_FSMV2_CPU is on, and read the flag once at construction", func() {
			setFlag("true")
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond), // Fresh: well inside the seam's 3s maxAge
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: cpuhealth.Verdict{State: cpuhealth.StateDegraded},
						Message: workerVerdictMessage,
					},
				},
			})

			// Read once at construction: the env var must be observed BEFORE the
			// service is built, not on every tick.
			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			// Toggling the env AFTER construction must not move the seam: the
			// flag is not re-read, and the off state must not take over.
			Expect(os.Setenv(envFlag, "false")).To(Succeed())

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// The worker's verdict drives the service-level CPU health...
			Expect(status.CPUHealth).To(Equal(models.Degraded))
			// ...and, as the CPU arm of OverallHealth, it drives the overall
			// health too (the memory/disk arms can only add to a degraded
			// CPU verdict, never pull it back to Active).
			Expect(status.OverallHealth).To(Equal(models.Degraded))
			// ...the worker's message lands where the protocol-converter
			// resource-limit check (IsResourceLimited) reads it...
			Expect(status.CPU.Health.Message).To(Equal(workerVerdictMessage))
			// ...the nested category follows the verdict...
			Expect(status.CPU.Health.Category).To(Equal(models.Degraded))
			// ...and the state vocabulary matches the other health records in
			// this package ("active"/"degraded"), not the worker's raw verdict.
			Expect(status.CPU.Health.ObservedState).To(Equal("degraded"))
			Expect(status.CPU.Health.DesiredState).To(Equal("active"))

			// A second tick still comes from the worker: construction-time read,
			// not per-tick, and the toggled env has not moved it.
			status, err = service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(status.CPU.Health.Message).To(Equal(workerVerdictMessage))
		})

		It("should keep the real measured numbers on a Fresh degraded worker verdict (a busy box the worker assessed degraded carries its genuine usage, not a nil or 0 absent marker)", func() {
			setFlag("true")
			// A measured degraded verdict as the worker really stores it: the
			// framework Degraded flag rides along (healthFromStatus degrades
			// the wrapper on every good degraded poll, and worker.go copies
			// that verdict into Status.Degraded), with the same composed
			// message in Reason and Result. The verdict's degraded state —
			// not the flag alone — is what separates this measured tick from
			// a failed poll, so the wire must keep the real getCPUMetrics
			// numbers.
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond), // Fresh: inside the seam's 3s maxAge
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: cpuhealth.Verdict{State: cpuhealth.StateDegraded},
						Message: workerVerdictMessage,
					},
					Degraded: true,
					Reason:   workerVerdictMessage,
				},
			})

			// Same benign cgroup + memory staging as the framework-Degraded spec,
			// so the legacy getCPUMetrics path succeeds with a deterministic real
			// number: 50% of the 2-core quota = 1000 mCPU; CoreCount is host cores.
			mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
				switch path {
				case "/sys/fs/cgroup/cpu.max":
					return []byte("200000 100000\n"), nil
				case "/sys/fs/cgroup/cpu.stat":
					return []byte("nr_periods 2000\nnr_throttled 0\nthrottled_usec 0\n"), nil
				case "/sys/fs/cgroup/memory.max":
					return []byte("4294967296\n"), nil
				case "/sys/fs/cgroup/memory.current":
					return []byte("1073741824\n"), nil
				}

				return nil, errors.New("file not found")
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)
			service.SetCPUUsageProvider(func(_ context.Context) (float64, error) {
				return 50.0, nil
			})

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// The measured degraded verdict degrades the CPU health and carries
			// the worker's message...
			Expect(status.CPUHealth).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Category).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Message).To(Equal(workerVerdictMessage))
			// ...but the numbers it judged are genuine measurements: the seam
			// must NOT nil them (that is for the cannot-measure arms) and must
			// NOT zero them. The injected 50% usage makes the value deterministic:
			// 0.5 * 2.0 quota * 1000 = 1000 mCPU.
			Expect(status.CPU.TotalUsageMCpu).NotTo(BeNil())
			Expect(*status.CPU.TotalUsageMCpu).To(BeNumerically(">", 0))
			Expect(status.CPU.CoreCount).NotTo(BeNil())
			Expect(*status.CPU.CoreCount).To(BeNumerically(">", 0))

			// Wire contract: the real measurement ships with both keys present.
			data, err := json.Marshal(status.CPU)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("totalUsageMCpu"))
			Expect(string(data)).To(ContainSubstring("coreCount"))
		})

		It("should fill status.CPU.Health from a Fresh healthy worker verdict with the Active category", func() {
			setFlag("true")
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond),
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: cpuhealth.Verdict{State: cpuhealth.StateHealthy},
						Message: workerHealthyMessage,
					},
				},
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// A real healthy verdict fills the nested health record. status.CPUHealth
			// is deliberately NOT asserted here: the authoritative-rule sibling below
			// pins that a consumed healthy verdict skips the legacy 70% re-judgement,
			// so this spec stays limited to the nested record.
			Expect(status.CPU.Health.Message).To(Equal(workerHealthyMessage))
			Expect(status.CPU.Health.Category).To(Equal(models.Active))
			Expect(status.CPU.Health.ObservedState).To(Equal("active"))
			Expect(status.CPU.Health.DesiredState).To(Equal("active"))
			// A healthy verdict was measured: the legacy numerics stay real and
			// non-nil (getCPUMetrics always sets them), shipping on the wire.
			Expect(status.CPU.TotalUsageMCpu).NotTo(BeNil())
			Expect(status.CPU.CoreCount).NotTo(BeNil())
			data, err := json.Marshal(status.CPU)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("totalUsageMCpu"))
			Expect(string(data)).To(ContainSubstring("coreCount"))
		})

		It("should not let the legacy 70% CPU-usage rule re-judge a fresh healthy worker verdict when USE_FSMV2_CPU is on (the consumed verdict is authoritative on a busy host)", func() {
			setFlag("true")
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond),
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: cpuhealth.Verdict{State: cpuhealth.StateHealthy},
						Message: workerHealthyMessage,
					},
				},
			})

			// Stage host-independent cgroup data: a benign memory usage so the
			// memory arm cannot degrade the aggregate, and a throttle-free
			// cpu.stat + a 2-core quota so the throttle path stays benign and the
			// CPU-percentage maths below is deterministic. The injected usage
			// provider is what trips the legacy 70% rule.
			mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
				switch path {
				case "/sys/fs/cgroup/cpu.max":
					return []byte("200000 100000\n"), nil
				case "/sys/fs/cgroup/cpu.stat":
					return []byte("nr_periods 2000\nnr_throttled 0\nthrottled_usec 0\n"), nil
				case "/sys/fs/cgroup/memory.max":
					return []byte("4294967296\n"), nil
				case "/sys/fs/cgroup/memory.current":
					return []byte("1073741824\n"), nil
				}

				return nil, errors.New("file not found")
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)
			// Inject a synthetic 90% usage: a host the legacy rule WOULD judge
			// degraded. The worker's fresh healthy verdict is authoritative under
			// flag-on, so neither the aggregate CPU health nor the overall health
			// may be flipped to Degraded by the legacy 70% re-judgement.
			service.SetCPUUsageProvider(func(_ context.Context) (float64, error) {
				return 90.0, nil
			})

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// The verdict was consumed by the seam...
			Expect(status.CPU.Health.Message).To(Equal(workerHealthyMessage))
			// ...and it is authoritative: the legacy 70% rule must NOT re-judge
			// the worker's numbers and flip a busy host to Degraded. Only
			// status.CPUHealth is asserted here (not status.OverallHealth): the
			// disk arm below reads the real host disk through gopsutil, which the
			// mock filesystem cannot stage, so OverallHealth would depend on the
			// CI host's disk state. The CPUHealth assertion is the discriminator —
			// the legacy rule sets it too, so any firing of the rule fails here.
			// This single-tick spec also exercises the throttle early-window (<2
			// snapshots): isThrottled cannot fire on the first tick, so a 90% host
			// with a healthy verdict is judged purely by the authoritative rule —
			// the accepted residual for a just-restarted host.
			Expect(status.CPUHealth).To(Equal(models.Active))
		})

		It("should keep the healthy verdict authoritative once the throttle window is armed, not only during the cold-start tick (a 2nd-tick window with a sub-threshold ratio must not let the legacy 70% rule flip the busy host)", func() {
			setFlag("true")
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond),
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: cpuhealth.Verdict{State: cpuhealth.StateHealthy},
						Message: workerHealthyMessage,
					},
				},
			})

			// Same benign cgroup staging as the cold-start spec, plus a cpu.stat
			// the test advances across the 2nd tick so the throttle window is
			// armed with a real 2-snapshot delta — not the <2-snapshot warm-up.
			cpuStat := []byte("nr_periods 1000\nnr_throttled 75\nthrottled_usec 5000000\n")
			mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
				switch path {
				case "/sys/fs/cgroup/cpu.max":
					return []byte("200000 100000\n"), nil
				case "/sys/fs/cgroup/cpu.stat":
					return cpuStat, nil
				case "/sys/fs/cgroup/memory.max":
					return []byte("4294967296\n"), nil
				case "/sys/fs/cgroup/memory.current":
					return []byte("1073741824\n"), nil
				}

				return nil, errors.New("file not found")
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)
			service.SetCPUUsageProvider(func(_ context.Context) (float64, error) {
				return 90.0, nil
			})

			// Tick 1 seeds the throttle window.
			_, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			// Tick 2 computes a real delta (1000 periods, 25 throttled = 2.5%) —
			// above the 2-snapshot arming threshold but below
			// CPUThrottleRatioThreshold, so isThrottled stays false and the
			// healthy verdict remains authoritative. The 90% injection would trip
			// the legacy 70% rule if it ran, so CPUHealth staying Active pins that
			// the authoritative skip is deliberate on an armed window too, not an
			// accident of the cold-start tick.
			// Re-publish the worker observation with a fresh CollectedAt so tick 2
			// cannot age the -500ms observation past the seam's 3s maxAge on a slow
			// host and fail closed to Stale, which would spuriously fail the Active
			// assertions below. The healthy verdict is what pins the behaviour.
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now(),
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: cpuhealth.Verdict{State: cpuhealth.StateHealthy},
						Message: workerHealthyMessage,
					},
				},
			})

			cpuStat = []byte("nr_periods 2000\nnr_throttled 100\nthrottled_usec 5000000\n")
			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// The verdict was consumed... and it is still authoritative: the armed
			// (but sub-threshold) throttle window must not hand the 90% host back
			// to the legacy rule. Only status.CPUHealth is asserted, for the same
			// real-host-disk reason as the cold-start spec.
			Expect(status.CPU.Health.Message).To(Equal(workerHealthyMessage))
			Expect(status.CPUHealth).To(Equal(models.Active))
		})

		It("should fail GetStatus with the usage provider's error rather than swallow it as a 0% reading (a broken usage source must not report Active)", func() {
			setFlag("true")
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond),
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: cpuhealth.Verdict{State: cpuhealth.StateHealthy},
						Message: workerHealthyMessage,
					},
				},
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)
			service.SetCPUUsageProvider(func(_ context.Context) (float64, error) {
				return 0, errors.New("usage source failed")
			})

			// The usage source fails before the seam runs: the error must
			// propagate as the "failed to get CPU metrics" wrapper — even over a
			// Fresh healthy worker verdict — so a broken source can never be
			// swallowed as a 0% usage that reads as a healthy box.
			_, err := service.GetStatus(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get CPU metrics"))
		})

		It("should keep the legacy throttled health when the worker's Fresh observation carries no determination (an empty result verdict with the framework not degraded must not erase legacy health)", func() {
			setFlag("true")
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond),
				// Genuine "no determination": an empty result verdict and no
				// framework degraded declaration. A poll error is NOT this shape —
				// the worker sets Degraded with a reason for that. This is a
				// successful poll that produced no verdict, and an empty result
				// must not be read as healthy.
				Status: simple.Status[fsmv2cpu.CPUStatus]{Result: fsmv2cpu.CPUStatus{}},
			})

			// Stage a throttled cgroup so the legacy path judges the box degraded:
			// the worker can fail to measure while legacy throttling detection still
			// works (they read different sources).
			cpuStat := []byte("nr_periods 1000\nnr_throttled 500\nthrottled_usec 50000000\n")
			mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
				switch path {
				case "/sys/fs/cgroup/cpu.max":
					return []byte("200000 100000\n"), nil
				case "/sys/fs/cgroup/cpu.stat":
					return cpuStat, nil
				}

				return nil, errors.New("file not found")
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			// Tick 1 seeds the throttle window; tick 2 sees the throttled delta.
			_, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			cpuStat = []byte("nr_periods 2000\nnr_throttled 1000\nthrottled_usec 50000000\n")
			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// The worker's missing verdict must not erase the legacy degraded
			// health of a genuinely throttled box.
			Expect(status.CPUHealth).To(Equal(models.Degraded))
			Expect(status.OverallHealth).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Category).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Message).To(ContainSubstring("CPU throttled"))
			Expect(status.CPU.Health.ObservedState).To(Equal("degraded"))
			Expect(status.CPU.Health.DesiredState).To(Equal("active"))
		})

		It("should let the legacy usage rule degrade CPU health when the worker's Fresh observation carries no determination (a no-verdict tick is not authoritative, so a hot host the rule sees stays degraded)", func() {
			setFlag("true")
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond),
				// Genuine "no determination": an empty result verdict and no framework
				// degraded declaration. The worker made no judgement, so the tick is
				// not authoritative and the legacy usage rule below still runs.
				Status: simple.Status[fsmv2cpu.CPUStatus]{Result: fsmv2cpu.CPUStatus{}},
			})

			// Stage a fractional cgroup quota (cpu.max 5000/100000 = 0.05 cores) so the
			// legacy usage rule fires on the mCpu math while the nested health record
			// stays Active: getRawCPUMetrics clamps the quota to 0.1 for TotalUsageMCpu,
			// but the rule below divides by the raw 0.05 quota (CgroupCores), so a 40%
			// host reads as 80% to the rule. The rule is therefore the ONLY degradation
			// source, pinning the else-if body against the worker verdict path.
			mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
				switch path {
				case "/sys/fs/cgroup/cpu.max":
					return []byte("5000 100000\n"), nil
				case "/sys/fs/cgroup/cpu.stat":
					return []byte("nr_periods 2000\nnr_throttled 0\nthrottled_usec 0\n"), nil
				case "/sys/fs/cgroup/memory.max":
					return []byte("4294967296\n"), nil
				case "/sys/fs/cgroup/memory.current":
					return []byte("1073741824\n"), nil
				}

				return nil, errors.New("file not found")
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)
			// Inject a 40% host: the nested record reads Active, but the quota-divergence
			// math makes the legacy rule judge it 80% and degrade the aggregates.
			service.SetCPUUsageProvider(func(_ context.Context) (float64, error) {
				return 40.0, nil
			})

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// A no-determination tick is not authoritative: the legacy usage rule still
			// runs and degrades a host it computes as >70% effective usage.
			Expect(status.CPUHealth).To(Equal(models.Degraded))
			Expect(status.OverallHealth).To(Equal(models.Degraded))
			// The rule only drives the service-level aggregates, not the nested record:
			// the 40% host read stays Active there, proving the else-if body — not the
			// nested health — did the degradation.
			Expect(status.CPU.Health.Category).To(Equal(models.Active))
			Expect(status.CPU.Health.Message).To(Equal("CPU utilization normal"))
		})

		It("should keep the legacy throttled health when a Fresh healthy verdict cannot erase it (the seam supersedes legacy only in the degraded direction)", func() {
			setFlag("true")
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond),
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: cpuhealth.Verdict{State: cpuhealth.StateHealthy},
						Message: workerHealthyMessage,
					},
				},
			})

			// Same throttled cgroup staging as the no-verdict spec: the legacy
			// path judges the box degraded by throttling while the worker's
			// Fresh healthy verdict claims otherwise. A healthy verdict must not
			// overwrite the throttle record the aggregate check below reads.
			cpuStat := []byte("nr_periods 1000\nnr_throttled 500\nthrottled_usec 50000000\n")
			mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
				switch path {
				case "/sys/fs/cgroup/cpu.max":
					return []byte("200000 100000\n"), nil
				case "/sys/fs/cgroup/cpu.stat":
					return cpuStat, nil
				}

				return nil, errors.New("file not found")
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)
			// Inject a deterministic non-zero usage so the preservation
			// assertions discriminate the throttle bypass from an idle host that
			// naturally reads 0 mCPU.
			service.SetCPUUsageProvider(func(_ context.Context) (float64, error) {
				return 50.0, nil
			})

			// Tick 1 seeds the throttle window; tick 2 sees the throttled delta.
			_, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			cpuStat = []byte("nr_periods 2000\nnr_throttled 1000\nthrottled_usec 50000000\n")
			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// The worker's healthy verdict must not erase the legacy degraded
			// health of a genuinely throttled box, in the record, the aggregate
			// CPU health, or the overall health.
			Expect(status.CPUHealth).To(Equal(models.Degraded))
			Expect(status.OverallHealth).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Category).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Message).To(ContainSubstring("CPU throttled"))
			Expect(status.CPU.Health.Message).NotTo(Equal(workerHealthyMessage))
			Expect(status.CPU.Health.ObservedState).To(Equal("degraded"))
			Expect(status.CPU.Health.DesiredState).To(Equal("active"))
			// The legacy throttle bypass preserves the real numerics: the worker
			// block is skipped on a legacy-throttled tick, so TotalUsageMCpu and
			// CoreCount keep the legacy getCPUMetrics readings instead of being
			// nil-ed or zeroed as absent markers (a build that nils or zeroes
			// under the throttle gate fails here).
			Expect(status.CPU.TotalUsageMCpu).NotTo(BeNil())
			Expect(*status.CPU.TotalUsageMCpu).To(BeNumerically(">", 0))
			Expect(status.CPU.CoreCount).NotTo(BeNil())
			Expect(*status.CPU.CoreCount).To(BeNumerically(">", 0))
		})
	})

	Context("[unmeasured-omits]", func() {
		It("should omit TotalUsageMCpu and CoreCount from the wire on a worker tick that could not measure (a framework-Degraded observation with no result verdict ships nil, not 0)", func() {
			setFlag("true")
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

			// Stage benign cgroup + memory files so the legacy getCPUMetrics
			// path SUCCEEDS (GetStatus must not early-return) and would
			// otherwise fill a real usage number. The injected 50% usage provider
			// makes that real number deterministic: 0.5 * effectiveCores * 1000
			// > 0, and CoreCount is runtime.NumCPU() from the legacy path.
			mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
				switch path {
				case "/sys/fs/cgroup/cpu.max":
					return []byte("200000 100000\n"), nil
				case "/sys/fs/cgroup/cpu.stat":
					return []byte("nr_periods 2000\nnr_throttled 0\nthrottled_usec 0\n"), nil
				case "/sys/fs/cgroup/memory.max":
					return []byte("4294967296\n"), nil
				case "/sys/fs/cgroup/memory.current":
					return []byte("1073741824\n"), nil
				}

				return nil, errors.New("file not found")
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)
			service.SetCPUUsageProvider(func(_ context.Context) (float64, error) {
				return 50.0, nil
			})

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// The genuinely-unmeasured framework-Degraded arm must NOT keep the
			// legacy real numbers and must NOT fabricate a 0: both fields are
			// nil, so the wire omits totalUsageMCpu and coreCount entirely.
			// This is the departure's core assertion — a build that leaves the
			// legacy real CoreCount/TotalUsageMCpu, or ships a 0, fails here.
			Expect(status.CPU.Health.Category).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Message).NotTo(BeEmpty())
			Expect(status.CPU.TotalUsageMCpu).To(BeNil())
			Expect(status.CPU.CoreCount).To(BeNil())

			// Wire contract: nil measurements omit both keys from the JSON.
			data, err := json.Marshal(status.CPU)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).NotTo(ContainSubstring("totalUsageMCpu"))
			Expect(string(data)).NotTo(ContainSubstring("coreCount"))
		})
	})

	Context("[cpuhealth-evidence]", func() {
		It("should fill status.CPU.CPUHealth from one worker tick's verdict and details, and omit the key entirely on a tick that measured nothing", func() {
			setFlag("true")

			// The fills arm stages what one measured tick stores: a degraded
			// verdict WITH attribution and causes — the console panel's whole
			// purpose, since without causes no row reddens — beside the Details
			// it judged, including a present p95 Reading so the optional-member
			// pass-through is pinned, not just the always-float members. The
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
			// observation is the only source of these values, so equality pins
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

			// The omits arm: a tick that measured nothing. The framework-
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

	Context("[freshness]", func() {
		It("should degrade CPU health when the worker's observation is stale, even though the verdict it carries is healthy", func() {
			setFlag("true")
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				// Older than the seam's 3s maxAge: GetFresh maps this to
				// Stale even though the verdict it wraps is a healthy one.
				CollectedAt: time.Now().Add(-4 * time.Second),
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: cpuhealth.Verdict{State: cpuhealth.StateHealthy},
						Message: workerHealthyMessage,
					},
				},
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)
			// Inject a deterministic non-zero usage so the zeroing assertion
			// discriminates the absent-marker write from an idle host that
			// naturally reads 0 mCPU.
			service.SetCPUUsageProvider(func(_ context.Context) (float64, error) {
				return 50.0, nil
			})

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// A stale observation is not a healthy one: the seam must fail
			// closed and drive the service-level CPU health to degraded...
			Expect(status.CPUHealth).To(Equal(models.Degraded))
			// ...and the degrade must say so in words, because the
			// protocol-converter resource-limit check (IsResourceLimited) reads
			// CPU.Health.Message as its block reason — the healthy verdict must
			// also not sail through.
			Expect(status.CPU.Health.Category).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Message).To(ContainSubstring("stale"))
			Expect(status.CPU.Health.Message).NotTo(Equal(workerHealthyMessage))
			Expect(status.CPU.Health.ObservedState).To(Equal("degraded"))
			Expect(status.CPU.Health.DesiredState).To(Equal("active"))
			// The freshness fail-closed verdict nils the numerics in the same write
			// as the Degraded category, exactly like the framework-Degraded arm:
			// a stale record must not ship a legacy real number beside its
			// verdict, nor fabricate a 0 (the injected 50% usage would otherwise
			// read non-zero here).
			Expect(status.CPU.TotalUsageMCpu).To(BeNil())
			Expect(status.CPU.CoreCount).To(BeNil())
		})

		It("should degrade CPU health when the worker is running but has never observed (the store returns ErrNotFound)", func() {
			setFlag("true")
			publishWorkerClientWithStub(&cpuStubStateReader{err: persistence.ErrNotFound})

			// Stage host-independent cgroup data so the cgroup read succeeds
			// and the throttle path stays benign. What pins the branch is the
			// message assertions: the legacy path and the read-error branch can
			// never emit "never observed", so the spec discriminates on any host
			// (without them, a busy CI host's legacy usage rule alone would
			// satisfy the degraded assertions on the legacy fallback).
			mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
				switch path {
				case "/sys/fs/cgroup/cpu.max":
					return []byte("200000 100000\n"), nil
				case "/sys/fs/cgroup/cpu.stat":
					return []byte("nr_periods 2000\nnr_throttled 0\nthrottled_usec 0\n"), nil
				}

				return nil, errors.New("file not found")
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// NeverObserved is an absence of measurement, and an absent
			// measurement is not a healthy one: it must fail closed, not fall
			// back to the legacy Active judgement, and the message must name the
			// never-observed cause (the protocol-converter resource-limit check
			// reads it as the bridge-block reason).
			Expect(status.CPUHealth).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Category).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Message).To(ContainSubstring("never observed"))
			Expect(status.CPU.Health.Message).NotTo(ContainSubstring("could not be read"))
			Expect(status.CPU.Health.ObservedState).To(Equal("degraded"))
			Expect(status.CPU.Health.DesiredState).To(Equal("active"))
		})

		It("should degrade CPU health when the worker's observation cannot be read (a generic store error maps Unknown to degraded, not a fall-through)", func() {
			setFlag("true")
			publishWorkerClientWithStub(&cpuStubStateReader{err: errors.New("cpu store read failed")})

			// Stage host-independent cgroup data so the cgroup read succeeds
			// and the throttle path stays benign. What pins the branch is the
			// message assertions: the legacy path and the never-observed branch
			// can never emit the read-error message, so the spec discriminates
			// on any host.
			mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
				switch path {
				case "/sys/fs/cgroup/cpu.max":
					return []byte("200000 100000\n"), nil
				case "/sys/fs/cgroup/cpu.stat":
					return []byte("nr_periods 2000\nnr_throttled 0\nthrottled_usec 0\n"), nil
				}

				return nil, errors.New("file not found")
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// GetFresh maps the read error to Unknown plus a verbatim error. A
			// switch over Freshness that omits the Unknown branch would leave
			// it Neutral and the box Active, so the assertion is Degraded
			// specifically — a fall-through passes only by accident on a box
			// the legacy 70% rule happened to judge degraded. The message must
			// also carry the read error verbatim, so this branch cannot
			// collapse into the never-observed branch.
			Expect(status.CPUHealth).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Category).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Message).To(ContainSubstring("could not be read"))
			Expect(status.CPU.Health.Message).To(ContainSubstring("cpu store read failed"))
			Expect(status.CPU.Health.Message).NotTo(ContainSubstring("never observed"))
			Expect(status.CPU.Health.ObservedState).To(Equal("degraded"))
			Expect(status.CPU.Health.DesiredState).To(Equal("active"))
			// The read-error arm is a genuinely-unmeasured tick: the numerics
			// are nil, so the wire omits them instead of shipping a fabricated 0.
			Expect(status.CPU.TotalUsageMCpu).To(BeNil())
			Expect(status.CPU.CoreCount).To(BeNil())
			data, err := json.Marshal(status.CPU)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).NotTo(ContainSubstring("totalUsageMCpu"))
			Expect(string(data)).NotTo(ContainSubstring("coreCount"))
		})

		It("should keep the legacy CPU health when the worker ref is not registered, falling back rather than degrading", func() {
			setFlag("true")
			// A client exists but cpu.Ref was never Upserted into its writer,
			// so GetFresh maps the ref to Unregistered before it ever reads.
			// The staged observation is one the worker path would serve as
			// degraded, so any seam that consulted the worker at all for an
			// unregistered ref fails these assertions.
			publishUnregisteredClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond),
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

			// Absence of the worker is a legacy fallback, not a degrade: the
			// health record comes from getCPUMetrics, whose message only the
			// legacy path can emit and the worker-degraded path cannot.
			Expect(status.CPU.Health.Message).To(ContainSubstring("CPU utilization"))
			Expect(status.CPU.Health.Message).NotTo(Equal(workerVerdictMessage))
		})
	})

	Context("[flicker]", func() {
		// flickerRun stages one observation age per GetStatus tick and returns
		// the sequence of CPUHealth categories the run produced. The observation
		// ALWAYS carries the same Fresh healthy verdict: only CollectedAt moves
		// between ticks, so a CPUHealth transition recorded here is a freshness
		// transition at the seam — exactly what SPEC §9 P4 R5's flicker gate
		// measures — never a verdict change.
		flickerRun := func(ages ...time.Duration) []models.HealthCategory {
			setFlag("true")
			obs := &fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now(),
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: cpuhealth.Verdict{State: cpuhealth.StateHealthy},
						Message: workerHealthyMessage,
					},
				},
			}
			publishWorkerClient(obs)

			// Benign cgroup and memory staging plus a pinned 50% usage provider,
			// so the legacy getCPUMetrics path never adds a degradation of its
			// own across the run: usagePercent 50 < 70 reads Active, the throttle
			// window stays quiet, and every transition below is a seam freshness
			// effect and nothing else.
			mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
				switch path {
				case "/sys/fs/cgroup/cpu.max":
					return []byte("200000 100000\n"), nil
				case "/sys/fs/cgroup/cpu.stat":
					return []byte("nr_periods 2000\nnr_throttled 0\nthrottled_usec 0\n"), nil
				case "/sys/fs/cgroup/memory.max":
					return []byte("4294967296\n"), nil
				case "/sys/fs/cgroup/memory.current":
					return []byte("1073741824\n"), nil
				}

				return nil, errors.New("file not found")
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)
			service.SetCPUUsageProvider(func(_ context.Context) (float64, error) {
				return 50.0, nil
			})

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
			// still inside maxAge, so GetFresh maps it Fresh and the healthy
			// verdict stays authoritative. Alternating on-time and one-poll-behind
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
			// The SPEC's positive control narrows maxAge to the poll interval so
			// the one-interval-lagged observation becomes not-Fresh and the
			// missed-poll run flips. The seam reads maxAge as its package
			// constant (3s), so this spec realizes the same boundary-crossing by
			// pushing the behind observations past it (age 4s > 3s maxAge): the
			// alternation now straddles the classification boundary and must flip
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

	Context("[legacy-off]", func() {
		It("should keep filling status.CPU through the legacy getCPUMetrics path when USE_FSMV2_CPU is off, even though a worker observation exists", func() {
			setFlag("false")
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond),
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

			// A worker observation exists and is Fresh-degraded, yet the off
			// path must ignore it: the worker's message is a value the legacy
			// path can never emit, so its absence proves the worker was not
			// consulted. The positive control pins that the legacy path really
			// produced its own message: a negative-only check would also pass
			// on a blank fallback, which is the seam's whole safety property
			// to lose.
			Expect(status.CPU.Health.Message).To(ContainSubstring("CPU utilization"))
			Expect(status.CPU.Health.Message).NotTo(Equal(workerVerdictMessage))
		})

		It("should still degrade CPU health through the legacy usage rule on a hot host when USE_FSMV2_CPU is off, even though a healthy worker observation exists", func() {
			setFlag("false")
			// A Fresh healthy worker observation the off path must ignore: if the
			// seam wrongly consulted the worker at flag-off it would consume this
			// and report Active, so the Degraded assertion below is the discriminator.
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond),
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: cpuhealth.Verdict{State: cpuhealth.StateHealthy},
						Message: workerHealthyMessage,
					},
				},
			})

			// Same fractional-quota staging as the no-determination positive control:
			// the legacy rule fires on the mCpu math while the nested record stays
			// Active, making the rule the ONLY degradation source.
			mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
				switch path {
				case "/sys/fs/cgroup/cpu.max":
					return []byte("5000 100000\n"), nil
				case "/sys/fs/cgroup/cpu.stat":
					return []byte("nr_periods 2000\nnr_throttled 0\nthrottled_usec 0\n"), nil
				case "/sys/fs/cgroup/memory.max":
					return []byte("4294967296\n"), nil
				case "/sys/fs/cgroup/memory.current":
					return []byte("1073741824\n"), nil
				}

				return nil, errors.New("file not found")
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)
			service.SetCPUUsageProvider(func(_ context.Context) (float64, error) {
				return 40.0, nil
			})

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// The worker observation is ignored at flag-off, so the legacy usage rule
			// still degrades a host it computes as >70% effective usage, in the CPU
			// health and the overall health.
			Expect(status.CPUHealth).To(Equal(models.Degraded))
			Expect(status.OverallHealth).To(Equal(models.Degraded))
			Expect(status.CPU.Health.Category).To(Equal(models.Active))
			Expect(status.CPU.Health.Message).To(Equal("CPU utilization normal"))
		})
	})

	Context("[warn-once]", func() {
		// noClient prepares the flag-on-but-no-client state both warn-once specs
		// share: a nil process client plus an observer on the component logger.
		noClient := func() *observer.ObservedLogs {
			previous := fsmv2client.GetClient()
			fsmv2client.SetClient(nil)
			DeferCleanup(func() { fsmv2client.SetClient(previous) })

			return observeWarns()
		}

		It("should warn once, naming the off prerequisite, when USE_FSMV2_CPU is on but USE_FSMV2_TRANSPORT is off", func() {
			setFlag("true")
			previous, had := os.LookupEnv("USE_FSMV2_TRANSPORT")
			Expect(os.Setenv("USE_FSMV2_TRANSPORT", "false")).To(Succeed())
			DeferCleanup(func() {
				if had {
					_ = os.Setenv("USE_FSMV2_TRANSPORT", previous)
				} else {
					_ = os.Unsetenv("USE_FSMV2_TRANSPORT")
				}
			})

			logs := noClient()

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			// The legacy fallback must keep serving a status (the flag-on-but-
			// cannot-run case is not an error), and the diagnostic warning must
			// fire once and name the transport prerequisite, not a generic
			// unreachable-client.
			for i := 0; i < 3; i++ {
				status, err := service.GetStatus(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(status.CPU).NotTo(BeNil())
			}

			transportWarns := logs.Filter(func(entry observer.LoggedEntry) bool {
				return entry.LoggerName == logger.ComponentContainerMonitorService &&
					entry.Message == seamTransportOffWarning
			}).Len()
			Expect(transportWarns).To(Equal(1))
		})

		It("should warn once, naming the missing credentials, when USE_FSMV2_CPU is on, transport is on, and API_URL or AUTH_TOKEN is unset", func() {
			setFlag("true")
			prevTransport, hadTransport := os.LookupEnv("USE_FSMV2_TRANSPORT")
			Expect(os.Setenv("USE_FSMV2_TRANSPORT", "true")).To(Succeed())
			DeferCleanup(func() {
				if hadTransport {
					_ = os.Setenv("USE_FSMV2_TRANSPORT", prevTransport)
				} else {
					_ = os.Unsetenv("USE_FSMV2_TRANSPORT")
				}
			})
			prevAPIURL, hadAPIURL := os.LookupEnv("API_URL")
			Expect(os.Setenv("API_URL", "")).To(Succeed())
			DeferCleanup(func() {
				if hadAPIURL {
					_ = os.Setenv("API_URL", prevAPIURL)
				} else {
					_ = os.Unsetenv("API_URL")
				}
			})
			prevToken, hadToken := os.LookupEnv("AUTH_TOKEN")
			Expect(os.Setenv("AUTH_TOKEN", "")).To(Succeed())
			DeferCleanup(func() {
				if hadToken {
					_ = os.Setenv("AUTH_TOKEN", prevToken)
				} else {
					_ = os.Unsetenv("AUTH_TOKEN")
				}
			})

			logs := noClient()

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			for i := 0; i < 3; i++ {
				status, err := service.GetStatus(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(status.CPU).NotTo(BeNil())
			}

			credentialsWarns := logs.Filter(func(entry observer.LoggedEntry) bool {
				return entry.LoggerName == logger.ComponentContainerMonitorService &&
					entry.Message == seamCredentialsWarning
			}).Len()
			Expect(credentialsWarns).To(Equal(1))
		})

		It("should warn once, naming the still-starting supervisor, when USE_FSMV2_CPU is on, transport and credentials are present, and the client is not published yet", func() {
			setFlag("true")
			prevTransport, hadTransport := os.LookupEnv("USE_FSMV2_TRANSPORT")
			Expect(os.Setenv("USE_FSMV2_TRANSPORT", "true")).To(Succeed())
			DeferCleanup(func() {
				if hadTransport {
					_ = os.Setenv("USE_FSMV2_TRANSPORT", prevTransport)
				} else {
					_ = os.Unsetenv("USE_FSMV2_TRANSPORT")
				}
			})
			prevAPIURL, hadAPIURL := os.LookupEnv("API_URL")
			Expect(os.Setenv("API_URL", "https://management.umh.app")).To(Succeed())
			DeferCleanup(func() {
				if hadAPIURL {
					_ = os.Setenv("API_URL", prevAPIURL)
				} else {
					_ = os.Unsetenv("API_URL")
				}
			})
			prevToken, hadToken := os.LookupEnv("AUTH_TOKEN")
			Expect(os.Setenv("AUTH_TOKEN", "test-token")).To(Succeed())
			DeferCleanup(func() {
				if hadToken {
					_ = os.Setenv("AUTH_TOKEN", prevToken)
				} else {
					_ = os.Unsetenv("AUTH_TOKEN")
				}
			})

			logs := noClient()

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			// This is the branch that fires on the realistic boot path: credentials
			// are present but the supervisor has not published the client yet. The
			// diagnostic must name the still-starting supervisor, once, not per
			// tick.
			for i := 0; i < 3; i++ {
				status, err := service.GetStatus(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(status.CPU).NotTo(BeNil())
			}

			stillStartingWarns := logs.Filter(func(entry observer.LoggedEntry) bool {
				return entry.LoggerName == logger.ComponentContainerMonitorService &&
					entry.Message == seamStillStartingWarning
			}).Len()
			Expect(stillStartingWarns).To(Equal(1))
		})
	})
})
