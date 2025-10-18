package protocol_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/protocol"
)

func RunTransportContractTests(factory func() protocol.Transport) {
	var (
		ctx       context.Context
		cancel    context.CancelFunc
		transport protocol.Transport
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		transport = factory()
	})

	AfterEach(func() {
		cancel()

		if transport != nil {
			_ = transport.Close()
		}
	})

	Describe("Send/Receive roundtrip", func() {
		It("should receive messages that were sent", func() {
			msgChan, err := transport.Receive(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(msgChan).ToNot(BeNil())

			testPayload := []byte("test-payload-data")
			err = transport.Send(ctx, "recipient-uuid", testPayload)
			Expect(err).ToNot(HaveOccurred())

			var received protocol.RawMessage
			Eventually(msgChan).Should(Receive(&received))
			Expect(received.Payload).To(Equal(testPayload))
			Expect(received.Timestamp).To(BeTemporally("~", time.Now(), time.Second))
		})

		It("should receive multiple messages in order", func() {
			msgChan, err := transport.Receive(ctx)
			Expect(err).ToNot(HaveOccurred())

			payloads := [][]byte{
				[]byte("message-1"),
				[]byte("message-2"),
				[]byte("message-3"),
			}

			for _, payload := range payloads {
				err = transport.Send(ctx, "recipient-uuid", payload)
				Expect(err).ToNot(HaveOccurred())
			}

			for _, expectedPayload := range payloads {
				var received protocol.RawMessage
				Eventually(msgChan).Should(Receive(&received))
				Expect(received.Payload).To(Equal(expectedPayload))
			}
		})
	})

	Describe("Sender UUID propagation", func() {
		It("should include From field in received messages", func() {
			if mockTransport, ok := transport.(*protocol.MockTransport); ok {
				mockTransport.SetSenderUUID("sender-123")
			}

			msgChan, err := transport.Receive(ctx)
			Expect(err).ToNot(HaveOccurred())

			err = transport.Send(ctx, "recipient-uuid", []byte("test"))
			Expect(err).ToNot(HaveOccurred())

			var received protocol.RawMessage
			Eventually(msgChan).Should(Receive(&received))

			if _, ok := transport.(*protocol.MockTransport); ok {
				Expect(received.From).To(Equal("sender-123"))
			} else {
				Expect(received.From).ToNot(BeEmpty())
			}
		})
	})

	Describe("Close behavior", func() {
		It("should close without error", func() {
			err := transport.Close()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return error when sending after close", func() {
			err := transport.Close()
			Expect(err).ToNot(HaveOccurred())

			err = transport.Send(ctx, "recipient-uuid", []byte("test"))
			Expect(err).To(HaveOccurred())
		})

		It("should return error when receiving after close", func() {
			err := transport.Close()
			Expect(err).ToNot(HaveOccurred())

			_, err = transport.Receive(ctx)
			Expect(err).To(HaveOccurred())
		})

		It("should be idempotent", func() {
			err := transport.Close()
			Expect(err).ToNot(HaveOccurred())

			err = transport.Close()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("Error propagation", func() {
		It("should propagate network errors as TransportNetworkError", func() {
			if mockTransport, ok := transport.(*protocol.MockTransport); ok {
				mockTransport.SimulateNetworkError(errors.New("network down"))

				err := transport.Send(ctx, "recipient-uuid", []byte("test"))
				Expect(err).To(HaveOccurred())

				var netErr protocol.TransportNetworkError
				Expect(errors.As(err, &netErr)).To(BeTrue())
			}
		})

		It("should propagate overflow errors as TransportOverflowError", func() {
			if mockTransport, ok := transport.(*protocol.MockTransport); ok {
				mockTransport.SimulateOverflowError(errors.New("queue full"))

				err := transport.Send(ctx, "recipient-uuid", []byte("test"))
				Expect(err).To(HaveOccurred())

				var overflowErr protocol.TransportOverflowError
				Expect(errors.As(err, &overflowErr)).To(BeTrue())
			}
		})

		It("should propagate timeout errors as TransportTimeoutError", func() {
			if mockTransport, ok := transport.(*protocol.MockTransport); ok {
				mockTransport.SimulateTimeoutError(errors.New("timeout"))

				err := transport.Send(ctx, "recipient-uuid", []byte("test"))
				Expect(err).To(HaveOccurred())

				var timeoutErr protocol.TransportTimeoutError
				Expect(errors.As(err, &timeoutErr)).To(BeTrue())
			}
		})
	})
}
