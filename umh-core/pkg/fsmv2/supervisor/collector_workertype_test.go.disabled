// Copyright 2025 UMH Systems GmbH
package supervisor_test

import (
	"context"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"go.uber.org/zap"
)

var _ = Describe("Collector WorkerType", func() {
	// TODO: These tests need to be rewritten for CSE storage.
	// Collector no longer directly saves state - supervisor does via TriangularStore.
	XIt("should use configured workerType when saving observed state", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		logger := zap.NewNop().Sugar()

		var capturedWorkerType string
		var mu sync.Mutex


		worker := &mockWorker{
			collectFunc: func(ctx context.Context) (fsmv2.ObservedState, error) {
				return &container.ContainerObservedState{CollectedAt: time.Now()}, nil
			},
		}

		collector := supervisor.NewCollector(supervisor.CollectorConfig{
			Worker:              worker,
			Identity:            fsmv2.Identity{ID: "test-worker"},
			Store:               nil,
			Logger:              logger,
			ObservationInterval: 50 * time.Millisecond,
			ObservationTimeout:  supervisor.DefaultObservationTimeout,
			WorkerType:          "s6",
		})

		err := collector.Start(ctx)
		Expect(err).ToNot(HaveOccurred())

		time.Sleep(150 * time.Millisecond)

		cancel()
		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		actualType := capturedWorkerType
		mu.Unlock()

		Expect(actualType).To(Equal("s6"), "collector should use workerType from config, not hardcoded 'container'")
	})

	XIt("should use different workerTypes for different collectors", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		logger := zap.NewNop().Sugar()

		capturedWorkerTypes := make(map[string]string)
		var mu sync.Mutex


		worker := &mockWorker{
			collectFunc: func(ctx context.Context) (fsmv2.ObservedState, error) {
				return &container.ContainerObservedState{CollectedAt: time.Now()}, nil
			},
		}

		containerCollector := supervisor.NewCollector(supervisor.CollectorConfig{
			Worker:              worker,
			Identity:            fsmv2.Identity{ID: "container-worker"},
			Store:               nil,
			Logger:              logger,
			ObservationInterval: 50 * time.Millisecond,
			ObservationTimeout:  supervisor.DefaultObservationTimeout,
			WorkerType:          "container",
		})

		s6Collector := supervisor.NewCollector(supervisor.CollectorConfig{
			Worker:              worker,
			Identity:            fsmv2.Identity{ID: "s6-worker"},
			Store:               nil,
			Logger:              logger,
			ObservationInterval: 50 * time.Millisecond,
			ObservationTimeout:  supervisor.DefaultObservationTimeout,
			WorkerType:          "s6",
		})

		benthosCollector := supervisor.NewCollector(supervisor.CollectorConfig{
			Worker:              worker,
			Identity:            fsmv2.Identity{ID: "benthos-worker"},
			Store:               nil,
			Logger:              logger,
			ObservationInterval: 50 * time.Millisecond,
			ObservationTimeout:  supervisor.DefaultObservationTimeout,
			WorkerType:          "benthos",
		})

		err := containerCollector.Start(ctx)
		Expect(err).ToNot(HaveOccurred())

		err = s6Collector.Start(ctx)
		Expect(err).ToNot(HaveOccurred())

		err = benthosCollector.Start(ctx)
		Expect(err).ToNot(HaveOccurred())

		time.Sleep(150 * time.Millisecond)

		cancel()
		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		types := make(map[string]string)
		for k, v := range capturedWorkerTypes {
			types[k] = v
		}
		mu.Unlock()

		Expect(types["container-worker"]).To(Equal("container"))
		Expect(types["s6-worker"]).To(Equal("s6"))
		Expect(types["benthos-worker"]).To(Equal("benthos"))
	})
})
