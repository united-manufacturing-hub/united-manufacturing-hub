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

package supervisor_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

// syncBuffer is a thread-safe wrapper around bytes.Buffer for concurrent log writing.
type syncBuffer struct {
	buf bytes.Buffer
	mu  sync.Mutex
}

func (s *syncBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.buf.String()
}

func (s *syncBuffer) Sync() error {
	return nil
}

var _ = Describe("Lifecycle Logging", func() {
	var (
		buf    *syncBuffer
		logger *zap.SugaredLogger
		store  *mockStore
		sup    *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState]
		done   <-chan struct{}
		mu     sync.Mutex
	)

	BeforeEach(func() {
		buf = &syncBuffer{}
		store = newMockStore()
	})

	AfterEach(func() {
		mu.Lock()
		localSup := sup
		localDone := done
		mu.Unlock()

		if localSup != nil {
			localSup.Shutdown()
			if localDone != nil {
				<-localDone
			}
		}
	})

	setupLogger := func(level zapcore.Level) {
		encoderConfig := zapcore.EncoderConfig{
			TimeKey:        "ts",
			LevelKey:       "level",
			MessageKey:     "msg",
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.StringDurationEncoder,
		}
		core := zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderConfig),
			zapcore.AddSync(buf),
			level,
		)
		logger = zap.New(core).Sugar()
	}

	setupSupervisor := func() {
		cfg := supervisor.Config{
			WorkerType:              "test",
			Store:                   store,
			Logger:                  logger,
			TickInterval:            100 * time.Millisecond,
			GracefulShutdownTimeout: 100 * time.Millisecond, // Short timeout for tests
		}
		sup = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](cfg)

		identity := fsmv2.Identity{
			ID:         "worker-1",
			Name:       "Test Worker",
			WorkerType: "test",
		}
		worker := &mockWorker{}

		err := sup.AddWorker(identity, worker)
		Expect(err).ToNot(HaveOccurred())
	}

	runSupervisor := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		startedDone := sup.Start(ctx)
		mu.Lock()
		done = startedDone
		mu.Unlock()
		time.Sleep(200 * time.Millisecond)
	}

	parseLogEntries := func() []map[string]interface{} {
		var entries []map[string]interface{}
		lines := strings.Split(buf.String(), "\n")

		for _, line := range lines {
			if line == "" {
				continue
			}

			var logEntry map[string]interface{}
			if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
				continue
			}

			entries = append(entries, logEntry)
		}

		return entries
	}

	findLogEntry := func(entries []map[string]interface{}, level string, lifecycleEvent string) map[string]interface{} {
		for _, entry := range entries {
			if entry["level"] == level && entry["lifecycle_event"] == lifecycleEvent {
				return entry
			}
		}

		return nil
	}

	Describe("Debug Level", func() {
		BeforeEach(func() {
			setupLogger(zapcore.DebugLevel)
		})

		It("should log tick_start with worker_id", func() {
			cfg := supervisor.Config{
				WorkerType:              "test",
				Store:                   store,
				Logger:                  logger,
				TickInterval:            100 * time.Millisecond,
				EnableTraceLogging:      true,
				GracefulShutdownTimeout: 100 * time.Millisecond,
			}
			sup = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](cfg)

			identity := fsmv2.Identity{
				ID:         "worker-1",
				Name:       "Test Worker",
				WorkerType: "test",
			}
			worker := &mockWorker{}

			err := sup.AddWorker(identity, worker)
			Expect(err).ToNot(HaveOccurred())
			runSupervisor()

			entries := parseLogEntries()
			tickStart := findLogEntry(entries, "debug", "tick_start")

			Expect(tickStart).ToNot(BeNil(), "Expected to find 'tick_start' lifecycle event in debug logs")
			Expect(tickStart["worker_id"]).To(Equal("worker-1"))
		})

		It("should log mutex_lock_acquire", func() {
			cfg := supervisor.Config{
				WorkerType:              "test",
				Store:                   store,
				Logger:                  logger,
				TickInterval:            100 * time.Millisecond,
				EnableTraceLogging:      true,
				GracefulShutdownTimeout: 100 * time.Millisecond,
			}
			sup = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](cfg)

			identity := fsmv2.Identity{
				ID:         "worker-1",
				Name:       "Test Worker",
				WorkerType: "test",
			}
			worker := &mockWorker{}

			err := sup.AddWorker(identity, worker)
			Expect(err).ToNot(HaveOccurred())

			runSupervisor()

			entries := parseLogEntries()
			mutexAcquire := findLogEntry(entries, "debug", "mutex_lock_acquire")

			Expect(mutexAcquire).ToNot(BeNil(), "Expected to find 'mutex_lock_acquire' lifecycle event in debug logs")
		})

		It("should log mutex_lock_acquired", func() {
			cfg := supervisor.Config{
				WorkerType:              "test",
				Store:                   store,
				Logger:                  logger,
				TickInterval:            100 * time.Millisecond,
				EnableTraceLogging:      true,
				GracefulShutdownTimeout: 100 * time.Millisecond,
			}
			sup = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](cfg)

			identity := fsmv2.Identity{
				ID:         "worker-1",
				Name:       "Test Worker",
				WorkerType: "test",
			}
			worker := &mockWorker{}

			err := sup.AddWorker(identity, worker)
			Expect(err).ToNot(HaveOccurred())

			runSupervisor()

			entries := parseLogEntries()
			mutexAcquired := findLogEntry(entries, "debug", "mutex_lock_acquired")

			Expect(mutexAcquired).ToNot(BeNil(), "Expected to find 'mutex_lock_acquired' lifecycle event in debug logs")
		})
	})

	Describe("Info Level", func() {
		BeforeEach(func() {
			setupLogger(zapcore.InfoLevel)
			setupSupervisor()
		})

		It("should not log lifecycle events at info level", func() {
			runSupervisor()

			entries := parseLogEntries()

			for _, entry := range entries {
				Expect(entry).ToNot(HaveKey("lifecycle_event"),
					"Found lifecycle_event in info-level logs: %v (should only appear at debug level)",
					entry["lifecycle_event"])
			}
		})
	})

	Describe("logTrace helper method", func() {
		Context("when enableTraceLogging is true", func() {
			BeforeEach(func() {
				setupLogger(zapcore.DebugLevel)
			})

			It("should emit log at debug level", func() {
				cfg := supervisor.Config{
					WorkerType:              "test",
					Store:                   store,
					Logger:                  logger,
					TickInterval:            100 * time.Millisecond,
					EnableTraceLogging:      true,
					GracefulShutdownTimeout: 100 * time.Millisecond,
				}
				sup = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](cfg)

				identity := fsmv2.Identity{
					ID:         "worker-1",
					Name:       "Test Worker",
					WorkerType: "test",
				}
				worker := &mockWorker{}

				err := sup.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())

				runSupervisor()

				entries := parseLogEntries()
				Expect(len(entries)).To(BeNumerically(">", 0), "Expected log entries to be emitted")

				foundDebugEntry := false
				for _, entry := range entries {
					if entry["level"] == "debug" {
						foundDebugEntry = true
						break
					}
				}

				Expect(foundDebugEntry).To(BeTrue(), "Expected at least one debug log entry")
			})
		})

		Context("when enableTraceLogging is false", func() {
			BeforeEach(func() {
				setupLogger(zapcore.DebugLevel)
			})

			It("should not emit lifecycle logs", func() {
				cfg := supervisor.Config{
					WorkerType:              "test",
					Store:                   store,
					Logger:                  logger,
					TickInterval:            100 * time.Millisecond,
					EnableTraceLogging:      false,
					GracefulShutdownTimeout: 100 * time.Millisecond,
				}
				sup = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](cfg)

				identity := fsmv2.Identity{
					ID:         "worker-1",
					Name:       "Test Worker",
					WorkerType: "test",
				}
				worker := &mockWorker{}

				err := sup.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())

				runSupervisor()

				entries := parseLogEntries()

				for _, entry := range entries {
					Expect(entry).ToNot(HaveKey("lifecycle_event"),
						"Found lifecycle_event when enableTraceLogging=false: %v",
						entry["lifecycle_event"])
				}
			})
		})
	})
})
