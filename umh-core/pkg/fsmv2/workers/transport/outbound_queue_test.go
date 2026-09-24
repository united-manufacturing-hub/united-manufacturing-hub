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

package transport_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/channelusage"
)

var _ = Describe("TransportDependencies outbound queue", func() {
	BeforeEach(func() {
		transport.SetChannelProvider(newTestChannelProvider())
		DeferCleanup(transport.ClearChannelProvider)
	})

	It("reports a burst the push child drained between two samples", func() {
		d := transport.NewTransportDependencies(nil, deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, deps.Identity{ID: "transport-001"}))
		d.RecordOutboundDepth(100)

		at := time.Now()

		var queue channelusage.Verdict
		for range 20 {
			queue = d.SampleOutboundQueue(at)
			at = at.Add(time.Second)
		}

		Expect(queue.Measured).To(BeTrue())
		Expect(queue.PeakPercent).To(Equal(100.0))
		Expect(queue.Degraded).To(BeTrue())
	})
})
