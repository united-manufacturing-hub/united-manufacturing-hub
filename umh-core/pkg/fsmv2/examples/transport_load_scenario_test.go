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

package examples_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	transportWorker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

// The transport-load scenario has no pass condition yet. What load the product
// must sustain is not decided, so this spec asserts only that the scenario
// completes and shuts down cleanly. Rows pinning an envelope belong here later,
// as a DescribeTable.
//
// The load is chosen so that shutdown is the hard case rather than the easy one.
// Several streams against an uplink far too slow for them leave the outbound
// channel full at the moment the run ends, and nothing reads that channel after
// the worker stops. A stream still parked on a send would then never return, so
// a regression in the scenario's own teardown appears here as Done never
// closing.
var _ = Describe("Transport Load Scenario", func() {
	var ctx context.Context
	var cancel context.CancelFunc

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 35*time.Second)
	})

	AfterEach(func() {
		cancel()
		// SetChannelProvider is process-global, so leaving it set pollutes every
		// later spec in this package.
		transportWorker.ClearChannelProvider()
	})

	It("completes and drains its own streams when the uplink cannot keep up", func() {
		result := examples.RunTransportLoadScenario(ctx,
			examples.TransportRunConfig{Duration: 3 * time.Second},
			examples.TransportLoadConfig{
				Subscribers:             4,
				PayloadBytes:            32 * 1024,
				BandwidthBytesPerSecond: 4096,
			})

		Expect(result.Error).NotTo(HaveOccurred())
		Eventually(result.Done, GracefulShutdownCascadingTimeout).Should(BeClosed())
		Expect(result.ShutdownClean).To(BeTrue())

		// Teardown outlives the run, so Done alone proves nothing about it.
		// Deleting the channel drain in stopLoad leaves the streams parked on a
		// full queue for ever, and this is the assertion that then fails.
		Eventually(result.LoadStopped, GracefulShutdownCascadingTimeout).Should(BeClosed())
	})
})
