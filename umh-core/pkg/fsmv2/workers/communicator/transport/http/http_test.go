package transport_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	transport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

var _ = Describe("IsTransient", func() {
	DescribeTable("classifies error types correctly",
		func(errType transport.ErrorType, expected bool) {
			Expect(errType.IsTransient()).To(Equal(expected))
		},
		Entry("Network", transport.ErrorTypeNetwork, true),
		Entry("ServerError", transport.ErrorTypeServerError, true),
		Entry("ChannelFull", transport.ErrorTypeChannelFull, true),
		Entry("BackendRateLimit", transport.ErrorTypeBackendRateLimit, true),
		Entry("InstanceDeleted", transport.ErrorTypeInstanceDeleted, false),
		Entry("InvalidToken", transport.ErrorTypeInvalidToken, false),
		Entry("ProxyBlock", transport.ErrorTypeProxyBlock, false),
		Entry("CloudflareChallenge", transport.ErrorTypeCloudflareChallenge, false),
		Entry("Unknown", transport.ErrorTypeUnknown, false),
	)
})
