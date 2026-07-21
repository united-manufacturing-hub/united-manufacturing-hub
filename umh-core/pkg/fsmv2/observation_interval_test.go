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

package fsmv2_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

// The per-worker-type observation-interval registry lets a worker declare its
// collection cadence once (via simple.MonitorSpec.Interval) so both the supervisor's
// collector and the fsmv1 adapter (StaleAfter = 3×Interval) read one source of
// truth. The getter returns (value, ok); the caller applies its own default so
// this core package need not import the supervisor's DefaultObservationInterval
// (ENG-5305).
var _ = Describe("Observation interval registry", func() {
	It("returns a registered worker type's interval", func() {
		fsmv2.RegisterObservationInterval("eng5305-rung2-worker", 15*time.Second)

		iv, ok := fsmv2.ObservationIntervalFor("eng5305-rung2-worker")
		Expect(ok).To(BeTrue())
		Expect(iv).To(Equal(15 * time.Second))
	})

	It("reports not-registered for an unknown worker type", func() {
		_, ok := fsmv2.ObservationIntervalFor("eng5305-never-registered")
		Expect(ok).To(BeFalse(), "caller must fall back to its own default")
	})

	It("ignores a non-positive interval so it falls back to the default", func() {
		fsmv2.RegisterObservationInterval("eng5305-zero-worker", 0)

		_, ok := fsmv2.ObservationIntervalFor("eng5305-zero-worker")
		Expect(ok).To(BeFalse())
	})
})
