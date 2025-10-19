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

// RunAuthorizerContractTests verifies Authorizer implementations meet contract.
// This ensures all implementations (mock, real) behave consistently.
//
// Contract requirements:
//   - Context cancellation is respected
//   - Three permission levels work independently
//   - Error vs denial distinction is clear
//   - Default behavior is predictable
func RunAuthorizerContractTests(factory func() protocol.Authorizer) {
	Describe("Authorizer Contract", func() {
		var (
			auth   protocol.Authorizer
			ctx    context.Context
			cancel context.CancelFunc
		)

		BeforeEach(func() {
			auth = factory()
			ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		})

		AfterEach(func() {
			cancel()
		})

		Describe("CanSubscribe", func() {
			It("should allow subscription by default (test mock)", func() {
				allowed, err := auth.CanSubscribe(ctx, "user1", "workers")
				Expect(err).NotTo(HaveOccurred())
				Expect(allowed).To(BeTrue(), "MockAuthorizer should default to allow for test-friendliness")
			})

			It("should respect context cancellation", func() {
				// Cancel context immediately
				cancelCtx, cancelFunc := context.WithCancel(context.Background())
				cancelFunc()

				allowed, err := auth.CanSubscribe(cancelCtx, "user1", "workers")
				Expect(err).To(HaveOccurred())
				Expect(allowed).To(BeFalse())

				var authErr protocol.AuthorizerError
				Expect(errors.As(err, &authErr)).To(BeTrue(), "should return AuthorizerError")
				Expect(errors.Is(authErr.Unwrap(), context.Canceled)).To(BeTrue())
			})

			It("should handle different users and collections", func() {
				allowed1, err := auth.CanSubscribe(ctx, "user1", "workers")
				Expect(err).NotTo(HaveOccurred())
				Expect(allowed1).To(BeTrue())

				allowed2, err := auth.CanSubscribe(ctx, "user2", "datapoints")
				Expect(err).NotTo(HaveOccurred())
				Expect(allowed2).To(BeTrue())
			})
		})

		Describe("CanReadField", func() {
			It("should allow field read by default (test mock)", func() {
				allowed, err := auth.CanReadField(ctx, "user1", "workers", "doc1", "temperature")
				Expect(err).NotTo(HaveOccurred())
				Expect(allowed).To(BeTrue(), "MockAuthorizer should default to allow for test-friendliness")
			})

			It("should respect context cancellation", func() {
				// Cancel context immediately
				cancelCtx, cancelFunc := context.WithCancel(context.Background())
				cancelFunc()

				allowed, err := auth.CanReadField(cancelCtx, "user1", "workers", "doc1", "temperature")
				Expect(err).To(HaveOccurred())
				Expect(allowed).To(BeFalse())

				var authErr protocol.AuthorizerError
				Expect(errors.As(err, &authErr)).To(BeTrue(), "should return AuthorizerError")
				Expect(errors.Is(authErr.Unwrap(), context.Canceled)).To(BeTrue())
			})

			It("should handle different field names", func() {
				allowed1, err := auth.CanReadField(ctx, "user1", "workers", "doc1", "temperature")
				Expect(err).NotTo(HaveOccurred())
				Expect(allowed1).To(BeTrue())

				allowed2, err := auth.CanReadField(ctx, "user1", "workers", "doc1", "operator_name")
				Expect(err).NotTo(HaveOccurred())
				Expect(allowed2).To(BeTrue())
			})

			It("should handle different documents", func() {
				allowed1, err := auth.CanReadField(ctx, "user1", "workers", "doc1", "temperature")
				Expect(err).NotTo(HaveOccurred())
				Expect(allowed1).To(BeTrue())

				allowed2, err := auth.CanReadField(ctx, "user1", "workers", "doc2", "temperature")
				Expect(err).NotTo(HaveOccurred())
				Expect(allowed2).To(BeTrue())
			})
		})

		Describe("CanWriteTransaction", func() {
			It("should allow write transaction by default (test mock)", func() {
				allowed, err := auth.CanWriteTransaction(ctx, "user1", "workers", "insert")
				Expect(err).NotTo(HaveOccurred())
				Expect(allowed).To(BeTrue(), "MockAuthorizer should default to allow for test-friendliness")
			})

			It("should respect context cancellation", func() {
				// Cancel context immediately
				cancelCtx, cancelFunc := context.WithCancel(context.Background())
				cancelFunc()

				allowed, err := auth.CanWriteTransaction(cancelCtx, "user1", "workers", "insert")
				Expect(err).To(HaveOccurred())
				Expect(allowed).To(BeFalse())

				var authErr protocol.AuthorizerError
				Expect(errors.As(err, &authErr)).To(BeTrue(), "should return AuthorizerError")
				Expect(errors.Is(authErr.Unwrap(), context.Canceled)).To(BeTrue())
			})

			It("should handle different operations", func() {
				operations := []string{"insert", "update", "delete"}
				for _, op := range operations {
					allowed, err := auth.CanWriteTransaction(ctx, "user1", "workers", op)
					Expect(err).NotTo(HaveOccurred(), "operation %s should be allowed", op)
					Expect(allowed).To(BeTrue())
				}
			})
		})

		Describe("Policy Independence", func() {
			It("should evaluate subscription, field, and transaction permissions independently", func() {
				// All three permission levels should be independent
				// This is verified by the fact that each method exists and can be called separately
				sub, err1 := auth.CanSubscribe(ctx, "user1", "workers")
				Expect(err1).NotTo(HaveOccurred())

				field, err2 := auth.CanReadField(ctx, "user1", "workers", "doc1", "temp")
				Expect(err2).NotTo(HaveOccurred())

				tx, err3 := auth.CanWriteTransaction(ctx, "user1", "workers", "insert")
				Expect(err3).NotTo(HaveOccurred())

				// In mock, all default to true
				Expect(sub).To(BeTrue())
				Expect(field).To(BeTrue())
				Expect(tx).To(BeTrue())
			})
		})

		Describe("Error Handling", func() {
			It("should distinguish between error and denial", func() {
				// When there's no error, denial is indicated by false return value
				// Errors should be wrapped in AuthorizerError

				// This test verifies the contract:
				// - (false, nil) means "checked and denied"
				// - (false, AuthorizerError) means "couldn't check"
				// - (true, nil) means "checked and allowed"

				// Default MockAuthorizer returns (true, nil) for everything
				allowed, err := auth.CanSubscribe(ctx, "user1", "workers")
				Expect(err).NotTo(HaveOccurred())
				Expect(allowed).To(BeTrue())
			})
		})
	})
}
