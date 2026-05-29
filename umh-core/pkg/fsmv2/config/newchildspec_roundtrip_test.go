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

// NewChildSpec[T] round-trip guard.
//
// Ensures that NewChildSpec marshals T into UserSpec.Config and that ParseUserSpec[T]
// recovers exactly the same fields. Catches two silent failure modes:
//   - marshalling the wrong thing (empty struct, wrong type)
//   - field name mismatch between marshal and unmarshal tags
var _ = Describe("NewChildSpec round-trip (P2-2)", func() {
	type sampleSpec struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	}

	It("round-trips a typed struct through NewChildSpec → ParseUserSpec", func() {
		original := sampleSpec{Host: "relay.example.com", Port: 8443}

		spec, err := config.NewChildSpec("myworker", "mytype", original, true)
		Expect(err).ToNot(HaveOccurred())

		// Config string must be non-empty (marshalling actually happened)
		Expect(spec.UserSpec.Config).ToNot(BeEmpty())

		// ParseUserSpec must recover the same field values
		recovered, err := config.ParseUserSpec[sampleSpec](spec.UserSpec)
		Expect(err).ToNot(HaveOccurred())
		Expect(recovered.Host).To(Equal("relay.example.com"))
		Expect(recovered.Port).To(Equal(8443))
	})

	It("propagates the enabled flag to ChildSpec.Enabled", func() {
		spec, err := config.NewChildSpec("w", "t", config.BaseUserSpec{}, true)
		Expect(err).ToNot(HaveOccurred())
		Expect(spec.Enabled).To(BeTrue())

		specDisabled, err := config.NewChildSpec("w", "t", config.BaseUserSpec{}, false)
		Expect(err).ToNot(HaveOccurred())
		Expect(specDisabled.Enabled).To(BeFalse())
	})

	It("sets Name and WorkerType correctly", func() {
		spec, err := config.NewChildSpec("push", "push", config.BaseUserSpec{}, false)
		Expect(err).ToNot(HaveOccurred())
		Expect(spec.Name).To(Equal("push"))
		Expect(spec.WorkerType).To(Equal("push"))
	})
})
