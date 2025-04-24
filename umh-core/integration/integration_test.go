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
// integration_test.go

package integration_test

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ---------- Actual Ginkgo Tests ----------

// Note: Redpanda allocates 2GB of memory per core.
// Additionally 1.5GB (or 7% of the total memory, whichever is greater) are required to be available, after this allocation to make seastar happy.
// If you change this, you need to update the Makefile.
const DEFAULT_MEMORY = "4096m"
const DEFAULT_CPUS = 2

var _ = Describe("UMH Container Integration", Ordered, Label("integration"), func() {

	AfterAll(func() {
		// Always stop container after the entire suite
		PrintLogsAndStopContainer()
		CleanupDockerBuildCache()

		//Keep temp dirs for debugging if the test failed
		if !CurrentSpecReport().Failed() {
			cleanupTmpDirs(containerName)
		}
	})

	Context("with an empty config", func() {
		BeforeAll(func() {
			By("Building an empty config and writing to data/config.yaml")
			// Create a config builder and ensure the metrics port is 8080 (container internal default)
			configBuilder := NewBuilder().SetMetricsPort(8080)
			emptyConfig := configBuilder.BuildYAML()

			Expect(writeConfigFile(emptyConfig)).To(Succeed())
			err := BuildAndRunContainer(emptyConfig, DEFAULT_MEMORY, DEFAULT_CPUS)
			if err != nil {
				// If container startup fails, print detailed debug info
				fmt.Println("Container startup failed, printing debug info:")
				printContainerDebugInfo()
				Expect(err).ToNot(HaveOccurred(), "Container startup failed")
			}
			Expect(waitForMetrics()).To(Succeed(), "Metrics endpoint should be available with empty config")
		})
		It("exposes metrics and has zero s6 services running", func() {
			// Check the /metrics endpoint
			Eventually(func() bool {
				resp, err := http.Get(GetMetricsURL())
				if err != nil {
					return false
				}
				defer func() {
					if err := resp.Body.Close(); err != nil {
						Fail(fmt.Sprintf("Error closing response body: %v\n", err))
					}
				}()

				if resp.StatusCode != http.StatusOK {
					return false
				}

				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return false
				}

				return strings.Contains(string(body), "umh_core_reconcile_duration_milliseconds")
			}, 10*time.Second, 1*time.Second).Should(BeTrue(), "Metrics endpoint should contain the expected metrics")
		})
	})

	Context("with a golden service config", func() {
		BeforeAll(func() {
			By("Building a config with the golden service and writing to data/config.yaml")
			cfg := NewBuilder().
				AddGoldenService().
				BuildYAML()

			Expect(writeConfigFile(cfg)).To(Succeed())
			Expect(BuildAndRunContainer(cfg, DEFAULT_MEMORY, DEFAULT_CPUS)).To(Succeed())
			Expect(waitForMetrics()).To(Succeed(), "Metrics endpoint should be available with golden service config")
		})

		It("should have the golden service up and expose metrics", func() {
			// Check /metrics
			Eventually(func() bool {
				resp, err := http.Get(GetMetricsURL())
				if err != nil {
					return false
				}
				defer func() {
					if err := resp.Body.Close(); err != nil {
						Fail(fmt.Sprintf("Error closing response body: %v\n", err))
					}
				}()

				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return false
				}

				return strings.Contains(string(body), "umh_core_reconcile_duration_milliseconds")
			}, 10*time.Second, 1*time.Second).Should(BeTrue(), "Metrics endpoint should contain the expected metrics")

			Eventually(func() int {
				return checkGoldenServiceStatusOnly()
			}, 10*time.Second, 1*time.Second).Should(Equal(200),
				"Golden service should respond with 200 OK on the mapped port")
		})
	})

	Context("with multiple services (golden + a 'sleep' service)", func() {
		BeforeAll(func() {
			By("Building a configuration with the golden service and a sleep service")
			cfg := NewBuilder().
				AddGoldenService().
				AddSleepService("sleepy", "600").
				BuildYAML()

			// Write the config and start the container with the new configuration.
			Expect(writeConfigFile(cfg)).To(Succeed())
			Expect(BuildAndRunContainer(cfg, DEFAULT_MEMORY, DEFAULT_CPUS)).To(Succeed())
			Expect(waitForMetrics()).To(Succeed(), "Metrics endpoint should be available with golden + sleep service config")

			// Verify that the golden service is ready
			Eventually(func() int {
				return checkGoldenServiceStatusOnly()
			}, 10*time.Second, 1*time.Second).Should(Equal(200),
				"Golden service should respond with 200 OK on the mapped port")
			GinkgoWriter.Println("Golden service is up and running")
		})

		It("should have both services active and expose healthy metrics", func() {
			By("Verifying the metrics endpoint contains expected metrics")
			Eventually(func() bool {
				resp, err := http.Get(GetMetricsURL())
				if err != nil {
					return false
				}
				defer func() {
					if err := resp.Body.Close(); err != nil {
						Fail(fmt.Sprintf("Error closing response body: %v\n", err))
					}
				}()

				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return false
				}

				return strings.Contains(string(body), "umh_core_reconcile_duration_milliseconds")
			}, 10*time.Second, 1*time.Second).Should(BeTrue(), "Metrics endpoint should contain the expected metrics")

			By("Verifying that the golden service is returning 200 OK")
			Eventually(func() int {
				return checkGoldenServiceStatusOnly()
			}, 10*time.Second, 1*time.Second).Should(Equal(200),
				"Golden service should respond with 200 OK on the mapped port")
		})
	})

	Context("with service scaling test", Label("scaling"), func() {

		BeforeAll(func() {
			By("Starting with an empty configuration")
			cfg := NewBuilder().BuildYAML()
			// Write the empty config and start the container
			Expect(writeConfigFile(cfg)).To(Succeed())
			Expect(BuildAndRunContainer(cfg, DEFAULT_MEMORY, DEFAULT_CPUS)).To(Succeed())
			Expect(waitForMetrics()).To(Succeed(), "Metrics endpoint should be available with empty config")
		})

		It("should scale up to multiple services while maintaining healthy metrics", func() {
			By("Adding the golden service as a baseline")
			// Build configuration with the golden service first
			builder := NewBuilder().AddGoldenService()
			cfg := builder.BuildYAML()
			Expect(writeConfigFile(cfg, getContainerName())).To(Succeed())

			By("Waiting for the golden service to become responsive")
			Eventually(func() int {
				return checkGoldenServiceStatusOnly()
			}, 20*time.Second, 1*time.Second).Should(Equal(200),
				"Golden service should respond with 200 OK")

			By("Scaling up by adding 10 sleep services")
			// Add 10 sleep services to the configuration
			for i := 0; i < 10; i++ {
				serviceName := fmt.Sprintf("sleepy-%d", i)
				builder.AddSleepService(serviceName, "600")
				cfg = builder.BuildYAML()
				GinkgoWriter.Printf("Added service %s\n", serviceName)
				Expect(writeConfigFile(cfg, getContainerName())).To(Succeed())
			}

			By("Simulating random stop/start actions on sleep services (chaos monkey)")
			// Create a deterministic random number generator for reproducibility
			r := rand.New(rand.NewSource(42))
			for i := 0; i < 100; i++ {
				// Pick a random sleep service index (0-9)
				randomIndex := r.Intn(10)
				randomServiceName := fmt.Sprintf("sleepy-%d", randomIndex)

				// Randomly decide to start or stop the service
				action := "start"
				if r.Float64() < 0.5 {
					action = "stop"
					builder.StopService(randomServiceName)
				} else {
					builder.StartService(randomServiceName)
				}
				GinkgoWriter.Printf("Chaos monkey: %sing service %s\n", action, randomServiceName)
				// Apply the updated configuration
				Expect(writeConfigFile(builder.BuildYAML(), getContainerName())).To(Succeed())

				// Random delay between operations
				delay := time.Duration(100+r.Intn(500)) * time.Millisecond

				// Check the health of the system
				monitorHealth()
				time.Sleep(delay)
			}

			GinkgoWriter.Println("Scaling test completed successfully")
		})
	})

	Context("with comprehensive chaos test", Label("chaos"), func() {

		BeforeAll(func() {
			//Skip("Skipping comprehensive chaos test due to time constraints")
			// Start with an empty config
			cfg := NewBuilder().BuildYAML()
			Expect(writeConfigFile(cfg)).To(Succeed())
			Expect(BuildAndRunContainer(cfg, DEFAULT_MEMORY, DEFAULT_CPUS)).To(Succeed())
			Expect(waitForMetrics()).To(Succeed(), "Metrics endpoint should be available with empty config")
		})

		It("should handle random service additions, deletions, starts and stops", func() {
			// Start monitoring goroutine
			testDuration := 3 * time.Minute

			// Create deterministic random number generator
			r := rand.New(rand.NewSource(42))

			// Add golden service as constant baseline
			builder := NewBuilder().AddGoldenService()
			Expect(writeConfigFile(builder.BuildYAML(), getContainerName())).To(Succeed())

			// Wait for golden service to be ready
			Eventually(func() int {
				return checkGoldenServiceStatusOnly()
			}, 20*time.Second, 1*time.Second).Should(Equal(200),
				"Golden service should respond with 200 OK")

			GinkgoWriter.Println("Starting comprehensive chaos test")

			// Track existing services (with their current state)
			existingServices := map[string]string{} // serviceName -> state ("running"/"stopped")
			// Add tracking of sleep durations
			serviceDurations := map[string]string{} // serviceName -> sleep duration
			maxServices := 100
			bulkSize := 5 + r.Intn(6) // Random bulk size between 5-10 services

			// ---------- WARMUP PHASE: Build up to target service count ----------
			targetServiceCount := 100
			warmupBatchSize := 20
			warmupDelay := 15 * time.Second

			GinkgoWriter.Printf("\n=== WARMUP PHASE ===\nBuilding up to %d services\n", targetServiceCount)

			// Initial warmup: Add services in batches until we reach the target
			for len(existingServices) < targetServiceCount {
				batchSize := min(warmupBatchSize, targetServiceCount-len(existingServices))

				builder := NewBuilder().AddGoldenService()

				// Add existing services with their original sleep durations
				for svc, state := range existingServices {
					duration := serviceDurations[svc]
					if state == "running" {
						builder.AddSleepService(svc, duration)
					} else {
						builder.AddSleepService(svc, duration)
						builder.StopService(svc)
					}
				}

				// Add new batch of services
				GinkgoWriter.Printf("Adding batch of %d services\n", batchSize)
				for i := 0; i < batchSize; i++ {
					serviceName := fmt.Sprintf("warmup-svc-%d", len(existingServices))
					duration := fmt.Sprintf("%d", 60+r.Intn(600))
					builder.AddSleepService(serviceName, duration)
					existingServices[serviceName] = "running"
					serviceDurations[serviceName] = duration
				}

				// Apply changes
				Expect(writeConfigFile(builder.BuildYAML(), getContainerName())).To(Succeed())

				// Check health
				monitorHealth()

				// Allow system to stabilize after each batch
				GinkgoWriter.Printf("Warmup: Total services now: %d - waiting for stabilization\n", len(existingServices))
				time.Sleep(warmupDelay)
			}

			// Final stabilization after reaching target count
			GinkgoWriter.Printf("\n=== WARMUP COMPLETE ===\nReached %d services - allowing system to stabilize\n", len(existingServices))
			time.Sleep(warmupDelay)

			// ---------- CHAOS PHASE: Begin regular chaos operations ----------
			GinkgoWriter.Println("\n=== CHAOS PHASE BEGINNING ===")

			// Test runs until the duration is reached
			startTime := time.Now()
			actionCount := 0
			lastBulkOperation := time.Now()
			stabilizationPeriod := 10 * time.Second

			for time.Since(startTime) < testDuration {
				actionCount++
				timeSinceLastBulk := time.Since(lastBulkOperation)

				// Always start with a fresh builder containing the golden service
				builder := NewBuilder().AddGoldenService()

				// If we haven't done a bulk operation recently, do one
				if timeSinceLastBulk > stabilizationPeriod {
					// Randomly choose between bulk add or bulk remove
					if r.Float64() < 0.7 || len(existingServices) == 0 { // 70% chance to add, or 100% if no services
						// Bulk add services
						numToAdd := min(bulkSize, maxServices-len(existingServices))
						if numToAdd > 0 {
							GinkgoWriter.Printf("\n=== BULK ADD OPERATION ===\nAdding %d new services\n", numToAdd)
							// Add all existing services first with their original durations
							for svc, state := range existingServices {
								duration := serviceDurations[svc]
								if state == "running" {
									builder.AddSleepService(svc, duration)
								} else {
									builder.AddSleepService(svc, duration)
									builder.StopService(svc)
								}
							}
							// Add new services
							for i := 0; i < numToAdd; i++ {
								serviceName := fmt.Sprintf("bulk-add-%d-%d", actionCount, i)
								duration := fmt.Sprintf("%d", 60+r.Intn(600))
								builder.AddSleepService(serviceName, duration)
								existingServices[serviceName] = "running"
								serviceDurations[serviceName] = duration
							}
							GinkgoWriter.Printf("Bulk add completed. Total services: %d\n", len(existingServices))
							lastBulkOperation = time.Now()
						}
					} else {
						// Bulk remove services
						keys := getKeys(existingServices)
						numToRemove := min(bulkSize, len(keys))
						if numToRemove > 0 {
							GinkgoWriter.Printf("\n=== BULK REMOVE OPERATION ===\nRemoving %d services\n", numToRemove)
							// Choose random services to remove
							indicesToRemove := make(map[int]bool)
							for i := 0; i < numToRemove; i++ {
								for {
									idx := r.Intn(len(keys))
									if !indicesToRemove[idx] {
										indicesToRemove[idx] = true
										break
									}
								}
							}

							// Add all services except those being removed
							for idx, svc := range keys {
								if !indicesToRemove[idx] {
									state := existingServices[svc]
									duration := serviceDurations[svc]
									if state == "running" {
										builder.AddSleepService(svc, duration)
									} else {
										builder.AddSleepService(svc, duration)
										builder.StopService(svc)
									}
								} else {
									delete(existingServices, svc)
									delete(serviceDurations, svc) // Clean up duration tracking too
								}
							}
							GinkgoWriter.Printf("Bulk remove completed. Remaining services: %d\n", len(existingServices))
							lastBulkOperation = time.Now()
						}
					}
				} else {
					// During stabilization period, only do minimal operations
					// Randomly choose between start/stop single service (30% chance)
					if r.Float64() < 0.3 {
						operationType := r.Float64()

						// Find stopped services
						stoppedServices := []string{}
						for svc, state := range existingServices {
							if state == "stopped" {
								stoppedServices = append(stoppedServices, svc)
							}
						}

						if operationType < 0.6 && len(stoppedServices) > 0 {
							// 60% chance to start a stopped service if any are stopped
							serviceToStart := stoppedServices[r.Intn(len(stoppedServices))]
							// Add all services with their current state and original durations
							for svc, state := range existingServices {
								duration := serviceDurations[svc]
								if state == "running" {
									builder.AddSleepService(svc, duration)
								} else {
									builder.AddSleepService(svc, duration)
									builder.StopService(svc)
								}
							}
							builder.StartService(serviceToStart)
							existingServices[serviceToStart] = "running"
							GinkgoWriter.Printf("Starting service %s during stabilization period\n", serviceToStart)
						} else if operationType >= 0.6 {
							// 40% chance to stop a running service if more than 80% are running
							// Collect running services
							runningServices := []string{}
							for svc, state := range existingServices {
								if state == "running" && svc != "golden-service" {
									runningServices = append(runningServices, svc)
								}
							}

							// Only stop a service if we have plenty of running services (>80% of total)
							runningThreshold := int(float64(len(existingServices)) * 0.8)
							if len(runningServices) > runningThreshold && len(runningServices) > 0 {
								serviceToStop := runningServices[r.Intn(len(runningServices))]

								// Add all services with their current state and original durations
								for svc, state := range existingServices {
									duration := serviceDurations[svc]
									if state == "running" {
										builder.AddSleepService(svc, duration)
									} else {
										builder.AddSleepService(svc, duration)
										builder.StopService(svc)
									}
								}

								// Stop the selected service
								builder.StopService(serviceToStop)
								existingServices[serviceToStop] = "stopped"
								GinkgoWriter.Printf("Stopping service %s during stabilization (to maintain ~80%% running)\n", serviceToStop)
							}
						}
					}
				}

				// Apply changes
				cfg := builder.BuildYAML()
				Expect(writeConfigFile(cfg, getContainerName())).To(Succeed())

				// Logging every 5 operations to show current state
				if actionCount%5 == 0 {
					running := countRunningServices(existingServices)
					runningPercentage := float64(running) / float64(len(existingServices)) * 100
					GinkgoWriter.Printf("Config applied, services: %d total, %d running (%.1f%%)\n",
						len(existingServices), running, runningPercentage)
				}

				// Longer delay between operations
				delay := time.Duration(2000+r.Intn(2000)) * time.Millisecond
				if timeSinceLastBulk < stabilizationPeriod {
					// During stabilization, use longer delays
					delay = time.Duration(5000+r.Intn(5000)) * time.Millisecond
				}

				// Check the health of the system
				monitorHealth()
				time.Sleep(delay)

				// Every 20 actions, print a status update
				if actionCount%20 == 0 {
					running := countRunningServices(existingServices)
					elapsedTime := time.Since(startTime).Round(time.Second)
					remainingTime := (testDuration - elapsedTime).Round(time.Second)
					timeUntilNextBulk := (stabilizationPeriod - timeSinceLastBulk).Round(time.Second)
					GinkgoWriter.Printf("\n=== Chaos Test Status ===\n"+
						"Actions completed: %d\n"+
						"Total services: %d\n"+
						"Running services: %d\n"+
						"Time elapsed: %v\n"+
						"Time remaining: %v\n"+
						"Time until next bulk operation: %v\n",
						actionCount, len(existingServices), running,
						elapsedTime, remainingTime, timeUntilNextBulk)
				}
			}

			GinkgoWriter.Printf("Chaos test actions completed (%d total actions), waiting for monitoring to complete\n", actionCount)
			GinkgoWriter.Println("Chaos test completed successfully")
		})
	})

	Context("with benthos scaling test", Label("benthos-scaling"), func() {

		BeforeAll(func() {
			By("Starting with an empty configuration")
			cfg := NewBuilder().BuildYAML()
			// Write the empty config and start the container
			Expect(writeConfigFile(cfg)).To(Succeed())
			Expect(BuildAndRunContainer(cfg, DEFAULT_MEMORY, DEFAULT_CPUS)).To(Succeed())
			Expect(waitForMetrics()).To(Succeed(), "Metrics endpoint should be available with empty config")
		})

		AfterAll(func() {
			By("Stopping the container after the benthos scaling test")
		})

		It("should scale up to multiple benthos instances while maintaining stability", func() {
			By("Adding the golden service as a baseline")
			builder := NewBenthosBuilder()
			builder.AddGoldenBenthos()
			cfg := builder.BuildYAML()
			Expect(writeConfigFile(cfg, getContainerName())).To(Succeed())

			By("Waiting for the golden service to become responsive")
			Eventually(func() int {
				return checkGoldenServiceStatusOnly()
			}, 20*time.Second, 1*time.Second).Should(Equal(200),
				"Golden service should respond with 200 OK")

			By("Scaling up by adding 10 benthos generator services")
			// Add 10 benthos services to the configuration
			for i := 0; i < 10; i++ {
				serviceName := fmt.Sprintf("benthos-%d", i)
				builder.AddGeneratorBenthos(serviceName, fmt.Sprintf("%ds", 1+i%3)) // Varying intervals
				cfg = builder.BuildYAML()
				GinkgoWriter.Printf("Added benthos service %s\n", serviceName)
				Expect(writeConfigFile(cfg, getContainerName())).To(Succeed())

				// Allow time for service to start before adding the next one
				time.Sleep(1 * time.Second)

				// Check health periodically
				monitorHealth()
			}

			By("Simulating random stop/start/update actions on benthos services")
			// Create a deterministic random number generator for reproducibility
			r := rand.New(rand.NewSource(42))
			for i := 0; i < 50; i++ {
				// Pick a random benthos service index (0-9)
				randomIndex := r.Intn(10)
				randomServiceName := fmt.Sprintf("benthos-%d", randomIndex)

				// Randomly decide operation: start, stop, or update
				opType := r.Intn(3)
				switch opType {
				case 0: // Start
					GinkgoWriter.Printf("Starting benthos service %s\n", randomServiceName)
					builder.StartBenthos(randomServiceName)
				case 1: // Stop
					GinkgoWriter.Printf("Stopping benthos service %s\n", randomServiceName)
					builder.StopBenthos(randomServiceName)
				case 2: // Update config
					newInterval := fmt.Sprintf("%ds", 1+r.Intn(5))
					GinkgoWriter.Printf("Updating benthos service %s with new interval %s\n", randomServiceName, newInterval)
					builder.UpdateGeneratorBenthos(randomServiceName, newInterval)
				}

				// Apply the updated configuration
				Expect(writeConfigFile(builder.BuildYAML(), getContainerName())).To(Succeed())

				// Random delay between operations
				delay := time.Duration(1000+r.Intn(2000)) * time.Millisecond
				time.Sleep(delay)

				// Check the health of the system
				monitorHealth()

				// Every 10 operations, print a status update
				if i%10 == 0 {
					activeCount := builder.CountActiveBenthos()
					GinkgoWriter.Printf("\n=== Benthos Scaling Test Status ===\n"+
						"Actions completed: %d\n"+
						"Total benthos services: %d\n"+
						"Active benthos services: %d\n",
						i+1, 11, activeCount) // 11 includes golden service
				}
			}

			GinkgoWriter.Println("Benthos scaling test completed successfully")
		})
	})

	Context("with redpanda enabled but no benthos services", Label("redpanda-only"), func() {
		BeforeAll(func() {
			By("Starting with an empty configuration")
			cfg := NewBuilder().BuildYAML()
			// Write the empty config and start the container
			Expect(writeConfigFile(cfg)).To(Succeed())
			Expect(BuildAndRunContainer(cfg, DEFAULT_MEMORY, DEFAULT_CPUS)).To(Succeed())
			Expect(waitForMetrics()).To(Succeed(), "Metrics endpoint should be available with empty config")
		})

		AfterAll(func() {
			By("Stopping the container after the redpanda-only test")
			PrintLogsAndStopContainer()
		})

		It("should run redpanda without errors when no benthos services are configured", func() {
			By("Adding a golden service as baseline")
			builder := NewRedpandaBuilder()
			builder.AddGoldenRedpanda()
			cfg := builder.BuildYAML()
			Expect(writeConfigFile(cfg, getContainerName())).To(Succeed())

			By("Waiting for the metrics endpoint to be healthy")
			Eventually(func() bool {
				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
				defer cancel()
				req, err := http.NewRequestWithContext(ctx, "GET", GetMetricsURL(), nil)
				if err != nil {
					return false
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return false
				}
				defer func() {
					if err := resp.Body.Close(); err != nil {
						Fail(fmt.Sprintf("Error closing response body: %v\n", err))
					}
				}()
				return resp.StatusCode == http.StatusOK
			}, 20*time.Second, 1*time.Second).Should(BeTrue(),
				"Metrics endpoint should be healthy")

			By("Verifying the system is stable for 30 seconds")
			startTime := time.Now()
			for time.Since(startTime) < 30*time.Second {
				// Check metrics endpoint is healthy
				failOnMetricsHealthIssue()

				// Check for any warning or error logs
				out, err := runDockerCommand("logs", getContainerName())
				Expect(err).NotTo(HaveOccurred(), "Should be able to retrieve container logs")

				// Count warnings and errors
				warningCount := 0
				errorCount := 0
				for _, line := range strings.Split(out, "\n") {
					if strings.Contains(line, "[WARN]") {
						warningCount++
					}
					if strings.Contains(line, "[ERROR]") {
						errorCount++
					}
				}

				GinkgoWriter.Printf("System status: Warnings: %d, Errors: %d\n", warningCount, errorCount)
				Expect(errorCount).To(Equal(0), "There should be no errors in the logs")

				// Wait before next check
				time.Sleep(5 * time.Second)
			}

			GinkgoWriter.Println("Redpanda-only test completed successfully")
		})
	})

	Context("with dataflowcomponent scaling test", Label("dataflowcomponent-scaling"), func() {
		BeforeAll(func() {
			By("Starting with an empty configuration")
			cfg := NewBuilder().BuildYAML()
			// Write the empty config and start the container
			Expect(writeConfigFile(cfg)).To(Succeed())
			Expect(BuildAndRunContainer(cfg, DEFAULT_MEMORY, DEFAULT_CPUS)).To(Succeed())
			Expect(waitForMetrics()).To(Succeed(), "Metrics endpoint should be available with empty config")

		})

		AfterAll(func() {
			By("Stopping the container after the dataflowcomponent scaling test")
		})

		It("should scale up to multiple dataflow components while maintaining stability", func() {
			By("Adding the golden dataflow component as a baseline")
			builder := NewDataFlowComponentBuilder()
			builder.AddGoldenDataFlowComponent()
			cfg := builder.BuildYAML()
			Expect(writeConfigFile(cfg, getContainerName())).To(Succeed())

			By("Waiting for the golden service to become responsive")
			Eventually(func() int {
				return checkGoldenServiceStatusOnly()
			}, 20*time.Second, 1*time.Second).Should(Equal(200),
				"Golden service should respond with 200 OK")

			By("Waiting for the metrics endpoint to be healthy")
			Eventually(func() bool {
				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
				defer cancel()
				req, err := http.NewRequestWithContext(ctx, "GET", GetMetricsURL(), nil)
				if err != nil {
					return false
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return false
				}
				defer func() {
					if err := resp.Body.Close(); err != nil {
						Fail(fmt.Sprintf("Error closing response body: %v\n", err))
					}
				}()
				return resp.StatusCode == http.StatusOK
			}, 20*time.Second, 1*time.Second).Should(BeTrue(),
				"Metrics endpoint should be healthy")

			By("Scaling up by adding 10 dataflow generator components")
			// Add 10 dataflow components to the configuration
			for i := 0; i < 10; i++ {
				componentName := fmt.Sprintf("dataflow-%d", i)
				builder.AddGeneratorDataFlowComponent(componentName, fmt.Sprintf("%ds", 1+i%3)) // Varying intervals
				cfg = builder.BuildYAML()
				GinkgoWriter.Printf("Added dataflow component %s\n", componentName)
				Expect(writeConfigFile(cfg, getContainerName())).To(Succeed())

				// Allow time for component to start before adding the next one
				time.Sleep(1 * time.Second)

				// Check health periodically
				monitorHealth()
			}

			By("Simulating random stop/start/update actions on dataflow components")
			// Create a deterministic random number generator for reproducibility
			r := rand.New(rand.NewSource(42))
			for i := 0; i < 50; i++ {
				// Pick a random dataflow component index (0-9)
				randomIndex := r.Intn(10)
				randomComponentName := fmt.Sprintf("dataflow-%d", randomIndex)

				// Randomly decide operation: start, stop, or update
				opType := r.Intn(3)
				switch opType {
				case 0: // Start
					GinkgoWriter.Printf("Starting dataflow component %s\n", randomComponentName)
					builder.StartDataFlowComponent(randomComponentName)
				case 1: // Stop
					GinkgoWriter.Printf("Stopping dataflow component %s\n", randomComponentName)
					builder.StopDataFlowComponent(randomComponentName)
				case 2: // Update config
					newInterval := fmt.Sprintf("%ds", 1+r.Intn(5))
					GinkgoWriter.Printf("Updating dataflow component %s with new interval %s\n", randomComponentName, newInterval)
					builder.UpdateGeneratorDataFlowComponent(randomComponentName, newInterval)
				}

				// Apply the updated configuration
				Expect(writeConfigFile(builder.BuildYAML(), getContainerName())).To(Succeed())

				// Random delay between operations
				delay := time.Duration(1000+r.Intn(2000)) * time.Millisecond
				time.Sleep(delay)

				// Check the health of the system
				monitorHealth()

				// Every 10 operations, print a status update
				if i%10 == 0 {
					activeCount := builder.CountActiveDataFlowComponents()
					GinkgoWriter.Printf("\n=== DataFlowComponent Scaling Test Status ===\n"+
						"Actions completed: %d\n"+
						"Total dataflow components: %d\n"+
						"Active dataflow components: %d\n",
						i+1, 11, activeCount) // 11 includes golden component
				}
			}

			GinkgoWriter.Println("DataFlowComponent scaling test completed successfully")
		})
	})

})

// Helper functions for the chaos test

// getKeys returns all keys from a map as a slice
func getKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// countRunningServices counts how many services are in the "running" state
func countRunningServices(services map[string]string) int {
	count := 0
	for _, state := range services {
		if state == "running" {
			count++
		}
	}
	return count
}
