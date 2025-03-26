package tracing

import (
	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
)

// AddTraceUUIDToSpan adds a trace UUID to a Sentry span for correlation
func AddTraceUUIDToSpan(span *sentry.Span, traceID uuid.UUID) {
	if span == nil || traceID == uuid.Nil {
		return
	}
	span.SetTag("trace-id", traceID.String())
}

/*
	# Distributed tracing @ UMH

	## Frontend

	Each message generates a random trace-id and adds it to the UMHMessage.

	## Backend

	Each transaction uses AddTraceUUIDToSpan to add the trace-id to the span, making it possible to search for the trace-id in Sentry.

	## Companion

	Each transaction uses AddTraceUUIDToSpan to add the trace-id to the span, making it possible to search for the trace-id in Sentry.
*/
