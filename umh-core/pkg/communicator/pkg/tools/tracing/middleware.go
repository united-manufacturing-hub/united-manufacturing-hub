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
