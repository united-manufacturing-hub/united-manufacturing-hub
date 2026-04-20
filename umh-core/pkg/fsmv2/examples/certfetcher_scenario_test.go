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

package examples_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
)

var _ = Describe("CertFetcher Scenario", func() {
	var ctx context.Context
	var cancel context.CancelFunc

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	})

	AfterEach(func() {
		cancel()
	})

	It("starts and fetches certs for subscribers", func() {
		result := examples.RunCertFetcherScenario(ctx, examples.CertFetcherRunConfig{
			Duration:         15 * time.Second,
			SubscriberEmails: []string{"alice@example.com", "bob@example.com"},
		})
		Expect(result.Error).NotTo(HaveOccurred())
		<-result.Done
		Expect(result.FetchCallCount).To(BeNumerically(">=", 1),
			"FetchAllCerts should have been called at least once for 2 subscribers")
	})

	It("enters degraded state after consecutive failures", func() {
		result := examples.RunCertFetcherScenario(ctx, examples.CertFetcherRunConfig{
			Duration:         20 * time.Second,
			SubscriberEmails: []string{"alice@example.com"},
			FetchError:       fmt.Errorf("simulated API failure"),
		})
		Expect(result.Error).NotTo(HaveOccurred())
		<-result.Done
		Expect(result.FetchCallCount).To(BeNumerically(">=", 3),
			"FetchAllCerts should be called at least DegradedThreshold times before degraded backoff kicks in")
	})

	It("stays in stopped state without sub handler", func() {
		result := examples.RunCertFetcherScenario(ctx, examples.CertFetcherRunConfig{
			Duration:         5 * time.Second,
			SubscriberEmails: nil, // No sub handler
		})
		Expect(result.Error).NotTo(HaveOccurred())
		<-result.Done
		Expect(result.FetchCallCount).To(Equal(0),
			"FetchAllCerts should never be called when there is no sub handler")
	})
})
