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
