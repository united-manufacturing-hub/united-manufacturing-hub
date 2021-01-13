package ginopentracing

import (
	"net/http"
	"runtime"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

// StartSpan will start a new span with no parent span.
func StartSpan(operationName, method, path string) opentracing.Span {
	return StartSpanWithParent(nil, operationName, method, path)
}

// StartDBSpanWithParent - start a DB operation span
func StartDBSpanWithParent(parent opentracing.SpanContext, operationName, dbInstance, dbType, dbStatement string) opentracing.Span {
	options := []opentracing.StartSpanOption{opentracing.Tag{Key: ext.SpanKindRPCServer.Key, Value: ext.SpanKindRPCServer.Value}}
	if len(dbInstance) > 0 {
		options = append(options, opentracing.Tag{Key: string(ext.DBInstance), Value: dbInstance})
	}
	if len(dbType) > 0 {
		options = append(options, opentracing.Tag{Key: string(ext.DBType), Value: dbType})
	}
	if len(dbStatement) > 0 {
		options = append(options, opentracing.Tag{Key: string(ext.DBStatement), Value: dbStatement})
	}
	if parent != nil {
		options = append(options, opentracing.ChildOf(parent))
	}

	return opentracing.StartSpan(operationName, options...)
}

// StartSpanWithParent will start a new span with a parent span.
// example:
//      span:= StartSpanWithParent(c.Get("tracing-context"),
func StartSpanWithParent(parent opentracing.SpanContext, operationName, method, path string) opentracing.Span {
	options := []opentracing.StartSpanOption{
		opentracing.Tag{Key: ext.SpanKindRPCServer.Key, Value: ext.SpanKindRPCServer.Value},
		opentracing.Tag{Key: string(ext.HTTPMethod), Value: method},
		opentracing.Tag{Key: string(ext.HTTPUrl), Value: path},
		opentracing.Tag{Key: "current-goroutines", Value: runtime.NumGoroutine()},
	}

	if parent != nil {
		options = append(options, opentracing.ChildOf(parent))
	}

	return opentracing.StartSpan(operationName, options...)
}

// StartSpanWithHeader will look in the headers to look for a parent span before starting the new span.
// example:
//  func handleGet(c *gin.Context) {
//     span := StartSpanWithHeader(&c.Request.Header, "api-request", method, path)
//     defer span.Finish()
//     c.Set("tracing-context", span) // add the span to the context so it can be used for the duration of the request.
//     bosePersonID := c.Param("bosePersonID")
//     span.SetTag("bosePersonID", bosePersonID)
//
func StartSpanWithHeader(header *http.Header, operationName, method, path string) opentracing.Span {
	var wireContext opentracing.SpanContext
	if header != nil {
		wireContext, _ = opentracing.GlobalTracer().Extract(opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(*header))
	}
	span := StartSpanWithParent(wireContext, operationName, method, path)
	span.SetTag("current-goroutines", runtime.NumGoroutine())
	return span
	// return StartSpanWithParent(wireContext, operationName, method, path)
}

// InjectTraceID injects the span ID into the provided HTTP header object, so that the
// current span will be propogated downstream to the server responding to an HTTP request.
// Specifying the span ID in this way will allow the tracing system to connect spans
// between servers.
//
//  Usage:
//          // resty example
// 	    r := resty.R()
//	    injectTraceID(span, r.Header)
//	    resp, err := r.Get(fmt.Sprintf("http://localhost:8000/users/%s", bosePersonID))
//
//          // galapagos_clients example
//          c := galapagos_clients.GetHTTPClient()
//          req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:8000/users/%s", bosePersonID))
//          injectTraceID(span, req.Header)
//          c.Do(req)
func InjectTraceID(ctx opentracing.SpanContext, header http.Header) {
	opentracing.GlobalTracer().Inject(
		ctx,
		opentracing.HTTPHeaders,
		opentracing.HTTPHeadersCarrier(header))
}
