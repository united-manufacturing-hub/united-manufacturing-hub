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

package control

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/starvationchecker"
)

// Generates defective configurations
func generateDefectiveConfig() config.FullConfig {
	// Create a local random generator
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	defects := []func(config.FullConfig) config.FullConfig{
		// Empty services
		func(cfg config.FullConfig) config.FullConfig {
			cfg.Internal.Services = []config.S6FSMConfig{}
			return cfg
		},
		// Service with empty name
		func(cfg config.FullConfig) config.FullConfig {
			cfg.Internal.Services = []config.S6FSMConfig{{
				FSMInstanceConfig: config.FSMInstanceConfig{
					Name:            "",
					DesiredFSMState: "running",
				},
			}}
			return cfg
		},
		// Service with invalid desired state
		func(cfg config.FullConfig) config.FullConfig {
			cfg.Internal.Services = []config.S6FSMConfig{{
				FSMInstanceConfig: config.FSMInstanceConfig{
					Name:            "test-service",
					DesiredFSMState: "invalid-state",
				},
			}}
			return cfg
		},
		// Multiple services with same name
		func(cfg config.FullConfig) config.FullConfig {
			cfg.Internal.Services = []config.S6FSMConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            "duplicate-service",
						DesiredFSMState: "running",
					},
				},
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            "duplicate-service",
						DesiredFSMState: "stopped",
					},
				},
			}
			return cfg
		},
	}

	// Choose a random defect
	defectIdx := rng.Intn(len(defects))
	return defects[defectIdx](config.FullConfig{})
}

