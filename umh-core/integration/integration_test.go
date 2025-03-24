// integration_test.go

package integration_test

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ---------- Actual Ginkgo Tests ----------

var _ = Describe("UMH Container Integration", Ordered, Label("integration"), func() {

	AfterAll(func() {
		// Always stop container after the entire suite
		StopContainer()
	})

	Context("with an empty config", func() {
		BeforeAll(func() {
			By("Building an empty config and writing to data/config.yaml")
			emptyConfig := `
agent:
  metricsPort: 8080
services: []
benthos: []
`
			Expect(writeConfigFile(emptyConfig)).To(Succeed())
			Expect(BuildAndRunContainer(emptyConfig, "1024m")).To(Succeed())
			Expect(waitForMetrics()).To(Succeed(), "Metrics endpoint should be available with empty config")
		})

		AfterAll(func() {
			StopContainer() // Stop container after empty config scenario
		})

		It("exposes metrics and has zero s6 services running", func() {
			// Check the /metrics endpoint
			resp, err := http.Get(metricsURL)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			// Check that the metrics endpoint contains the expected metrics
			Expect(string(body)).To(ContainSubstring("umh_core_reconcile_duration_milliseconds"))
		})
	})

	Context("with a golden service config", func() {
		BeforeAll(func() {
			By("Building a config with the golden service and writing to data/config.yaml")
			cfg := NewBuilder().
				AddGoldenService().
				BuildYAML()

			Expect(writeConfigFile(cfg)).To(Succeed())
			Expect(BuildAndRunContainer(cfg, "1024m")).To(Succeed())
			Expect(waitForMetrics()).To(Succeed(), "Metrics endpoint should be available with golden service config")
		})

		AfterAll(func() {
			StopContainer() // Stop container after golden config scenario
		})

		It("should have the golden service up and expose metrics", func() {
			// Check /metrics
			resp, err := http.Get(metricsURL)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(ContainSubstring("umh_core_reconcile_duration_milliseconds"))

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
			Expect(BuildAndRunContainer(cfg, "1024m")).To(Succeed())
			Expect(waitForMetrics()).To(Succeed(), "Metrics endpoint should be available with golden + sleep service config")

			// Verify that the golden service is ready
			Eventually(func() int {
				return checkGoldenServiceStatusOnly()
			}, 10*time.Second, 1*time.Second).Should(Equal(200),
				"Golden service should respond with 200 OK on the mapped port")
			GinkgoWriter.Println("Golden service is up and running")
		})

		AfterAll(func() {
			By("Stopping container after the multiple services test")
			StopContainer()
		})

		It("should have both services active and expose healthy metrics", func() {
			By("Verifying the metrics endpoint contains expected metrics")
			resp, err := http.Get(metricsURL)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(ContainSubstring("umh_core_reconcile_duration_milliseconds"))

			By("Verifying that the golden service is returning 200 OK")
			Eventually(func() int {
				return checkGoldenServiceStatusOnly()
			}, 10*time.Second, 1*time.Second).Should(Equal(200),
				"Golden service should respond with 200 OK on the mapped port")
		})
	})

	Context("with service scaling test", Label("scaling"), func() {
		AfterEach(func() {
			if CurrentSpecReport().Failed() {
				fmt.Println("Test failed, printing container logs:")
				printContainerLogs()

				// Print the latest YAML config
				fmt.Println("\nLatest YAML config at time of failure:")
				config, err := os.ReadFile("data/config.yaml")
				if err != nil {
					fmt.Printf("Failed to read config file: %v\n", err)
				} else {
					fmt.Println(string(config))
				}
			}
		})

		BeforeAll(func() {
			By("Starting with an empty configuration")
			cfg := NewBuilder().BuildYAML()
			// Write the empty config and start the container
			Expect(writeConfigFile(cfg)).To(Succeed())
			Expect(BuildAndRunContainer(cfg, "1024m")).To(Succeed())
			Expect(waitForMetrics()).To(Succeed(), "Metrics endpoint should be available with empty config")
		})

		AfterAll(func() {
			By("Stopping the container after the scaling test")
			StopContainer()
		})

		It("should scale up to multiple services while maintaining healthy metrics", func() {
			By("Adding the golden service as a baseline")
			// Build configuration with the golden service first
			builder := NewBuilder().AddGoldenService()
			cfg := builder.BuildYAML()
			Expect(writeConfigFile(cfg)).To(Succeed())

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
				Expect(writeConfigFile(cfg)).To(Succeed())
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
				Expect(writeConfigFile(builder.BuildYAML())).To(Succeed())

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

		AfterEach(func() {
			if CurrentSpecReport().Failed() {
				//fmt.Println("Test failed, printing container logs:")
				//printContainerLogs()

				// Print the latest YAML config
				//fmt.Println("\nLatest YAML config at time of failure:")
				//config, err := os.ReadFile("data/config.yaml")
				//if err != nil {
				//	fmt.Printf("Failed to read config file: %v\n", err)
				//} else {
				//	fmt.Println(string(config))
				//}
			}
		})

		BeforeAll(func() {
			//Skip("Skipping comprehensive chaos test due to time constraints")
			// Start with an empty config
			cfg := NewBuilder().BuildYAML()
			Expect(writeConfigFile(cfg)).To(Succeed())
			Expect(BuildAndRunContainer(cfg, "1024m")).To(Succeed())
			Expect(waitForMetrics()).To(Succeed(), "Metrics endpoint should be available with empty config")
		})

		AfterAll(func() {
			// Get the docker logs and find any [WARN] and [ERROR] messages
			out, err := runDockerCommand("logs", containerName)
			if err != nil {
				fmt.Printf("Failed to get container logs: %v\n", err)
				return
			}
			for _, line := range strings.Split(out, "\n") {
				if strings.Contains(line, "[WARN]") || strings.Contains(line, "[ERROR]") {
					fmt.Printf("Container logs:\n%s\n", line)
				}
			}
			StopContainer()
		})

		It("should handle random service additions, deletions, starts and stops", func() {
			// Start monitoring goroutine
			testDuration := 3 * time.Minute

			// Create deterministic random number generator
			r := rand.New(rand.NewSource(42))

			// Add golden service as constant baseline
			builder := NewBuilder().AddGoldenService()
			Expect(writeConfigFile(builder.BuildYAML())).To(Succeed())

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
				Expect(writeConfigFile(builder.BuildYAML())).To(Succeed())

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
				Expect(writeConfigFile(cfg)).To(Succeed())

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
		AfterEach(func() {
			if CurrentSpecReport().Failed() {
				fmt.Println("Test failed, printing container logs:")
				printContainerLogs()

				// Print the latest YAML config
				fmt.Println("\nLatest YAML config at time of failure:")
				config, err := os.ReadFile("data/config.yaml")
				if err != nil {
					fmt.Printf("Failed to read config file: %v\n", err)
				} else {
					fmt.Println(string(config))
				}
			}
		})

		BeforeAll(func() {
			By("Starting with an empty configuration")
			cfg := NewBuilder().BuildYAML()
			// Write the empty config and start the container
			Expect(writeConfigFile(cfg)).To(Succeed())
			Expect(BuildAndRunContainer(cfg, "2048m")).To(Succeed())
			Expect(waitForMetrics()).To(Succeed(), "Metrics endpoint should be available with empty config")
		})

		AfterAll(func() {
			By("Stopping the container after the benthos scaling test")
			StopContainer()
		})

		It("should scale up to multiple benthos instances while maintaining stability", func() {
			By("Adding the golden service as a baseline")
			builder := NewBenthosBuilder()
			builder.AddGoldenBenthos()
			cfg := builder.BuildYAML()
			Expect(writeConfigFile(cfg)).To(Succeed())

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
				Expect(writeConfigFile(cfg)).To(Succeed())

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
				Expect(writeConfigFile(builder.BuildYAML())).To(Succeed())

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
