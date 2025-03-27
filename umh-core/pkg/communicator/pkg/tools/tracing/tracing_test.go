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

package tracing_test

import (
	"context"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTracing(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tracing Suite")
}

var _ = Describe("Tracing", func() {
	actionUUID := uuid.New().String()
	BeforeEach(func() {
		// Initialize Sentry with test configuration
		err := sentry.Init(sentry.ClientOptions{
			Dsn:              "https://abc@staging.management.umh.app/sentry",
			EnableTracing:    true,
			TracesSampleRate: 1.0, // Always sample in tests
			Environment:      "development",
			Debug:            true,
			AttachStacktrace: true,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		sentry.Flush(2 * time.Second) // Ensure all events are sent before closing
		GinkgoWriter.Printf("Action UUID: %s\n", actionUUID)
	})

	It("should generate a trace correctly", func() {
		generateTrace(actionUUID)
		time.Sleep(1 * time.Second)
		generateTrace(actionUUID)
	})

})

func generateTrace(actionUUID string) {
	hub := sentry.CurrentHub().Clone()
	ctx := sentry.SetHubOnContext(context.Background(), hub)

	options := []sentry.SpanOption{
		sentry.WithOpName("test-transactionT"),
		sentry.WithDescription("test-transactionD"),
		sentry.WithTransactionSource(sentry.SourceCustom),
	}

	span := sentry.StartTransaction(ctx, "test-transaction123", options...)

	// Add a child span
	childSpan := span.StartChild("action")
	childSpan.SetTag("action-uuid", actionUUID)
	childSpan.Finish()

	span.Finish()
}
