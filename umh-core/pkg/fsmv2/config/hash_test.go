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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

var _ = Describe("ComputeUserSpecHash", func() {
	Describe("determinism", func() {
		It("produces same hash for same UserSpec", func() {
			spec := config.UserSpec{
				Config: "test config",
				Variables: config.VariableBundle{
					User: map[string]any{"IP": "192.168.1.100"},
				},
			}

			hash1 := config.ComputeUserSpecHash(spec)
			hash2 := config.ComputeUserSpecHash(spec)

			Expect(hash1).To(Equal(hash2))
		})

		It("produces same hash for identical UserSpecs created separately", func() {
			spec1 := config.UserSpec{
				Config: "mqtt:\n  url: tcp://localhost:1883",
				Variables: config.VariableBundle{
					User: map[string]any{
						"IP":   "10.0.0.1",
						"PORT": 502,
					},
					Global: map[string]any{
						"cluster_id": "test-cluster",
					},
				},
			}

			spec2 := config.UserSpec{
				Config: "mqtt:\n  url: tcp://localhost:1883",
				Variables: config.VariableBundle{
					User: map[string]any{
						"IP":   "10.0.0.1",
						"PORT": 502,
					},
					Global: map[string]any{
						"cluster_id": "test-cluster",
					},
				},
			}

			hash1 := config.ComputeUserSpecHash(spec1)
			hash2 := config.ComputeUserSpecHash(spec2)

			Expect(hash1).To(Equal(hash2))
		})
	})

	Describe("different inputs produce different hashes", func() {
		It("produces different hash when Config differs", func() {
			spec1 := config.UserSpec{
				Config: "config version 1",
				Variables: config.VariableBundle{
					User: map[string]any{"key": "value"},
				},
			}

			spec2 := config.UserSpec{
				Config: "config version 2",
				Variables: config.VariableBundle{
					User: map[string]any{"key": "value"},
				},
			}

			hash1 := config.ComputeUserSpecHash(spec1)
			hash2 := config.ComputeUserSpecHash(spec2)

			Expect(hash1).ToNot(Equal(hash2))
		})

		It("produces different hash when Variables.User differs", func() {
			spec1 := config.UserSpec{
				Config: "same config",
				Variables: config.VariableBundle{
					User: map[string]any{"IP": "192.168.1.100"},
				},
			}

			spec2 := config.UserSpec{
				Config: "same config",
				Variables: config.VariableBundle{
					User: map[string]any{"IP": "192.168.1.200"},
				},
			}

			hash1 := config.ComputeUserSpecHash(spec1)
			hash2 := config.ComputeUserSpecHash(spec2)

			Expect(hash1).ToNot(Equal(hash2))
		})

		It("produces different hash when Variables.Global differs", func() {
			spec1 := config.UserSpec{
				Config: "same config",
				Variables: config.VariableBundle{
					User:   map[string]any{"IP": "192.168.1.100"},
					Global: map[string]any{"env": "production"},
				},
			}

			spec2 := config.UserSpec{
				Config: "same config",
				Variables: config.VariableBundle{
					User:   map[string]any{"IP": "192.168.1.100"},
					Global: map[string]any{"env": "staging"},
				},
			}

			hash1 := config.ComputeUserSpecHash(spec1)
			hash2 := config.ComputeUserSpecHash(spec2)

			Expect(hash1).ToNot(Equal(hash2))
		})
	})

	Describe("empty UserSpec handling", func() {
		It("produces consistent hash for empty UserSpec", func() {
			spec := config.UserSpec{}

			hash1 := config.ComputeUserSpecHash(spec)
			hash2 := config.ComputeUserSpecHash(spec)

			Expect(hash1).To(Equal(hash2))
		})

		It("produces consistent hash for UserSpec with empty Config", func() {
			spec := config.UserSpec{
				Config: "",
				Variables: config.VariableBundle{
					User: map[string]any{"key": "value"},
				},
			}

			hash1 := config.ComputeUserSpecHash(spec)
			hash2 := config.ComputeUserSpecHash(spec)

			Expect(hash1).To(Equal(hash2))
		})

		It("produces consistent hash for UserSpec with nil Variables maps", func() {
			spec := config.UserSpec{
				Config: "some config",
				Variables: config.VariableBundle{
					User:   nil,
					Global: nil,
				},
			}

			hash1 := config.ComputeUserSpecHash(spec)
			hash2 := config.ComputeUserSpecHash(spec)

			Expect(hash1).To(Equal(hash2))
		})
	})

	Describe("map key ordering independence", func() {
		It("produces same hash regardless of map key insertion order", func() {
			// Create first spec with keys added in one order
			spec1 := config.UserSpec{
				Config: "test config",
				Variables: config.VariableBundle{
					User: map[string]any{
						"alpha": "first",
						"beta":  "second",
						"gamma": "third",
					},
				},
			}

			// Create second spec with keys added in different order
			// (Go maps don't guarantee order, but we explicitly construct differently)
			user2 := make(map[string]any)
			user2["gamma"] = "third"
			user2["alpha"] = "first"
			user2["beta"] = "second"

			spec2 := config.UserSpec{
				Config: "test config",
				Variables: config.VariableBundle{
					User: user2,
				},
			}

			hash1 := config.ComputeUserSpecHash(spec1)
			hash2 := config.ComputeUserSpecHash(spec2)

			Expect(hash1).To(Equal(hash2))
		})

		It("produces same hash for nested maps regardless of key order", func() {
			spec1 := config.UserSpec{
				Config: "nested config",
				Variables: config.VariableBundle{
					User: map[string]any{
						"outer": map[string]any{
							"a": 1,
							"b": 2,
							"c": 3,
						},
					},
				},
			}

			// Create nested map with different insertion order
			inner2 := make(map[string]any)
			inner2["c"] = 3
			inner2["a"] = 1
			inner2["b"] = 2

			spec2 := config.UserSpec{
				Config: "nested config",
				Variables: config.VariableBundle{
					User: map[string]any{
						"outer": inner2,
					},
				},
			}

			hash1 := config.ComputeUserSpecHash(spec1)
			hash2 := config.ComputeUserSpecHash(spec2)

			Expect(hash1).To(Equal(hash2))
		})
	})

	Describe("hash format", func() {
		It("returns a non-empty string", func() {
			spec := config.UserSpec{
				Config: "test",
			}

			hash := config.ComputeUserSpecHash(spec)

			Expect(hash).ToNot(BeEmpty())
		})

		It("returns a hash of reasonable length", func() {
			spec := config.UserSpec{
				Config: "test config with some content",
				Variables: config.VariableBundle{
					User: map[string]any{"key": "value"},
				},
			}

			hash := config.ComputeUserSpecHash(spec)

			// FNV-1a 64-bit hash is 16 hex chars (64 bits / 4 bits per hex char)
			// Ensure it's at least 8 chars for reasonable uniqueness
			Expect(len(hash)).To(BeNumerically(">=", 8))
		})
	})
})
