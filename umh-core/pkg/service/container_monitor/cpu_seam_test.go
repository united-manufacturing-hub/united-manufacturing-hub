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
	"errors"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
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
	cpuhealth.Signals{ThrottleRatio: 0.5},
)

// workerHealthyMessage is cpuhealth.ComposeMessage's output for a healthy
// verdict on the no-limit budget dashboard, staged from the same source and
// discriminating the two paths exactly like workerVerdictMessage.
var workerHealthyMessage = cpuhealth.ComposeMessage(
	cpuhealth.Verdict{State: cpuhealth.StateHealthy},
	cpuhealth.Signals{
		CapacityCores:          8,
		LimitApplies:           false,
		HostBusyRingActive:     true,
		HostBusyCoresAvailable: true,
		HostBusyCores60sMean:   2,
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
}

func (s *cpuStubStateReader) LoadObservedTyped(_ context.Context, _, _ string, result interface{}) error {
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

	// publishWorkerClient registers the single CPU monitor ref in a real writer
	// and publishes a REAL fsmv2 client (NewFSMv2Client + SetClient) whose read
	// side serves the staged observation verbatim. DeferCleanup restores the
	// process globals, so a leak can never bleed into another spec.
	publishWorkerClient := func(obs *fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]) *fsmv2client.FSMv2Client {
		writer := dynamicchildren.NewWriter()
		Expect(writer.Upsert(fsmv2cpu.Ref, map[string]any{})).To(Succeed())

		stub := &cpuStubStateReader{obs: obs}

		client := fsmv2client.NewFSMv2Client(writer, stub)
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
			// ...and the framework reason must land where the admission gate
			// reads the block message.
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
						Verdict: "degraded",
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
			// ...the worker's message lands where the admission gate reads it...
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

		It("should fill status.CPU.Health from a Fresh healthy worker verdict with the Active category", func() {
			setFlag("true")
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond),
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: "healthy",
						Message: workerHealthyMessage,
					},
				},
			})

			service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

			status, err := service.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())

			// A real healthy verdict fills the nested health record. status.CPUHealth
			// is deliberately NOT asserted here: until the follow-up rung gates the
			// legacy CPU-usage rule, a high-usage tick can still re-judge the
			// aggregate below (over-degrading is the known, fail-safe interim).
			Expect(status.CPU.Health.Message).To(Equal(workerHealthyMessage))
			Expect(status.CPU.Health.Category).To(Equal(models.Active))
			Expect(status.CPU.Health.ObservedState).To(Equal("active"))
			Expect(status.CPU.Health.DesiredState).To(Equal("active"))
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

		It("should keep the legacy throttled health when a Fresh healthy verdict cannot erase it (the seam supersedes legacy only in the degraded direction)", func() {
			setFlag("true")
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond),
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: "healthy",
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
		})
	})

	Context("[legacy-off]", func() {
		It("should keep filling status.CPU through the legacy getCPUMetrics path when USE_FSMV2_CPU is off, even though a worker observation exists", func() {
			setFlag("false")
			publishWorkerClient(&fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
				CollectedAt: time.Now().Add(-500 * time.Millisecond),
				Status: simple.Status[fsmv2cpu.CPUStatus]{
					Result: fsmv2cpu.CPUStatus{
						Verdict: "degraded",
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
