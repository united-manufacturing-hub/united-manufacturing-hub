package protocol_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/protocol"
)

var _ = Describe("Crypto Interface", func() {
	Describe("MockCrypto", func() {
		Context("Contract tests", func() {
			RunCryptoContractTests(func() protocol.Crypto {
				return protocol.NewMockCrypto()
			})
		})

		Context("Mock-specific features", func() {
			var (
				ctx    context.Context
				crypto *protocol.MockCrypto
			)

			BeforeEach(func() {
				ctx = context.Background()
				crypto = protocol.NewMockCrypto()
			})

			It("should simulate encryption failures", func() {
				testErr := errors.New("simulated encryption error")
				crypto.SimulateEncryptFailure(testErr)

				plaintext := []byte("test message")
				_, err := crypto.Encrypt(ctx, plaintext, "recipient-123")
				Expect(err).To(HaveOccurred())

				var encErr protocol.CryptoEncryptionError
				Expect(errors.As(err, &encErr)).To(BeTrue())
				Expect(encErr.Unwrap()).To(Equal(testErr))
			})

			It("should simulate decryption failures", func() {
				plaintext := []byte("test message")
				ciphertext, err := crypto.Encrypt(ctx, plaintext, "recipient-123")
				Expect(err).NotTo(HaveOccurred())

				testErr := errors.New("simulated decryption error")
				crypto.SimulateDecryptFailure(testErr)

				_, err = crypto.Decrypt(ctx, ciphertext, "sender-456")
				Expect(err).To(HaveOccurred())

				var decErr protocol.CryptoDecryptionError
				Expect(errors.As(err, &decErr)).To(BeTrue())
				Expect(decErr.Unwrap()).To(Equal(testErr))
			})

			It("should simulate authentication failures", func() {
				plaintext := []byte("test message")
				ciphertext, err := crypto.Encrypt(ctx, plaintext, "recipient-123")
				Expect(err).NotTo(HaveOccurred())

				crypto.SimulateAuthFailure()

				_, err = crypto.Decrypt(ctx, ciphertext, "sender-456")
				Expect(err).To(HaveOccurred())

				var authErr protocol.CryptoAuthenticationError
				Expect(errors.As(err, &authErr)).To(BeTrue())
			})

			It("should clear simulated errors", func() {
				testErr := errors.New("simulated error")
				crypto.SimulateEncryptFailure(testErr)

				crypto.ClearSimulatedErrors()

				plaintext := []byte("test message")
				ciphertext, err := crypto.Encrypt(ctx, plaintext, "recipient-123")
				Expect(err).NotTo(HaveOccurred())

				decrypted, err := crypto.Decrypt(ctx, ciphertext, "sender-456")
				Expect(err).NotTo(HaveOccurred())
				Expect(decrypted).To(Equal(plaintext))
			})

			It("should generate different nonces for idempotency", func() {
				plaintext := []byte("same message")

				ct1, err := crypto.Encrypt(ctx, plaintext, "recipient-123")
				Expect(err).NotTo(HaveOccurred())

				ct2, err := crypto.Encrypt(ctx, plaintext, "recipient-123")
				Expect(err).NotTo(HaveOccurred())

				Expect(ct1).NotTo(Equal(ct2))
			})
		})
	})
})
