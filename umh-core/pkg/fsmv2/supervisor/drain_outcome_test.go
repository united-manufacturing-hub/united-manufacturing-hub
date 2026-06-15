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

// Package supervisor internal test for the structured drain-outcome signal
// (ENG-4971): DrainOutcomeClean lets a caller tell a clean graceful shutdown
// from one that warned graceful_shutdown_timeout or
// graceful_shutdown_budget_exhausted, without scraping logs. The signal is
// pinned to the real warn via findLogEvents so it cannot pass by coincidence.
package supervisor

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

var _ = Describe("DrainOutcomeClean (ENG-4971 structured drain-outcome signal)", func() {
	const drainOutcomeType = "drainoutcome"

	var buf *shutdownTestSyncBuffer

	BeforeEach(func() {
		buf = &shutdownTestSyncBuffer{}
	})

	It("reports not-clean when a worker ignores shutdown and the drain times out", func() {
		ctx := context.Background()

		basicStore := memory.NewInMemoryStore()
		for _, suffix := range []string{"_identity", "_desired", "_observed"} {
			Expect(basicStore.CreateCollection(ctx, drainOutcomeType+suffix, nil)).To(Succeed())
		}
		triangularStore := storage.NewTriangularStore(basicStore, deps.NewNopFSMLogger())

		logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)

		root := NewSupervisor[*TestObservedState, *TestDesiredState](Config{
			WorkerType:              drainOutcomeType,
			Store:                   triangularStore,
			Logger:                  logger,
			TickInterval:            cascadeTickInterval,
			GracefulShutdownTimeout: cascadeTruncationBase,
		})

		identity := deps.Identity{
			ID:            "drain-outcome-stuck-worker",
			Name:          "Drain Outcome Stuck Worker",
			WorkerType:    drainOutcomeType,
			HierarchyPath: "drain-outcome-stuck-worker(" + drainOutcomeType + ")",
		}
		Expect(root.AddWorker(identity, &cascadeTreeWorker{
			id:           identity.ID,
			initialState: ignoresShutdownState{},
		})).To(Succeed())

		desiredDoc := persistence.Document{
			FieldID:             identity.ID,
			"ShutdownRequested": false,
			"state":             "running",
		}
		_, err := triangularStore.SaveDesired(ctx, drainOutcomeType, identity.ID, desiredDoc)
		Expect(err).ToNot(HaveOccurred())

		runCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_ = root.Start(runCtx)
		defer root.Shutdown()

		Eventually(func() int {
			return len(root.ListWorkers())
		}, 5*time.Second, 20*time.Millisecond).Should(Equal(1),
			"the stuck worker never spawned")

		root.Shutdown()

		logOutput := buf.String()

		// Pin the flag to the real timeout signal: the warn must have fired,
		// so DrainOutcomeClean=false reflects an actual unclean drain, not a
		// coincidental default.
		Expect(findLogEvents(logOutput, "graceful_shutdown_timeout")).ToNot(BeEmpty(),
			"expected graceful_shutdown_timeout to be logged for a worker that ignores shutdown")

		Expect(root.DrainOutcomeClean()).To(BeFalse(),
			"DrainOutcomeClean must be false after a drain that warned graceful_shutdown_timeout")
	})

	It("reports clean when a worker honors shutdown and the drain completes within budget", func() {
		ctx := context.Background()

		basicStore := memory.NewInMemoryStore()
		for _, suffix := range []string{"_identity", "_desired", "_observed"} {
			Expect(basicStore.CreateCollection(ctx, drainOutcomeType+suffix, nil)).To(Succeed())
		}
		triangularStore := storage.NewTriangularStore(basicStore, deps.NewNopFSMLogger())

		logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)

		root := NewSupervisor[*TestObservedState, *TestDesiredState](Config{
			WorkerType:              drainOutcomeType,
			Store:                   triangularStore,
			Logger:                  logger,
			TickInterval:            cascadeTickInterval,
			GracefulShutdownTimeout: cascadeTruncationBase,
		})

		identity := deps.Identity{
			ID:            "drain-outcome-prompt-worker",
			Name:          "Drain Outcome Prompt Worker",
			WorkerType:    drainOutcomeType,
			HierarchyPath: "drain-outcome-prompt-worker(" + drainOutcomeType + ")",
		}
		Expect(root.AddWorker(identity, &cascadeTreeWorker{
			id:           identity.ID,
			initialState: shutdownHonoringState{},
		})).To(Succeed())

		desiredDoc := persistence.Document{
			FieldID:             identity.ID,
			"ShutdownRequested": false,
			"state":             "running",
		}
		_, err := triangularStore.SaveDesired(ctx, drainOutcomeType, identity.ID, desiredDoc)
		Expect(err).ToNot(HaveOccurred())

		runCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_ = root.Start(runCtx)
		defer root.Shutdown()

		Eventually(func() int {
			return len(root.ListWorkers())
		}, 5*time.Second, 20*time.Millisecond).Should(Equal(1),
			"the prompt worker never spawned")

		root.Shutdown()

		logOutput := buf.String()

		Expect(findLogEvents(logOutput, "graceful_shutdown_timeout")).To(BeEmpty(),
			"a worker that honors shutdown must drain warn-free")

		Expect(root.DrainOutcomeClean()).To(BeTrue(),
			"DrainOutcomeClean must be true after a warn-free drain")
	})
})
