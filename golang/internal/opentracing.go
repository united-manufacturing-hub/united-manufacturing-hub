package internal

import (
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
)

// ExtractTraceID extracts the trace ID from a given opentracing.Span
func ExtractTraceID(sp opentracing.Span) (string, bool) {
	sctx, ok := sp.Context().(jaeger.SpanContext)
	if !ok {
		return "", false
	}

	return sctx.TraceID().String(), true
}
