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

package snapshot_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
)

var _ = Describe("TransportDependencies interface", func() {
	It("should be defined as an interface type", func() {
		// Compile-time check: verify the interface is importable and non-nil as a type.
		// The actual implementations are tested via action tests.
		var _ snapshot.TransportDependencies
		Expect(true).To(BeTrue()) // Placeholder — compile-time check is the real test
	})
})
