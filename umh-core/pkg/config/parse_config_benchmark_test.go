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

package config

import (
	"os"
	"testing"
)

// BenchmarkParseConfig_100 benchmarks parseConfig with the example-config-100.yaml file
func BenchmarkParseConfig_100(b *testing.B) {
	cfg, err := os.ReadFile("../../examples/example-config-100.yaml")
	if err != nil {
		b.Fatalf("Failed to read example config file: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := ParseConfig(cfg, false)
		if err != nil {
			b.Fatalf("parseConfig failed: %v", err)
		}
	}
}

// BenchmarkParseConfig_Comm benchmarks parseConfig with the example-config-comm.yaml file
func BenchmarkParseConfig_Comm(b *testing.B) {
	cfg, err := os.ReadFile("../../examples/example-config-comm.yaml")
	if err != nil {
		b.Fatalf("Failed to read example config file: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := ParseConfig(cfg, false)
		if err != nil {
			b.Fatalf("parseConfig failed: %v", err)
		}
	}
}
