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

package types_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

var _ = Describe("ErrorType", func() {
	It("covers every ErrorType value — fails when a new constant is added without classifying it", func() {
		for i := range types.ErrorTypeMax {
			errType := types.ErrorType(i)
			// Maps return the zero value for absent keys (false / ""), so we
			// cannot rely on the return value to detect gaps. Instead we assert
			// that an explicit entry exists for every iota value.
			Expect(types.IsTransientTypes).To(HaveKey(errType),
				"ErrorType(%d) has no entry in IsTransientTypes — add it to types.go", i)
			Expect(types.ErrorTypeNames).To(HaveKey(errType),
				"ErrorType(%d) has no entry in ErrorTypeNames — add it to types.go", i)
			Expect(types.ErrorTypeCounters).To(HaveKey(errType),
				"ErrorType(%d) has no entry in ErrorTypeCounters — add it to types.go", i)
		}
	})
})
