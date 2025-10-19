package protocol_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/protocol"
)

func RunCryptoContractTests(factory func() protocol.Crypto) {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		crypto protocol.Crypto
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		crypto = factory()
	})

	AfterEach(func() {
		cancel()
	})

	Describe("Crypto Contract", func() {
		Describe("Encrypt/Decrypt roundtrip", func() {
			It("should encrypt and decrypt roundtrip", func() {
				plaintext := []byte("secret message")
				ciphertext, err := crypto.Encrypt(ctx, plaintext, "recipient-123")
				Expect(err).NotTo(HaveOccurred())
				Expect(ciphertext).NotTo(BeNil())

				decrypted, err := crypto.Decrypt(ctx, ciphertext, "sender-456")
				Expect(err).NotTo(HaveOccurred())
				Expect(decrypted).To(Equal(plaintext))
			})

			It("should produce different ciphertext for same plaintext (nonce/IV)", func() {
				plaintext := []byte("same message")
				ct1, err := crypto.Encrypt(ctx, plaintext, "recipient-123")
				Expect(err).NotTo(HaveOccurred())

				ct2, err := crypto.Encrypt(ctx, plaintext, "recipient-123")
				Expect(err).NotTo(HaveOccurred())

				Expect(ct1).NotTo(Equal(ct2))
			})

			It("should handle empty plaintext", func() {
				plaintext := []byte("")
				ciphertext, err := crypto.Encrypt(ctx, plaintext, "recipient-123")
				Expect(err).NotTo(HaveOccurred())

				decrypted, err := crypto.Decrypt(ctx, ciphertext, "sender-456")
				Expect(err).NotTo(HaveOccurred())
				Expect(decrypted).To(Equal(plaintext))
			})

			It("should handle large plaintexts", func() {
				plaintext := make([]byte, 10000)
				for i := range plaintext {
					plaintext[i] = byte(i % 256)
				}

				ciphertext, err := crypto.Encrypt(ctx, plaintext, "recipient-123")
				Expect(err).NotTo(HaveOccurred())

				decrypted, err := crypto.Decrypt(ctx, ciphertext, "sender-456")
				Expect(err).NotTo(HaveOccurred())
				Expect(decrypted).To(Equal(plaintext))
			})
		})

		Describe("Context cancellation", func() {
			It("should respect context cancellation in Encrypt", func() {
				cancelledCtx, cancel := context.WithCancel(context.Background())
				cancel()

				plaintext := []byte("test message")
				_, err := crypto.Encrypt(cancelledCtx, plaintext, "recipient-123")
				Expect(err).To(HaveOccurred())

				var encErr protocol.CryptoEncryptionError
				Expect(err).To(BeAssignableToTypeOf(encErr))
			})

			It("should respect context cancellation in Decrypt", func() {
				plaintext := []byte("test message")
				ciphertext, err := crypto.Encrypt(ctx, plaintext, "recipient-123")
				Expect(err).NotTo(HaveOccurred())

				cancelledCtx, cancel := context.WithCancel(context.Background())
				cancel()

				_, err = crypto.Decrypt(cancelledCtx, ciphertext, "sender-456")
				Expect(err).To(HaveOccurred())

				var decErr protocol.CryptoDecryptionError
				Expect(err).To(BeAssignableToTypeOf(decErr))
			})

			It("should respect context timeout in Encrypt", func() {
				timeoutCtx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
				defer cancel()

				time.Sleep(10 * time.Millisecond)

				plaintext := []byte("test message")
				_, err := crypto.Encrypt(timeoutCtx, plaintext, "recipient-123")
				Expect(err).To(HaveOccurred())
			})
		})

		Describe("Authentication", func() {
			It("should detect tampered ciphertext", func() {
				plaintext := []byte("secret message")
				ciphertext, err := crypto.Encrypt(ctx, plaintext, "recipient-123")
				Expect(err).NotTo(HaveOccurred())

				if len(ciphertext) > 20 {
					tamperedCiphertext := make([]byte, len(ciphertext))
					copy(tamperedCiphertext, ciphertext)
					tamperedCiphertext[20] ^= 0xFF

					_, err = crypto.Decrypt(ctx, tamperedCiphertext, "sender-456")
					Expect(err).To(HaveOccurred())

					var authErr protocol.CryptoAuthenticationError
					Expect(err).To(BeAssignableToTypeOf(authErr))
				}
			})

			It("should reject invalid ciphertext format", func() {
				invalidCiphertext := []byte("invalid data")
				_, err := crypto.Decrypt(ctx, invalidCiphertext, "sender-456")
				Expect(err).To(HaveOccurred())

				var authErr protocol.CryptoAuthenticationError
				Expect(err).To(BeAssignableToTypeOf(authErr))
			})

			It("should reject empty ciphertext", func() {
				emptyCiphertext := []byte("")
				_, err := crypto.Decrypt(ctx, emptyCiphertext, "sender-456")
				Expect(err).To(HaveOccurred())

				var authErr protocol.CryptoAuthenticationError
				Expect(err).To(BeAssignableToTypeOf(authErr))
			})
		})

		Describe("Error types", func() {
			It("should return typed errors", func() {
				invalidCiphertext := []byte("invalid")
				_, err := crypto.Decrypt(ctx, invalidCiphertext, "sender-456")
				Expect(err).To(HaveOccurred())

				var authErr protocol.CryptoAuthenticationError
				Expect(err).To(BeAssignableToTypeOf(authErr))
			})
		})
	})
}
