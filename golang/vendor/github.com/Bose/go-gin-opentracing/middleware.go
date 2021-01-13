package ginopentracing

import (
	"github.com/gin-gonic/gin"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

// OpenTracer - middleware that addes opentracing
func OpenTracer(operationPrefix []byte) gin.HandlerFunc {
	if operationPrefix == nil {
		operationPrefix = []byte("api-request-")
	}
	return func(c *gin.Context) {
		// all before request is handled
		var span opentracing.Span
		if cspan, ok := c.Get("tracing-context"); ok {
			span = StartSpanWithParent(cspan.(opentracing.Span).Context(), string(operationPrefix)+c.Request.Method, c.Request.Method, c.Request.URL.Path)

		} else {
			span = StartSpanWithHeader(&c.Request.Header, string(operationPrefix)+c.Request.Method, c.Request.Method, c.Request.URL.Path)
		}
		defer span.Finish()            // after all the other defers are completed.. finish the span
		c.Set("tracing-context", span) // add the span to the context so it can be used for the duration of the request.
		c.Next()

		span.SetTag(string(ext.HTTPStatusCode), c.Writer.Status())
	}
}
