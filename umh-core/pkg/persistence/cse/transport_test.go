package cse_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/cse"
)

var _ = Describe("Transport Interface", func() {
	Context("RawMessage", func() {
		It("should contain sender UUID, payload, and timestamp", func() {
			timestamp := time.Now()
			msg := cse.RawMessage{
				From:      "sender-uuid-123",
				Payload:   []byte("encrypted-data"),
				Timestamp: timestamp,
			}

			Expect(msg.From).To(Equal("sender-uuid-123"))
			Expect(msg.Payload).To(Equal([]byte("encrypted-data")))
			Expect(msg.Timestamp).To(Equal(timestamp))
		})

		It("should support timestamp validation for replay attack detection", func() {
			oldTimestamp := time.Now().Add(-10 * time.Minute)
			msg := cse.RawMessage{
				From:      "sender-uuid",
				Payload:   []byte("data"),
				Timestamp: oldTimestamp,
			}

			maxAge := 5 * time.Minute
			age := time.Since(msg.Timestamp)
			Expect(age).To(BeNumerically(">", maxAge))
		})
	})

	Context("TransportError", func() {
		It("should support network error type", func() {
			err := cse.TransportNetworkError{Err: errors.New("connection failed")}
			Expect(err.Error()).To(ContainSubstring("connection failed"))
		})

		It("should support overflow error type", func() {
			err := cse.TransportOverflowError{Err: errors.New("queue full")}
			Expect(err.Error()).To(ContainSubstring("queue full"))
		})

		It("should support config error type", func() {
			err := cse.TransportConfigError{Err: errors.New("invalid config")}
			Expect(err.Error()).To(ContainSubstring("invalid config"))
		})

		It("should support timeout error type", func() {
			err := cse.TransportTimeoutError{Err: errors.New("deadline exceeded")}
			Expect(err.Error()).To(ContainSubstring("deadline exceeded"))
		})

		It("should support auth error type", func() {
			err := cse.TransportAuthError{Err: errors.New("invalid token")}
			Expect(err.Error()).To(ContainSubstring("invalid token"))
		})
	})

	Context("MockTransport", func() {
		var (
			ctx       context.Context
			cancel    context.CancelFunc
			transport *cse.MockTransport
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			transport = cse.NewMockTransport()
		})

		AfterEach(func() {
			cancel()
			if transport != nil {
				transport.Close()
			}
		})

		It("should satisfy Transport interface", func() {
			var _ cse.Transport = transport
		})

		It("should send messages successfully", func() {
			err := transport.Send(ctx, "recipient-uuid", []byte("test-payload"))
			Expect(err).ToNot(HaveOccurred())
		})

		It("should receive sent messages", func() {
			msgChan, err := transport.Receive(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(msgChan).ToNot(BeNil())

			err = transport.Send(ctx, "recipient-uuid", []byte("test-payload"))
			Expect(err).ToNot(HaveOccurred())

			Eventually(msgChan).Should(Receive())
		})

		It("should include sender UUID in received messages", func() {
			transport.SetSenderUUID("sender-123")
			msgChan, err := transport.Receive(ctx)
			Expect(err).ToNot(HaveOccurred())

			err = transport.Send(ctx, "recipient-uuid", []byte("test-payload"))
			Expect(err).ToNot(HaveOccurred())

			var msg cse.RawMessage
			Eventually(msgChan).Should(Receive(&msg))
			Expect(msg.From).To(Equal("sender-123"))
			Expect(msg.Payload).To(Equal([]byte("test-payload")))
		})

		It("should close gracefully", func() {
			err := transport.Close()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return error when sending after close", func() {
			err := transport.Close()
			Expect(err).ToNot(HaveOccurred())

			err = transport.Send(ctx, "recipient-uuid", []byte("test-payload"))
			Expect(err).To(HaveOccurred())
		})

		It("should simulate network errors", func() {
			transport.SimulateNetworkError(errors.New("network down"))
			err := transport.Send(ctx, "recipient-uuid", []byte("test-payload"))
			Expect(err).To(HaveOccurred())

			var netErr cse.TransportNetworkError
			Expect(errors.As(err, &netErr)).To(BeTrue())
		})

		It("should simulate overflow errors", func() {
			transport.SimulateOverflowError(errors.New("queue full"))
			err := transport.Send(ctx, "recipient-uuid", []byte("test-payload"))
			Expect(err).To(HaveOccurred())

			var overflowErr cse.TransportOverflowError
			Expect(errors.As(err, &overflowErr)).To(BeTrue())
		})

		It("should simulate timeout errors", func() {
			transport.SimulateTimeoutError(errors.New("timeout"))
			err := transport.Send(ctx, "recipient-uuid", []byte("test-payload"))
			Expect(err).To(HaveOccurred())

			var timeoutErr cse.TransportTimeoutError
			Expect(errors.As(err, &timeoutErr)).To(BeTrue())
		})

		It("should clear simulated errors", func() {
			transport.SimulateNetworkError(errors.New("network down"))
			err := transport.Send(ctx, "recipient-uuid", []byte("test-payload"))
			Expect(err).To(HaveOccurred())

			transport.ClearSimulatedErrors()
			err = transport.Send(ctx, "recipient-uuid", []byte("test-payload"))
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
