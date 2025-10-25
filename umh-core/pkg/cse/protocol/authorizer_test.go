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

package protocol_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/protocol"
)

var _ = Describe("MockAuthorizer", func() {
	// Run contract tests to ensure MockAuthorizer implements Authorizer correctly
	RunAuthorizerContractTests(func() protocol.Authorizer {
		return protocol.NewMockAuthorizer()
	})

	Describe("Policy Configuration", func() {
		var (
			auth   *protocol.MockAuthorizer
			ctx    context.Context
			cancel context.CancelFunc
		)

		BeforeEach(func() {
			auth = protocol.NewMockAuthorizer()
			ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		})

		AfterEach(func() {
			cancel()
		})

		Describe("Subscription Policy", func() {
			It("should enforce subscription policy when set to deny", func() {
				// Set policy to deny
				auth.SetSubscribePolicy("user1", "workers", false)

				allowed, err := auth.CanSubscribe(ctx, "user1", "workers")
				Expect(err).NotTo(HaveOccurred())
				Expect(allowed).To(BeFalse(), "should deny when policy is set to false")
			})

			It("should enforce subscription policy when set to allow", func() {
				// Set policy to allow
				auth.SetSubscribePolicy("user1", "workers", true)

				allowed, err := auth.CanSubscribe(ctx, "user1", "workers")
				Expect(err).NotTo(HaveOccurred())
				Expect(allowed).To(BeTrue(), "should allow when policy is set to true")
			})

			It("should enforce different policies for different users", func() {
				auth.SetSubscribePolicy("user1", "workers", true)
				auth.SetSubscribePolicy("user2", "workers", false)

				allowed1, err1 := auth.CanSubscribe(ctx, "user1", "workers")
				Expect(err1).NotTo(HaveOccurred())
				Expect(allowed1).To(BeTrue())

				allowed2, err2 := auth.CanSubscribe(ctx, "user2", "workers")
				Expect(err2).NotTo(HaveOccurred())
				Expect(allowed2).To(BeFalse())
			})

			It("should enforce different policies for different collections", func() {
				auth.SetSubscribePolicy("user1", "workers", true)
				auth.SetSubscribePolicy("user1", "datapoints", false)

				allowed1, err1 := auth.CanSubscribe(ctx, "user1", "workers")
				Expect(err1).NotTo(HaveOccurred())
				Expect(allowed1).To(BeTrue())

				allowed2, err2 := auth.CanSubscribe(ctx, "user1", "datapoints")
				Expect(err2).NotTo(HaveOccurred())
				Expect(allowed2).To(BeFalse())
			})
		})

		Describe("Field Read Policy", func() {
			It("should enforce field read policy when set to deny", func() {
				auth.SetReadFieldPolicy("user1", "workers", "doc1", "temperature", false)

				allowed, err := auth.CanReadField(ctx, "user1", "workers", "doc1", "temperature")
				Expect(err).NotTo(HaveOccurred())
				Expect(allowed).To(BeFalse(), "should deny when policy is set to false")
			})

			It("should enforce field read policy when set to allow", func() {
				auth.SetReadFieldPolicy("user1", "workers", "doc1", "temperature", true)

				allowed, err := auth.CanReadField(ctx, "user1", "workers", "doc1", "temperature")
				Expect(err).NotTo(HaveOccurred())
				Expect(allowed).To(BeTrue(), "should allow when policy is set to true")
			})

			It("should enforce different policies for different fields in same document", func() {
				auth.SetReadFieldPolicy("user1", "workers", "doc1", "temperature", true)
				auth.SetReadFieldPolicy("user1", "workers", "doc1", "operator_name", false)

				allowed1, err1 := auth.CanReadField(ctx, "user1", "workers", "doc1", "temperature")
				Expect(err1).NotTo(HaveOccurred())
				Expect(allowed1).To(BeTrue())

				allowed2, err2 := auth.CanReadField(ctx, "user1", "workers", "doc1", "operator_name")
				Expect(err2).NotTo(HaveOccurred())
				Expect(allowed2).To(BeFalse())
			})

			It("should enforce different policies for same field in different documents", func() {
				auth.SetReadFieldPolicy("user1", "workers", "doc1", "temperature", true)
				auth.SetReadFieldPolicy("user1", "workers", "doc2", "temperature", false)

				allowed1, err1 := auth.CanReadField(ctx, "user1", "workers", "doc1", "temperature")
				Expect(err1).NotTo(HaveOccurred())
				Expect(allowed1).To(BeTrue())

				allowed2, err2 := auth.CanReadField(ctx, "user1", "workers", "doc2", "temperature")
				Expect(err2).NotTo(HaveOccurred())
				Expect(allowed2).To(BeFalse())
			})

			It("should enforce different policies for different users on same field", func() {
				auth.SetReadFieldPolicy("user1", "workers", "doc1", "temperature", true)
				auth.SetReadFieldPolicy("user2", "workers", "doc1", "temperature", false)

				allowed1, err1 := auth.CanReadField(ctx, "user1", "workers", "doc1", "temperature")
				Expect(err1).NotTo(HaveOccurred())
				Expect(allowed1).To(BeTrue())

				allowed2, err2 := auth.CanReadField(ctx, "user2", "workers", "doc1", "temperature")
				Expect(err2).NotTo(HaveOccurred())
				Expect(allowed2).To(BeFalse())
			})
		})

		Describe("Write Transaction Policy", func() {
			It("should enforce write transaction policy when set to deny", func() {
				auth.SetWriteTransactionPolicy("user1", "workers", "insert", false)

				allowed, err := auth.CanWriteTransaction(ctx, "user1", "workers", "insert")
				Expect(err).NotTo(HaveOccurred())
				Expect(allowed).To(BeFalse(), "should deny when policy is set to false")
			})

			It("should enforce write transaction policy when set to allow", func() {
				auth.SetWriteTransactionPolicy("user1", "workers", "insert", true)

				allowed, err := auth.CanWriteTransaction(ctx, "user1", "workers", "insert")
				Expect(err).NotTo(HaveOccurred())
				Expect(allowed).To(BeTrue(), "should allow when policy is set to true")
			})

			It("should enforce different policies for different operations", func() {
				auth.SetWriteTransactionPolicy("user1", "workers", "insert", true)
				auth.SetWriteTransactionPolicy("user1", "workers", "delete", false)

				allowed1, err1 := auth.CanWriteTransaction(ctx, "user1", "workers", "insert")
				Expect(err1).NotTo(HaveOccurred())
				Expect(allowed1).To(BeTrue())

				allowed2, err2 := auth.CanWriteTransaction(ctx, "user1", "workers", "delete")
				Expect(err2).NotTo(HaveOccurred())
				Expect(allowed2).To(BeFalse())
			})

			It("should enforce different policies for different collections", func() {
				auth.SetWriteTransactionPolicy("user1", "workers", "insert", true)
				auth.SetWriteTransactionPolicy("user1", "datapoints", "insert", false)

				allowed1, err1 := auth.CanWriteTransaction(ctx, "user1", "workers", "insert")
				Expect(err1).NotTo(HaveOccurred())
				Expect(allowed1).To(BeTrue())

				allowed2, err2 := auth.CanWriteTransaction(ctx, "user1", "datapoints", "insert")
				Expect(err2).NotTo(HaveOccurred())
				Expect(allowed2).To(BeFalse())
			})

			It("should enforce different policies for different users", func() {
				auth.SetWriteTransactionPolicy("user1", "workers", "insert", true)
				auth.SetWriteTransactionPolicy("user2", "workers", "insert", false)

				allowed1, err1 := auth.CanWriteTransaction(ctx, "user1", "workers", "insert")
				Expect(err1).NotTo(HaveOccurred())
				Expect(allowed1).To(BeTrue())

				allowed2, err2 := auth.CanWriteTransaction(ctx, "user2", "workers", "insert")
				Expect(err2).NotTo(HaveOccurred())
				Expect(allowed2).To(BeFalse())
			})
		})

		Describe("Policy Independence", func() {
			It("should not affect other permission levels when setting subscription policy", func() {
				auth.SetSubscribePolicy("user1", "workers", false)

				// Subscribe should be denied
				subAllowed, err1 := auth.CanSubscribe(ctx, "user1", "workers")
				Expect(err1).NotTo(HaveOccurred())
				Expect(subAllowed).To(BeFalse())

				// But field read and write should still be allowed (default)
				fieldAllowed, err2 := auth.CanReadField(ctx, "user1", "workers", "doc1", "temp")
				Expect(err2).NotTo(HaveOccurred())
				Expect(fieldAllowed).To(BeTrue())

				txAllowed, err3 := auth.CanWriteTransaction(ctx, "user1", "workers", "insert")
				Expect(err3).NotTo(HaveOccurred())
				Expect(txAllowed).To(BeTrue())
			})

			It("should not affect other permission levels when setting field policy", func() {
				auth.SetReadFieldPolicy("user1", "workers", "doc1", "temp", false)

				// Field read should be denied
				fieldAllowed, err1 := auth.CanReadField(ctx, "user1", "workers", "doc1", "temp")
				Expect(err1).NotTo(HaveOccurred())
				Expect(fieldAllowed).To(BeFalse())

				// But subscribe and write should still be allowed (default)
				subAllowed, err2 := auth.CanSubscribe(ctx, "user1", "workers")
				Expect(err2).NotTo(HaveOccurred())
				Expect(subAllowed).To(BeTrue())

				txAllowed, err3 := auth.CanWriteTransaction(ctx, "user1", "workers", "insert")
				Expect(err3).NotTo(HaveOccurred())
				Expect(txAllowed).To(BeTrue())
			})

			It("should not affect other permission levels when setting transaction policy", func() {
				auth.SetWriteTransactionPolicy("user1", "workers", "insert", false)

				// Write should be denied
				txAllowed, err1 := auth.CanWriteTransaction(ctx, "user1", "workers", "insert")
				Expect(err1).NotTo(HaveOccurred())
				Expect(txAllowed).To(BeFalse())

				// But subscribe and field read should still be allowed (default)
				subAllowed, err2 := auth.CanSubscribe(ctx, "user1", "workers")
				Expect(err2).NotTo(HaveOccurred())
				Expect(subAllowed).To(BeTrue())

				fieldAllowed, err3 := auth.CanReadField(ctx, "user1", "workers", "doc1", "temp")
				Expect(err3).NotTo(HaveOccurred())
				Expect(fieldAllowed).To(BeTrue())
			})
		})
	})

	Describe("Error Simulation", func() {
		var (
			auth   *protocol.MockAuthorizer
			ctx    context.Context
			cancel context.CancelFunc
		)

		BeforeEach(func() {
			auth = protocol.NewMockAuthorizer()
			ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		})

		AfterEach(func() {
			cancel()
		})

		Describe("Subscribe Failure Simulation", func() {
			It("should simulate subscribe failure with custom error", func() {
				testErr := errors.New("policy service unavailable")
				auth.SimulateSubscribeFailure(testErr)

				allowed, err := auth.CanSubscribe(ctx, "user1", "workers")
				Expect(err).To(HaveOccurred())
				Expect(allowed).To(BeFalse())

				var authErr protocol.AuthorizerError
				Expect(errors.As(err, &authErr)).To(BeTrue())
				Expect(authErr.Unwrap()).To(Equal(testErr))
			})

			It("should not affect field read and write transaction", func() {
				testErr := errors.New("policy service unavailable")
				auth.SimulateSubscribeFailure(testErr)

				// Subscribe should fail
				_, err1 := auth.CanSubscribe(ctx, "user1", "workers")
				Expect(err1).To(HaveOccurred())

				// But field read and write should succeed
				fieldAllowed, err2 := auth.CanReadField(ctx, "user1", "workers", "doc1", "temp")
				Expect(err2).NotTo(HaveOccurred())
				Expect(fieldAllowed).To(BeTrue())

				txAllowed, err3 := auth.CanWriteTransaction(ctx, "user1", "workers", "insert")
				Expect(err3).NotTo(HaveOccurred())
				Expect(txAllowed).To(BeTrue())
			})
		})

		Describe("Read Field Failure Simulation", func() {
			It("should simulate read field failure with custom error", func() {
				testErr := errors.New("field policy cache miss")
				auth.SimulateReadFieldFailure(testErr)

				allowed, err := auth.CanReadField(ctx, "user1", "workers", "doc1", "temp")
				Expect(err).To(HaveOccurred())
				Expect(allowed).To(BeFalse())

				var authErr protocol.AuthorizerError
				Expect(errors.As(err, &authErr)).To(BeTrue())
				Expect(authErr.Unwrap()).To(Equal(testErr))
			})

			It("should not affect subscribe and write transaction", func() {
				testErr := errors.New("field policy cache miss")
				auth.SimulateReadFieldFailure(testErr)

				// Field read should fail
				_, err1 := auth.CanReadField(ctx, "user1", "workers", "doc1", "temp")
				Expect(err1).To(HaveOccurred())

				// But subscribe and write should succeed
				subAllowed, err2 := auth.CanSubscribe(ctx, "user1", "workers")
				Expect(err2).NotTo(HaveOccurred())
				Expect(subAllowed).To(BeTrue())

				txAllowed, err3 := auth.CanWriteTransaction(ctx, "user1", "workers", "insert")
				Expect(err3).NotTo(HaveOccurred())
				Expect(txAllowed).To(BeTrue())
			})
		})

		Describe("Write Transaction Failure Simulation", func() {
			It("should simulate write transaction failure with custom error", func() {
				testErr := errors.New("transaction policy timeout")
				auth.SimulateWriteTransactionFailure(testErr)

				allowed, err := auth.CanWriteTransaction(ctx, "user1", "workers", "insert")
				Expect(err).To(HaveOccurred())
				Expect(allowed).To(BeFalse())

				var authErr protocol.AuthorizerError
				Expect(errors.As(err, &authErr)).To(BeTrue())
				Expect(authErr.Unwrap()).To(Equal(testErr))
			})

			It("should not affect subscribe and field read", func() {
				testErr := errors.New("transaction policy timeout")
				auth.SimulateWriteTransactionFailure(testErr)

				// Write should fail
				_, err1 := auth.CanWriteTransaction(ctx, "user1", "workers", "insert")
				Expect(err1).To(HaveOccurred())

				// But subscribe and field read should succeed
				subAllowed, err2 := auth.CanSubscribe(ctx, "user1", "workers")
				Expect(err2).NotTo(HaveOccurred())
				Expect(subAllowed).To(BeTrue())

				fieldAllowed, err3 := auth.CanReadField(ctx, "user1", "workers", "doc1", "temp")
				Expect(err3).NotTo(HaveOccurred())
				Expect(fieldAllowed).To(BeTrue())
			})
		})

		Describe("Clear Simulated Errors", func() {
			It("should clear all simulated errors", func() {
				// Set up all three error simulations
				auth.SimulateSubscribeFailure(errors.New("sub error"))
				auth.SimulateReadFieldFailure(errors.New("field error"))
				auth.SimulateWriteTransactionFailure(errors.New("tx error"))

				// Clear all errors
				auth.ClearSimulatedErrors()

				// All operations should now succeed
				subAllowed, err1 := auth.CanSubscribe(ctx, "user1", "workers")
				Expect(err1).NotTo(HaveOccurred())
				Expect(subAllowed).To(BeTrue())

				fieldAllowed, err2 := auth.CanReadField(ctx, "user1", "workers", "doc1", "temp")
				Expect(err2).NotTo(HaveOccurred())
				Expect(fieldAllowed).To(BeTrue())

				txAllowed, err3 := auth.CanWriteTransaction(ctx, "user1", "workers", "insert")
				Expect(err3).NotTo(HaveOccurred())
				Expect(txAllowed).To(BeTrue())
			})
		})

		Describe("Error vs Denial", func() {
			It("should distinguish between policy denial and check failure", func() {
				// Policy denial: (false, nil)
				auth.SetSubscribePolicy("user1", "workers", false)
				allowed1, err1 := auth.CanSubscribe(ctx, "user1", "workers")
				Expect(err1).NotTo(HaveOccurred(), "denial should not return error")
				Expect(allowed1).To(BeFalse(), "should be denied by policy")

				// Check failure: (false, AuthorizerError)
				auth.SimulateSubscribeFailure(errors.New("check failed"))
				allowed2, err2 := auth.CanSubscribe(ctx, "user1", "workers")
				Expect(err2).To(HaveOccurred(), "check failure should return error")
				Expect(allowed2).To(BeFalse())

				var authErr protocol.AuthorizerError
				Expect(errors.As(err2, &authErr)).To(BeTrue(), "error should be AuthorizerError")
			})
		})
	})
})
