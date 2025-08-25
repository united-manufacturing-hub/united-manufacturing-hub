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

package starvationchecker

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

var _ = Describe("StarvationChecker", func() {
	var checker *StarvationChecker
	var ctx context.Context
	var cancel context.CancelFunc

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		checker = NewStarvationChecker(100 * time.Millisecond)
	})

	AfterEach(func() {
		checker.Stop()
		cancel()
	})

	Describe("Background starvation check", func() {
		It("should detect starvation when no reconciles happen", func() {
			// Wait for more than the starvation threshold
			time.Sleep(150 * time.Millisecond)

			// Verify the last reconcile time hasn't changed
			lastReconcile := checker.GetLastReconcileTime()
			Expect(time.Since(lastReconcile)).To(BeNumerically(">=", 150*time.Millisecond))
		})

		It("should update last reconcile time when Reconcile is called", func() {
			// Wait a bit
			time.Sleep(50 * time.Millisecond)

			// Call Reconcile
			_, _ = checker.Reconcile(ctx, config.FullConfig{})

			// Verify the last reconcile time was updated
			lastReconcile := checker.GetLastReconcileTime()
			Expect(time.Since(lastReconcile)).To(BeNumerically("<", 50*time.Millisecond))
		})
	})

	Describe("Reconcile method", func() {
		It("should update last reconcile time", func() {
			// Get initial time
			initialTime := checker.GetLastReconcileTime()

			// Wait a bit
			time.Sleep(50 * time.Millisecond)

			// Call Reconcile
			_, _ = checker.Reconcile(ctx, config.FullConfig{})

			// Verify the time was updated
			newTime := checker.GetLastReconcileTime()
			Expect(newTime).To(BeTemporally(">", initialTime))
		})

		It("should not detect starvation when reconciles happen frequently", func() {
			// Call Reconcile multiple times with small delays
			for range 3 {
				_, _ = checker.Reconcile(ctx, config.FullConfig{})
				time.Sleep(30 * time.Millisecond)
			}

			// Verify the last reconcile time is recent
			lastReconcile := checker.GetLastReconcileTime()
			Expect(time.Since(lastReconcile)).To(BeNumerically("<", 50*time.Millisecond))
		})
	})

	Describe("Stop method", func() {
		It("should stop the background checker", func() {
			// Get initial time
			initialTime := checker.GetLastReconcileTime()

			// Stop the checker
			checker.Stop()

			// Wait a bit
			time.Sleep(150 * time.Millisecond)

			// Verify the time hasn't changed
			newTime := checker.GetLastReconcileTime()
			Expect(newTime).To(Equal(initialTime))
		})
	})
})
