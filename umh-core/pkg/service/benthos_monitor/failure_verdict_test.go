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

package benthos_monitor

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("FailureVerdict.ErrorFree", func() {
	reason := func(increase int64) string {
		v := FailureVerdict{Fired: []FailureEntry{{Signal: "output_error", Label: "output error", Count: increase, Window: 10 * time.Minute}}}
		ok, r := v.ErrorFree()
		Expect(ok).To(BeFalse())

		return r
	}

	It("names one failure in the singular", func() {
		Expect(reason(1)).To(Equal("benthos reported 1 output error in the last 10m0s"))
	})

	It("names more than one failure in the plural", func() {
		Expect(reason(2)).To(Equal("benthos reported 2 output errors in the last 10m0s"))
	})
})
