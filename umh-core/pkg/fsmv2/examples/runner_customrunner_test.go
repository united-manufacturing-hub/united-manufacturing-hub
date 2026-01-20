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

package examples_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
)

// TODO; is this still required?
var _ = Describe("Runner CustomRunner Support", func() {
	var ctx context.Context
	var cancel context.CancelFunc
	var logger *zap.SugaredLogger

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		logger = zap.NewNop().Sugar()
	})

	AfterEach(func() {
		cancel()
	})

	It("uses CustomRunner when provided", func() {
		customCalled := false
		testScenario := examples.Scenario{
			Name:        "custom-test",
			Description: "Test custom runner",
			CustomRunner: func(runCtx context.Context, cfg examples.RunConfig) (*examples.RunResult, error) {
				customCalled = true
				done := make(chan struct{})
				close(done)

				return &examples.RunResult{Done: done}, nil
			},
		}

		result, err := examples.Run(ctx, examples.RunConfig{
			Scenario: testScenario,
			Duration: 1 * time.Second,
			Logger:   logger,
			Store:    examples.SetupStore(logger),
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(customCalled).To(BeTrue())
		Eventually(result.Done).Should(BeClosed())
	})

	It("falls through to ApplicationSupervisor when CustomRunner is nil", func() {
		// Use a context with timeout to ensure the supervisor stops
		shortCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		result, err := examples.Run(shortCtx, examples.RunConfig{
			Scenario: examples.SimpleScenario,
			Duration: 1 * time.Second,
			Logger:   logger,
			Store:    examples.SetupStore(logger),
		})

		Expect(err).NotTo(HaveOccurred())
		// Call Shutdown to stop the supervisor gracefully
		result.Shutdown()
		Eventually(result.Done, 5*time.Second).Should(BeClosed())
	})

	It("propagates CustomRunner errors", func() {
		expectedErr := errors.New("custom runner error")
		testScenario := examples.Scenario{
			Name: "error-test",
			CustomRunner: func(runCtx context.Context, cfg examples.RunConfig) (*examples.RunResult, error) {
				return nil, expectedErr
			},
		}

		_, err := examples.Run(ctx, examples.RunConfig{
			Scenario: testScenario,
			Duration: 1 * time.Second,
			Logger:   logger,
			Store:    examples.SetupStore(logger),
		})

		Expect(err).To(Equal(expectedErr))
	})

	It("passes RunConfig to CustomRunner", func() {
		var receivedCfg examples.RunConfig
		testScenario := examples.Scenario{
			Name: "config-test",
			CustomRunner: func(runCtx context.Context, cfg examples.RunConfig) (*examples.RunResult, error) {
				receivedCfg = cfg
				done := make(chan struct{})
				close(done)

				return &examples.RunResult{Done: done}, nil
			},
		}

		expectedDuration := 5 * time.Second
		_, err := examples.Run(ctx, examples.RunConfig{
			Scenario: testScenario,
			Duration: expectedDuration,
			Logger:   logger,
			Store:    examples.SetupStore(logger),
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(receivedCfg.Duration).To(Equal(expectedDuration))
		Expect(receivedCfg.Logger).To(Equal(logger))
	})

	It("returns error when neither YAMLConfig nor CustomRunner is set", func() {
		testScenario := examples.Scenario{
			Name:        "empty-test",
			Description: "Has neither YAML nor CustomRunner",
		}

		_, err := examples.Run(ctx, examples.RunConfig{
			Scenario: testScenario,
			Duration: 1 * time.Second,
			Logger:   logger,
			Store:    examples.SetupStore(logger),
		})

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("neither YAMLConfig nor CustomRunner"))
	})

	It("returns error when both YAMLConfig and CustomRunner are set", func() {
		testScenario := examples.Scenario{
			Name:       "conflict-test",
			YAMLConfig: "some: yaml",
			CustomRunner: func(ctx context.Context, cfg examples.RunConfig) (*examples.RunResult, error) {
				return nil, nil
			},
		}

		_, err := examples.Run(ctx, examples.RunConfig{
			Scenario: testScenario,
			Duration: 1 * time.Second,
			Logger:   logger,
			Store:    examples.SetupStore(logger),
		})

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("both YAMLConfig and CustomRunner"))
	})
})
