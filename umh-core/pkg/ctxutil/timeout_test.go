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

package ctxutil_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/ctxutil"
)

var _ = Describe("HasSufficientTime", func() {
	// These tests validate the HasSufficientTime function which is designed to check
	// if a context has enough time remaining before its deadline to perform an operation.
	// Importantly, insufficient time is not treated as an error condition - it simply
	// returns sufficient=false with nil error, allowing callers to gracefully skip operations
	// rather than treating timeout as a failure condition.
	It("should return error for context with no deadline", func() {
		ctx := context.Background()
		remaining, sufficient, err := ctxutil.HasSufficientTime(ctx, time.Millisecond*10)

		Expect(sufficient).To(BeFalse(), "Expected insufficient time for context without deadline")
		Expect(err).To(MatchError(ctxutil.ErrNoDeadline))
		Expect(remaining).To(Equal(time.Duration(0)))
	})

	It("should return sufficient=true for context with enough time", func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		remaining, sufficient, err := ctxutil.HasSufficientTime(ctx, time.Millisecond*100)

		Expect(sufficient).To(BeTrue(), "Expected sufficient time")
		Expect(err).ToNot(HaveOccurred(), "Expected no error")
		Expect(remaining).To(BeNumerically(">", 0), "Expected positive remaining time")
	})

	It("should return no error for context with insufficient time", func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*5)
		defer cancel()

		// Wait to ensure time is nearly expired
		time.Sleep(time.Millisecond * 2)

		remaining, sufficient, err := ctxutil.HasSufficientTime(ctx, time.Millisecond*10)

		Expect(sufficient).To(BeFalse(), "Expected insufficient time")
		Expect(err).ToNot(HaveOccurred(), "Expected no error")
		Expect(remaining).To(BeNumerically("<", time.Millisecond*10))
	})
})
