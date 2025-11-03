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

package v2_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v2 "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2"
	"go.uber.org/zap"
)

var _ = Describe("Login", func() {
	var logger *zap.SugaredLogger

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
	})

	Describe("NewLogin with context cancellation", func() {
		It("should return nil promptly when context is cancelled", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			start := time.Now()
			result := v2.NewLogin(ctx, "invalid-token", true, "https://invalid.example.com", logger)
			elapsed := time.Since(start)

			Expect(result).To(BeNil(), "NewLogin should return nil when context is cancelled")
			Expect(elapsed).To(BeNumerically("<", 500*time.Millisecond), "NewLogin should return promptly (< 500ms) when context is cancelled")
		})

		It("should stop retrying when context is cancelled during backoff", func() {
			ctx, cancel := context.WithCancel(context.Background())

			done := make(chan *v2.LoginResponse, 1)
			go func() {
				result := v2.NewLogin(ctx, "invalid-token", true, "https://invalid.example.com", logger)
				done <- result
			}()

			time.Sleep(100 * time.Millisecond)
			cancel()

			select {
			case result := <-done:
				Expect(result).To(BeNil(), "NewLogin should return nil when context is cancelled during retry")
			case <-time.After(2 * time.Second):
				Fail("NewLogin did not return within 2 seconds after context cancellation")
			}
		})

		It("should not leak goroutines when context is cancelled", func() {
			ctx, cancel := context.WithCancel(context.Background())

			go func() {
				time.Sleep(50 * time.Millisecond)
				cancel()
			}()

			result := v2.NewLogin(ctx, "invalid-token", true, "https://invalid.example.com", logger)

			Expect(result).To(BeNil(), "NewLogin should return nil when context is cancelled")
		})
	})
})
