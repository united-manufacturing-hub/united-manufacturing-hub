// Copyright 2025 UMH Systems GmbH
package explorer_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/explorer"
)

var _ = Describe("ContainerScenario", func() {
	Describe("setup and cleanup", func() {
		It("should create container scenario", func() {
			scenario := explorer.NewContainerScenario()
			Expect(scenario).ToNot(BeNil())
		})

		It("should setup Docker container", func() {
			scenario := explorer.NewContainerScenario()

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			err := scenario.Setup(ctx)
			Expect(err).ToNot(HaveOccurred())

			defer func() {
				// 30s timeout: Docker graceful shutdown (~10s) + API overhead (~5s) + buffer
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()
				_ = scenario.Cleanup(cleanupCtx)
			}()

			state, _ := scenario.GetCurrentState()
			Expect(state).ToNot(BeEmpty())
		})

		It("should cleanup Docker container", func() {
			scenario := explorer.NewContainerScenario()

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			err := scenario.Setup(ctx)
			Expect(err).ToNot(HaveOccurred())

			// 30s timeout: Docker graceful shutdown (~10s) + API overhead (~5s) + buffer
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cleanupCancel()

			err = scenario.Cleanup(cleanupCtx)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("FSM tick", func() {
		It("should execute FSM tick", func() {
			scenario := explorer.NewContainerScenario()

			setupCtx, setupCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer setupCancel()

			err := scenario.Setup(setupCtx)
			Expect(err).ToNot(HaveOccurred())

			defer func() {
				// 30s timeout: Docker graceful shutdown (~10s) + API overhead (~5s) + buffer
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()
				_ = scenario.Cleanup(cleanupCtx)
			}()

			tickCtx, tickCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer tickCancel()

			err = scenario.Tick(tickCtx)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("state inspection", func() {
		It("should return current FSM state", func() {
			scenario := explorer.NewContainerScenario()

			setupCtx, setupCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer setupCancel()

			err := scenario.Setup(setupCtx)
			Expect(err).ToNot(HaveOccurred())

			defer func() {
				// 30s timeout: Docker graceful shutdown (~10s) + API overhead (~5s) + buffer
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()
				_ = scenario.Cleanup(cleanupCtx)
			}()

			state, reason := scenario.GetCurrentState()
			Expect(state).ToNot(BeEmpty())
			Expect(reason).ToNot(BeEmpty())
		})
	})
})
