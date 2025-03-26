package tracing

import (
	"fmt"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
)

// SentryTracingMiddleware is a middleware that extracts the tracecontext from the request and adds it to the context
// It also creates a span for the request and adds useful attributes to it
func SentryTracingMiddleware(c *gin.Context) {
	ctx := c.Request.Context()
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub().Clone()
		ctx = sentry.SetHubOnContext(ctx, hub)
	}

	options := []sentry.SpanOption{
		sentry.WithOpName("http.server"),
		sentry.ContinueFromRequest(c.Request),
		sentry.WithTransactionSource(sentry.SourceURL),
	}

	transaction := sentry.StartTransaction(ctx, fmt.Sprintf("%s %s", c.Request.Method, c.Request.URL.Path), options...)
	// Add Request information to the transaction
	transaction.SetData("method", c.Request.Method)
	transaction.SetData("url", c.Request.URL.String())

	// Store transaction in the context for downstream handlers
	ctx = transaction.Context()
	c.Request = c.Request.WithContext(ctx)

	// Process the request
	c.Next()

	// Post Processing of the transaction
	// Add response information to the transaction
	transaction.SetData("status_code", c.Writer.Status())
	if len(c.Errors) > 0 {
		transaction.SetData("error", c.Errors.String())
	}

	transaction.Finish()
}
