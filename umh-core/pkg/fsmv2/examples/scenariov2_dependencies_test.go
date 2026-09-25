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
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
)

var _ = Describe("ScenarioV2 Dependencies failure", func() {
	It("fails the run before the supervisor starts when Dependencies returns an error, naming the scenario", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		depsErr := errors.New("dependency setup failed")
		runRan := false
		failing := examples.ScenarioV2{
			Name:        "deps-error",
			Description: "test-local Run for the Dependencies error path",
			Dependencies: func() (map[string]any, func(), error) {
				return nil, nil, depsErr
			},
			Run: func(_ context.Context, _ examples.Env) error {
				runRan = true

				// Returning an error makes the runner tear down at once. If a
				// regression reaches Run, no supervisor outlives this spec.
				return errors.New("Run must not be reached when Dependencies failed")
			},
		}

		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)

		result, err := examples.Run(context.Background(), examples.RunConfig{
			ScenarioV2:   failing,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).To(MatchError(depsErr),
			"the runner must propagate the Dependencies error")
		Expect(err.Error()).To(ContainSubstring("deps-error"),
			"the error must name the scenario whose dependencies failed")
		Expect(result).To(BeNil(),
			"a failed Dependencies must not return a run result")
		Expect(runRan).To(BeFalse(),
			"the run must fail before the scenario's Run is invoked")
		Expect(register.GlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(BeNil(),
			"a failed Dependencies must not leave the configworker deps key behind")
	})
})

var _ = Describe("ScenarioV2 Dependencies cleanup", func() {
	It("calls the cleanup exactly once after the supervisor stopped following a normal run", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		var cleanupCalls atomic.Int32

		normal := examples.ScenarioV2{
			Name:        "cleanup-normal",
			Description: "test-local Run for the cleanup path after a normal run",
			Dependencies: func() (map[string]any, func(), error) {
				return nil, func() {
					cleanupCalls.Add(1)
				}, nil
			},
			Run: func(_ context.Context, _ examples.Env) error {
				return nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   normal,
			Duration:     10 * time.Second,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(cleanupCalls.Load()).To(Equal(int32(0)),
			"the cleanup must not run while the run's Duration is still elapsing")

		Eventually(result.Done, "55s").Should(BeClosed(),
			"the normal run must finish its Duration and tear down on its own")

		Expect(cleanupCalls.Load()).To(Equal(int32(1)),
			"the cleanup must run exactly once after the supervisor stopped")
	})

	It("calls the cleanup exactly once when Run returns an error", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		runErr := errors.New("boom")

		var cleanupCalls atomic.Int32

		failing := examples.ScenarioV2{
			Name:        "cleanup-error",
			Description: "test-local Run for the cleanup path after a failing Run",
			Dependencies: func() (map[string]any, func(), error) {
				return nil, func() {
					cleanupCalls.Add(1)
				}, nil
			},
			Run: func(_ context.Context, _ examples.Env) error {
				return runErr
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		_, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   failing,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).To(MatchError(runErr),
			"the runner must propagate the Run error")

		Expect(cleanupCalls.Load()).To(Equal(int32(1)),
			"the cleanup must run exactly once before the Run error is returned")
	})

	It("calls the cleanup exactly once when Run panics", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		var cleanupCalls atomic.Int32

		panicking := examples.ScenarioV2{
			Name:        "cleanup-panic",
			Description: "test-local Run for the cleanup path after a panicking Run",
			Dependencies: func() (map[string]any, func(), error) {
				return nil, func() {
					cleanupCalls.Add(1)
				}, nil
			},
			Run: func(_ context.Context, _ examples.Env) error {
				panic("Run exploded")
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		Expect(func() {
			_, _ = examples.Run(ctx, examples.RunConfig{
				ScenarioV2:   panicking,
				TickInterval: 50 * time.Millisecond,
				Logger:       logger,
				Store:        store,
			})
		}).To(PanicWith("Run exploded"),
			"the runner must not swallow a Run panic")

		Expect(cleanupCalls.Load()).To(Equal(int32(1)),
			"the cleanup must run exactly once while the panic unwinds")
	})

	It("runs a scenario whose Dependencies returns no cleanup", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		cleanupFree := examples.ScenarioV2{
			Name:        "nil-cleanup",
			Description: "test-local Run for the nil-cleanup path",
			Dependencies: func() (map[string]any, func(), error) {
				return nil, nil, nil
			},
			Run: func(_ context.Context, _ examples.Env) error {
				return nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   cleanupFree,
			Duration:     time.Second,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred(),
			"a nil cleanup must not break the run")
		Eventually(result.Done, "55s").Should(BeClosed())
	})
})
