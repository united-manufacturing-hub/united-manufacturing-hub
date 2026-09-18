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

package telemetry_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/telemetry"
)

var _ = Describe("Identifier", func() {
	It("reports zero exactly when the tag is empty", func() {
		generated := telemetry.Identifier{
			Tag:      "cpu::read_failed",
			Brief:    "A cgroup CPU file could not be read.",
			Severity: telemetry.SeverityWarning,
		}

		Expect(telemetry.Identifier{}.IsZero()).To(BeTrue())
		Expect(telemetry.Identifier{Brief: "something"}.IsZero()).To(BeTrue())
		Expect(generated.IsZero()).To(BeFalse())
	})
})
