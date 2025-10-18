package cse_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/cse"
)

var _ = Describe("Transport Interface", func() {
	Describe("MockTransport", func() {
		Context("Contract tests", func() {
			RunTransportContractTests(func() cse.Transport {
				return cse.NewMockTransport()
			})
		})

		Context("Mock-specific utilities", func() {
			var transport *cse.MockTransport

			BeforeEach(func() {
				transport = cse.NewMockTransport()
			})

			AfterEach(func() {
				if transport != nil {
					_ = transport.Close()
				}
			})

			It("should clear simulated errors", func() {
				transport.SimulateNetworkError(errors.New("network down"))

				transport.ClearSimulatedErrors()

				ctx := context.Background()
				err := transport.Send(ctx, "recipient-uuid", []byte("test"))
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