var _ = Describe("ControlLoop", func() {
	var (
		controlLoop     *ControlLoop
		mockManager     *fsm.MockFSMManager
		mockConfig      *config.MockConfigManager
		mockSvcRegistry *serviceregistry.Registry
		ctx             context.Context
		cancel          context.CancelFunc
		tick            uint64
	)

	BeforeEach(func() {
		mockManager = fsm.NewMockFSMManager()
		mockConfig = config.NewMockConfigManager()
		mockSvcRegistry = serviceregistry.NewMockRegistry()

		// Set up a context with timeout
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)

		starvationChecker := starvationchecker.NewStarvationChecker(constants.StarvationThreshold)

		// Initialize control loop with mocks
		controlLoop = &ControlLoop{
			tickerTime:        100 * time.Millisecond,
			managers:          []fsm.FSMManager[any]{mockManager},
			configManager:     mockConfig,
			logger:            logger.For(logger.ComponentControlLoop),
			starvationChecker: starvationChecker,
			services:          mockSvcRegistry,
		}
		tick = uint64(0)
	})

	AfterEach(func() {
		cancel()
	})

	Describe("Creating a new control loop", func() {
		It("should set default values", func() {
			loop := NewControlLoop(mockConfig)
			Expect(loop).NotTo(BeNil())
			Expect(loop.tickerTime).To(Equal(constants.DefaultTickerTime))
			Expect(loop.managers).To(HaveLen(7))
			Expect(loop.configManager).NotTo(BeNil())
		})
	})

	Describe("Reconcile", func() {
		It("should fetch config and call manager's reconcile", func() {
			expectedConfig := config.FullConfig{
				Internal: config.InternalConfig{
					Services: []config.S6FSMConfig{
						{
							FSMInstanceConfig: config.FSMInstanceConfig{
								Name:            "test-service",
								DesiredFSMState: "running",
							},
						},
					},
				},
			}
			mockConfig.Config = expectedConfig

			err := controlLoop.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(mockConfig.GetConfigCalled).To(BeTrue())
			Expect(mockManager.ReconcileCalled).To(BeTrue())
		})

		It("should not return error if config manager returns error", func() { // config manager should go into backoff
			mockConfig.ConfigError = errors.New("config error")

			err := controlLoop.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(mockConfig.GetConfigCalled).To(BeTrue())
			Expect(mockManager.ReconcileCalled).To(BeFalse())
		})

		It("should return error if manager returns error", func() {
			mockManager.ReconcileError = errors.New("reconcile error")

			err := controlLoop.Reconcile(ctx, 0)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("manager MockFSMManager reconciliation failed: reconcile error"))
			Expect(mockConfig.GetConfigCalled).To(BeTrue())
			Expect(mockManager.ReconcileCalled).To(BeTrue())
		})

		It("should respect context cancellation", func() {
			// Create a context that's already canceled
			canceledCtx, cancelFunc := context.WithCancel(context.Background())
			cancelFunc()

			// Add delays to ensure cancellation takes effect
			mockConfig.ConfigDelay = 50 * time.Millisecond

			err := controlLoop.Reconcile(canceledCtx, tick)
			tick++
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context canceled"))
		})
	})

	Describe("Execute", func() {
		It("should call Reconcile repeatedly until context is cancelled", func() {
			// Create a tracking config manager that we can use to monitor calls
			trackingConfig := config.NewMockConfigManager().WithConfig(config.FullConfig{})
			starvationChecker := starvationchecker.NewStarvationChecker(constants.StarvationThreshold)

			// We'll create a new control loop specifically for this test
			testLoop := &ControlLoop{
				tickerTime:        5 * time.Millisecond, // Fast ticker for tests
				managers:          []fsm.FSMManager[any]{fsm.NewMockFSMManager()},
				configManager:     trackingConfig,
				starvationChecker: starvationChecker,
				logger:            logger.For(logger.ComponentControlLoop),
				services:          mockSvcRegistry,
			}

			// Use an atomic counter to track calls safely
			var callCount int32

			// Create a channel to signal when we've seen enough calls
			enoughCalls := make(chan struct{})

			// Set up a goroutine to monitor GetConfigCalled changes
			go func() {
				// Wait until we've seen at least 2 calls
				for {
					if trackingConfig.GetConfigCalled {
						atomic.AddInt32(&callCount, 1)
						trackingConfig.ResetCalls() // Reset for next call detection

						// If we've counted at least 2 calls, signal and exit monitoring
						if atomic.LoadInt32(&callCount) >= 2 {
							enoughCalls <- struct{}{}
							return
						}
					}
					time.Sleep(1 * time.Millisecond) // Small sleep to avoid tight loop
				}
			}()

			// Start executing in a goroutine
			execDone := make(chan error)
			go func() {
				execDone <- testLoop.Execute(ctx)
			}()

			// Wait for enough calls or timeout
			select {
			case <-enoughCalls:
				// We've seen enough calls, good
			case <-time.After(100 * time.Millisecond):
				Fail("Timed out waiting for reconcile calls")
			}

			// Cancel the context
			cancel()

			// Wait for Execute to finish
			err := <-execDone
			Expect(err).NotTo(HaveOccurred())
			Expect(atomic.LoadInt32(&callCount)).To(BeNumerically(">=", 2))
		})

		It("should stop execution if Reconcile returns a non-timeout error", func() {
			mockManager.ReconcileError = errors.New("reconcile error")

			// Start executing in a goroutine
			execDone := make(chan error)
			go func() {
				execDone <- controlLoop.Execute(ctx)
			}()

			// Wait for Execute to finish with error
			err := <-execDone
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("manager MockFSMManager reconciliation failed: reconcile error"))
		})

		It("should continue execution if Reconcile returns a context timeout error", func() {
			// Set up a mock with context deadline exceeded error
			timeoutConfig := config.NewMockConfigManager()
			timeoutConfig.WithConfigError(context.DeadlineExceeded)

			// Track calls to verify the loop continues
			var callCount int32

			starvationChecker := starvationchecker.NewStarvationChecker(constants.StarvationThreshold)

			// Create a control loop with this config
			timeoutLoop := &ControlLoop{
				tickerTime:        5 * time.Millisecond,
				managers:          []fsm.FSMManager[any]{fsm.NewMockFSMManager()},
				configManager:     timeoutConfig,
				logger:            logger.For(logger.ComponentControlLoop),
				starvationChecker: starvationChecker,
				services:          mockSvcRegistry,
			}

			// Start executing in a goroutine
			execDone := make(chan error)
			go func() {
				execDone <- timeoutLoop.Execute(ctx)
			}()

			// Wait a bit to ensure the timeout error is encountered
			time.Sleep(20 * time.Millisecond)

			// Change the error to nil on the first call to track we got past the error
			atomic.AddInt32(&callCount, 1)
			timeoutConfig.WithConfigError(nil)

			// Create a channel to detect if execution continues
			continuedExecution := make(chan struct{})

			// Set up a goroutine to monitor calls
			go func() {
				// Wait for the GetConfigCalled to be true again after we cleared the error
				for i := 0; i < 50; i++ { // Try for ~50ms
					// If GetConfigCalled becomes true after we cleared the error
					if timeoutConfig.GetConfigCalled {
						atomic.AddInt32(&callCount, 1)
						continuedExecution <- struct{}{}
						return
					}
					time.Sleep(1 * time.Millisecond)
					timeoutConfig.ResetCalls()
				}
			}()

			// Wait for continued execution or timeout
			select {
			case <-continuedExecution:
				// Loop continued after timeout, which is what we want
			case <-time.After(100 * time.Millisecond):
				Fail("Timed out waiting for the loop to continue after timeout error")
			}

			// Cancel the context to end the test
			cancel()

			// Wait for Execute to finish without error
			err := <-execDone
			Expect(err).NotTo(HaveOccurred())
			Expect(atomic.LoadInt32(&callCount)).To(BeNumerically(">", 1))
		})
	})

	Describe("Stop", func() {
		It("should cancel the context", func() {
			err := controlLoop.Stop(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Fuzz Testing", func() {
		// This randomizes the behavior of the control loop
		FuzzRandomDelays := func() {
			// Create a local random generator
			rng := rand.New(rand.NewSource(time.Now().UnixNano()))

			// Set up random delays in mocks
			mockConfig.ConfigDelay = time.Duration(rng.Intn(30)) * time.Millisecond
			mockManager.ReconcileDelay = time.Duration(rng.Intn(30)) * time.Millisecond

			// Use a context with longer timeout for random delays
			fuzzCtx, fuzzCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer fuzzCancel()

			err := controlLoop.Reconcile(fuzzCtx, tick)
			tick++

			// Check if error is context cancellation or timing out
			if err != nil {
				Expect(err.Error()).To(Or(
					ContainSubstring("context deadline exceeded"),
					ContainSubstring("context canceled"),
				))
			}
		}

		It("should handle random timing delays", func() {
			// Run multiple iterations with random delays
			for i := 0; i < 10; i++ {
				// Create a local random generator for each iteration
				mockManager.ResetCalls()
				mockConfig.ResetCalls()

				FuzzRandomDelays()
			}
		})

		It("should handle filesystem failures gracefully", func() {
			// Run multiple iterations with different failure patterns
			for i := 0; i < 10; i++ {
				// Create a new mockFS for each iteration with random failure characteristics
				mockFS := filesystem.NewMockFileSystem().
					WithFailureRate(0.3). // 30% chance of failure
					WithDelayRange(30 * time.Millisecond)

				// Set up a config manager that uses the mock file system
				fileConfigManager := config.NewFileConfigManager()
				fileConfigManager.WithFileSystemService(mockFS)

				// Replace the control loop's config manager
				controlLoop.configManager = fileConfigManager

				// Use a context with longer timeout for file system operations
				fuzzCtx, fuzzCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)

				// Run reconcile and expect potential errors
				err := controlLoop.Reconcile(fuzzCtx, tick)
				tick++

				// Clean up
				fuzzCancel()

				// We're not asserting specific outcomes because we want to simulate chaos
				// Just make sure the system doesn't panic
				if err != nil {
					GinkgoWriter.Println("Fuzz error:", err.Error())
				}
			}
		})

		FuzzDefectiveConfigs := func() {
			// Generate a defective config
			defectiveConfig := generateDefectiveConfig()
			mockConfig.Config = defectiveConfig

			// Set up a context with timeout
			fuzzCtx, fuzzCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer fuzzCancel()

			// Run reconcile and observe behavior
			err := controlLoop.Reconcile(fuzzCtx, tick)
			tick++

			// We're not expecting specific outcomes in a fuzz test
			// Just ensure the system handles bad configs without panicking
			if err != nil {
				GinkgoWriter.Println("Defective config error:", err.Error())
			}
		}

		It("should handle defective configurations", func() {
			// Run multiple iterations with different defective configs
			for i := 0; i < 10; i++ {
				mockManager.ResetCalls()

				FuzzDefectiveConfigs()
			}
		})

		// Combination fuzz test for maximum chaos
		It("should handle combined failures, delays, and bad configs", func() {
			// Create a more complex test environment
			complexCtx, complexCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer complexCancel()

			for i := 0; i < 5; i++ {
				// Create a local random generator for each iteration
				rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(i)))

				// Mix of defective configs, file system failures, and delays
				mockConfig.Config = generateDefectiveConfig()
				mockConfig.ConfigDelay = time.Duration(rng.Intn(30)) * time.Millisecond
				mockManager.ReconcileDelay = time.Duration(rng.Intn(30)) * time.Millisecond

				// Add random manager failures
				if rng.Float64() < 0.3 {
					mockManager.ReconcileError = fmt.Errorf("random manager error #%d", i)
				} else {
					mockManager.ReconcileError = nil
				}

				// Run the control loop
				err := controlLoop.Reconcile(complexCtx, tick)
				tick++
				if err != nil {
					GinkgoWriter.Println("Complex fuzz error:", err.Error())
				}
			}
		})
	})
})
