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

var _ = Describe("ComputeUserSpecHash Performance", func() {
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
