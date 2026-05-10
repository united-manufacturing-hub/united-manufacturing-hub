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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// convCfgA / convStatusA are the types ConvertWorkerSnapshot is parameterised with
// in the happy-path and wrong-desired-type tests.
type convCfgA struct{ URL string }

// convCfgB is a different config type used to trigger the wrong-desired panic.
type convCfgB struct{ Port int }

// convStatusA / convStatusB are distinct status types for the happy-path and
// wrong-observed panic tests respectively.
type convStatusA struct{ Healthy bool }
type convStatusB struct{ Errors int }

var _ = Describe("ConvertWorkerSnapshot", func() {
	Describe("panic paths", func() {
		It("panics when snapAny is not an fsmv2.Snapshot", func() {
			Expect(func() {
				fsmv2.ConvertWorkerSnapshot[convCfgA, convStatusA]("not-a-snapshot")
			}).To(PanicWith(ContainSubstring("ConvertWorkerSnapshot")))
		})

		It("panics when Observed carries the wrong Observation type", func() {
			// Snapshot is valid, but Observed holds Observation[convStatusB]
			// while the caller requests Observation[convStatusA].
			snap := fsmv2.Snapshot{
				Observed: fsmv2.Observation[convStatusB]{},
				Desired:  &fsmv2.WrappedDesiredState[convCfgA]{},
				Identity: deps.Identity{},
			}
			Expect(func() {
				fsmv2.ConvertWorkerSnapshot[convCfgA, convStatusA](snap)
			}).To(PanicWith(ContainSubstring("ConvertWorkerSnapshot")))
		})

		It("panics when Desired carries the wrong WrappedDesiredState type", func() {
			// Observed is correct, but Desired holds *WrappedDesiredState[convCfgB]
			// while the caller requests *WrappedDesiredState[convCfgA].
			snap := fsmv2.Snapshot{
				Observed: fsmv2.Observation[convStatusA]{},
				Desired:  &fsmv2.WrappedDesiredState[convCfgB]{},
				Identity: deps.Identity{},
			}
			Expect(func() {
				fsmv2.ConvertWorkerSnapshot[convCfgA, convStatusA](snap)
			}).To(PanicWith(ContainSubstring("ConvertWorkerSnapshot")))
		})
	})
})
