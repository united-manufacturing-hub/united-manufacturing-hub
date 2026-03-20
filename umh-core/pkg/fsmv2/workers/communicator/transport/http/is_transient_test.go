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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

var _ = Describe("IsTransient", func() {
	DescribeTable("classifies all ErrorType values",
		func(errType httpTransport.ErrorType, expected bool) {
			Expect(errType.IsTransient()).To(Equal(expected))
		},
		Entry("Unknown is persistent", httpTransport.ErrorTypeUnknown, false),
		Entry("CloudflareChallenge is persistent", httpTransport.ErrorTypeCloudflareChallenge, false),
		Entry("InvalidToken is persistent", httpTransport.ErrorTypeInvalidToken, false),
		Entry("InstanceDeleted is persistent", httpTransport.ErrorTypeInstanceDeleted, false),
		Entry("ProxyBlock is persistent", httpTransport.ErrorTypeProxyBlock, false),
		Entry("Network is transient", httpTransport.ErrorTypeNetwork, true),
		Entry("ServerError is transient", httpTransport.ErrorTypeServerError, true),
		Entry("ChannelFull is transient", httpTransport.ErrorTypeChannelFull, true),
		Entry("BackendRateLimit is transient", httpTransport.ErrorTypeBackendRateLimit, true),
	)
})
