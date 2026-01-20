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

package fsmv2_adapter_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/fsmv2_adapter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var _ = Describe("LegacyChannelBridge", func() {
	var (
		legacyInbound  chan *models.UMHMessage
		legacyOutbound chan *models.UMHMessage
		logger         *zap.SugaredLogger
	)

	BeforeEach(func() {
		legacyInbound = make(chan *models.UMHMessage, 100)
		legacyOutbound = make(chan *models.UMHMessage, 100)
		logger = zap.NewNop().Sugar()
	})

	Describe("NewLegacyChannelBridge", func() {
		It("should create a bridge with correct buffer sizes", func() {
			bridge := fsmv2_adapter.NewLegacyChannelBridge(
				legacyInbound,
				legacyOutbound,
				logger,
			)

			Expect(bridge).NotTo(BeNil())
		})
	})

	Describe("GetChannels", func() {
		It("should return channels for a given worker ID", func() {
			bridge := fsmv2_adapter.NewLegacyChannelBridge(
				legacyInbound,
				legacyOutbound,
				logger,
			)

			inbound, outbound := bridge.GetChannels("test-worker")

			Expect(inbound).NotTo(BeNil())
			Expect(outbound).NotTo(BeNil())
		})
	})

	Describe("Interface compliance", func() {
		It("should implement communicator.ChannelProvider interface", func() {
			bridge := fsmv2_adapter.NewLegacyChannelBridge(
				legacyInbound,
				legacyOutbound,
				logger,
			)

			// Verify interface compliance via type assertion
			var provider communicator.ChannelProvider = bridge
			Expect(provider).NotTo(BeNil())
		})
	})
})
