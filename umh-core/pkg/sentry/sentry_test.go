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

package sentry_test

import (
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
)

var _ = Describe("Sentry Integration", func() {
	var logger *zap.SugaredLogger

	BeforeEach(func() {
		// Create a test logger
		testLogger := zaptest.NewLogger(GinkgoT())
		logger = testLogger.Sugar()
	})

	// This test is focused (F) so it can be run manually
	// Use: go test -v -ginkgo.focus "FManually sends a test message to Sentry" ./pkg/sentry
	It("Manually sends a test message to Sentry", func() {
		Skip("Skipping Sentry test")
		// Initialize Sentry with a test version
		sentry.InitSentry("0.0.0-test")

		// Generate a unique test message with timestamp
		testMessage := fmt.Sprintf("Sentry test message at %s", time.Now().Format(time.RFC3339))
		testError := errors.New(testMessage)

		By("Sending a warning via ReportIssue")
		sentry.ReportIssue(testError, sentry.IssueTypeWarning, logger)

		By("Sending an error via ReportIssue")
		sentry.ReportIssue(testError, sentry.IssueTypeError, logger)

		By("Sending a formatted message via ReportIssuef")
		sentry.ReportIssuef(sentry.IssueTypeWarning, logger, "Formatted test message: %s", testMessage)

		// Flush to ensure messages are sent before test completes
		// Sleep to allow Sentry to process the messages
		time.Sleep(5 * time.Second)

		// This test doesn't actually assert anything as we're just checking
		// if messages appear in the Sentry dashboard
		Expect(true).To(BeTrue(), "Test completed - check Sentry dashboard for messages")

		// Print instructions for the user
		fmt.Println("\n==================================================")
		fmt.Println("Check your Sentry dashboard for these test messages:")
		fmt.Println("- Warning issue:", testError.Error())
		fmt.Println("- Error issue:", testError.Error())
		fmt.Println("- Formatted warning:", "Formatted test message: "+testMessage)
		fmt.Println("==================================================")
	})
})
