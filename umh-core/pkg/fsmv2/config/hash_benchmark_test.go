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

package config_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// generateLargeConfig creates a config string of approximately the specified size in bytes.
func generateLargeConfig(sizeBytes int) string {
	// Each line is approximately 50 bytes
	lineTemplate := "  - name: sensor_%d\n    address: 192.168.1.%d\n"

	var sb strings.Builder

	sb.WriteString("sensors:\n")

	for i := 0; sb.Len() < sizeBytes; i++ {
		sb.WriteString(fmt.Sprintf(lineTemplate, i, i%256))
	}

	return sb.String()
}

// generateConfigWithTemplates creates a config with template variables.
func generateConfigWithTemplates(sizeBytes int) string {
	lineTemplate := "  - name: sensor_{{ .DEVICE_ID }}_%d\n    address: {{ .IP }}:{{ .PORT }}\n"

	var sb strings.Builder

	sb.WriteString("sensors:\n")

	for i := 0; sb.Len() < sizeBytes; i++ {
		sb.WriteString(fmt.Sprintf(lineTemplate, i))
	}

	return sb.String()
}

var _ = Describe("ComputeUserSpecHash Performance", func() {
	// Ratio-based performance assertions
	// Uses ratios instead of absolute times because:
	// - Absolute times vary 2-3x across different CI runners
	// - Ratios remain consistent because both operations run on same host
	// - Conservative 5x threshold gives 10x margin (actual is ~50x)

	Describe("hash vs render performance ratio", func() {
		const (
			warmupIterations = 100
			measureIterations = 1000
			minimumSpeedupRatio = 5.0 // Conservative: actual is ~50x
		)

		measurePerformance := func(configSize int, description string) {
			It(fmt.Sprintf("hash is at least %.0fx faster than render for %s configs", minimumSpeedupRatio, description), func() {
				// Create test data
				configTemplate := generateConfigWithTemplates(configSize)
				spec := config.UserSpec{
					Config: configTemplate,
					Variables: config.VariableBundle{
						User: map[string]any{
							"IP":        "192.168.1.100",
							"PORT":      502,
							"DEVICE_ID": "plc-01",
						},
						Global: map[string]any{
							"cluster_id": "test-cluster",
							"region":     "us-west-2",
						},
					},
				}

				// Warmup both operations
				for range warmupIterations {
					config.ComputeUserSpecHash(spec)
				}
				for range warmupIterations {
					_, _ = config.RenderConfigTemplate(spec.Config, spec.Variables)
				}

				// Measure hash time
				hashStart := time.Now()
				for range measureIterations {
					config.ComputeUserSpecHash(spec)
				}
				hashDuration := time.Since(hashStart)

				// Measure render time
				renderStart := time.Now()
				for range measureIterations {
					_, _ = config.RenderConfigTemplate(spec.Config, spec.Variables)
				}
				renderDuration := time.Since(renderStart)

				// Compute speedup ratio
				speedup := float64(renderDuration) / float64(hashDuration)

				// Log actual ratio for monitoring
				GinkgoWriter.Printf(
					"Performance for %s (%d bytes): hash=%v render=%v speedup=%.1fx\n",
					description, len(configTemplate), hashDuration, renderDuration, speedup,
				)

				// Assert minimum speedup (ratio-based, not absolute time)
				Expect(speedup).To(BeNumerically(">=", minimumSpeedupRatio),
					fmt.Sprintf("Hash should be at least %.0fx faster than render, got %.1fx", minimumSpeedupRatio, speedup))
			})
		}

		measurePerformance(1024, "1KB")       // Small config
		measurePerformance(100*1024, "100KB") // Medium config
		measurePerformance(1024*1024, "1MB")  // Large config
	})

	Describe("hash consistency under load", func() {
		It("produces identical hashes across many iterations", func() {
			spec := config.UserSpec{
				Config: generateLargeConfig(10 * 1024), // 10KB config
				Variables: config.VariableBundle{
					User: map[string]any{
						"IP":        "10.0.0.1",
						"PORT":      502,
						"DEVICE_ID": "sensor-array-1",
					},
				},
			}

			// Compute hash once as reference
			referenceHash := config.ComputeUserSpecHash(spec)

			// Verify 10,000 iterations all produce same hash
			for range 10000 {
				hash := config.ComputeUserSpecHash(spec)
				Expect(hash).To(Equal(referenceHash), "Hash should be deterministic across all iterations")
			}
		})
	})
})

// Standard Go benchmarks for use with benchstat
// Run with: go test -bench=. ./pkg/fsmv2/config/... -benchmem -count=10

func BenchmarkComputeUserSpecHash(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
		{"1MB", 1024 * 1024},
	}

	for _, s := range sizes {
		spec := config.UserSpec{
			Config: generateLargeConfig(s.size),
			Variables: config.VariableBundle{
				User: map[string]any{
					"IP":        "192.168.1.100",
					"PORT":      502,
					"DEVICE_ID": "plc-01",
				},
				Global: map[string]any{
					"cluster_id": "test-cluster",
				},
			},
		}

		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()

			for range b.N {
				config.ComputeUserSpecHash(spec)
			}
		})
	}
}

func BenchmarkRenderConfigTemplate(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
		{"1MB", 1024 * 1024},
	}

	for _, s := range sizes {
		configTemplate := generateConfigWithTemplates(s.size)
		variables := config.VariableBundle{
			User: map[string]any{
				"IP":        "192.168.1.100",
				"PORT":      502,
				"DEVICE_ID": "plc-01",
			},
			Global: map[string]any{
				"cluster_id": "test-cluster",
			},
		}

		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()

			for range b.N {
				_, _ = config.RenderConfigTemplate(configTemplate, variables)
			}
		})
	}
}

// BenchmarkHashVsRender provides a direct comparison for benchstat.
func BenchmarkHashVsRender(b *testing.B) {
	configTemplate := generateConfigWithTemplates(100 * 1024) // 100KB
	spec := config.UserSpec{
		Config: configTemplate,
		Variables: config.VariableBundle{
			User: map[string]any{
				"IP":        "192.168.1.100",
				"PORT":      502,
				"DEVICE_ID": "plc-01",
			},
		},
	}

	b.Run("Hash", func(b *testing.B) {
		b.ReportAllocs()

		for range b.N {
			config.ComputeUserSpecHash(spec)
		}
	})

	b.Run("Render", func(b *testing.B) {
		b.ReportAllocs()

		for range b.N {
			_, _ = config.RenderConfigTemplate(spec.Config, spec.Variables)
		}
	})
}
