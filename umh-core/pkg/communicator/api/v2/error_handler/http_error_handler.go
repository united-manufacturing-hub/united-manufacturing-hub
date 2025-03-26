package error_handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/united-manufacturing-hub/ManagementConsole/shared/tools/fail"
	"github.com/united-manufacturing-hub/ManagementConsole/shared/tools/safejson"
)

// Package error_handler provides HTTP error handling with intelligent error reporting based on error types.
// It implements a mechanism to handle transient errors differently from permanent errors, reducing noise
// in error reporting while ensuring critical issues are not missed.

/*
HTTPErrorHandler Design Philosophy:

1. Error Classification:
   - Distinguishes between transient (temporary) and permanent errors
   - Transient errors are those that may resolve themselves with a retry
   - Permanent errors indicate systemic issues requiring immediate attention

2. Smart Error Reporting:
   - Transient errors are initially suppressed to reduce noise
   - Only reported after exceeding a threshold, indicating a potential systemic issue
   - Permanent errors are always reported immediately
   - Helps maintain a balance between error visibility and alert fatigue

*/

// HTTPErrorContext contains all relevant information about an HTTP error
type HTTPErrorContext struct {
	Method      string
	Endpoint    string
	StatusCode  int
	Error       error
	RequestBody interface{}
	Response    []byte
	Timestamp   time.Time
	Headers     map[string][]string // Response headers
	Duration    time.Duration       // Request duration
}

// transientErrorCodes defines HTTP status codes that typically indicate temporary issues.
// These errors often resolve themselves and should be handled with a retry mechanism.
var transientErrorCodes = []int{
	408, // Request Timeout - Server didn't receive complete request in time; retry may succeed if network conditions improve
	421, // Misdirected Request - Request was sent to a server unable to produce response; retry may reach correct server
	422, // Unprocessable Entity - Often indicates temporary validation failure; retry with corrected state may succeed
	425, // Too Early - Server unwilling to process potential replay request; retry after some delay may be accepted
	429, // Too Many Requests - Rate limit exceeded; retry after respecting rate limit will likely succeed
	500, // Internal Server Error - Temporary server error or crash; retry may succeed once server recovers
	502, // Bad Gateway - Upstream server returned invalid response; retry may succeed when upstream service stabilizes
	503, // Service Unavailable - Server temporary overload or maintenance; retry after backoff will likely succeed
	504, // Gateway Timeout - Upstream server didn't respond in time; retry may succeed when network/server conditions improve
}

// transientErrorThreshold defines how many times a transient error can occur before being reported.
// This helps prevent alert fatigue while still catching persistent issues.
const transientErrorThreshold = 10

// transientErrorCountMap tracks the occurrence count of each transient error code.
// The map is protected by a mutex to ensure thread-safe access.
var (
	transientErrorCountMap    = make(map[int]int)
	transientErrorCountMapMux sync.Mutex
)

// ReportHTTPErrors processes and reports HTTP errors with detailed context
func ReportHTTPErrors(err error, status int, endpoint string, method string, requestBody interface{}, responseBody []byte) {
	ctx := HTTPErrorContext{
		Method:      method,
		Endpoint:    endpoint,
		StatusCode:  status,
		Error:       err,
		RequestBody: requestBody,
		Response:    responseBody,
		Timestamp:   time.Now(),
	}

	attachments := buildErrorAttachments(ctx)
	errorMessage := buildErrorMessage(ctx)
	additionalContext := buildErrorContext(ctx)

	// Always report invalid status codes
	if status < 200 || status > 999 {
		fail.ErrorBatchedfWithAttachmentAndContext(errorMessage, attachments, additionalContext)
		return
	}

	// Handle transient errors
	if slices.Contains(transientErrorCodes, status) {
		transientErrorCountMapMux.Lock()
		transientErrorCountMap[status]++
		count := transientErrorCountMap[status]
		transientErrorCountMapMux.Unlock()

		if count >= transientErrorThreshold {
			fail.ErrorBatchedfWithAttachmentAndContext(
				fmt.Sprintf("[HTTP Error] %s (occurred %d times)", errorMessage, count),
				attachments,
				additionalContext,
			)
		}
		return
	}

	// Report permanent errors immediately
	fail.ErrorBatchedfWithAttachmentAndContext(errorMessage, attachments, additionalContext)
}

// buildErrorMessage creates a detailed error message
func buildErrorMessage(ctx HTTPErrorContext) string {
	return fmt.Sprintf("%s %s - Status: %d - Error: %v",
		ctx.Method,
		ctx.Endpoint,
		ctx.StatusCode,
		ctx.Error,
	)
}

// buildErrorAttachments creates error attachments with relevant debugging information
func buildErrorAttachments(ctx HTTPErrorContext) []sentry.Attachment {
	var attachments []sentry.Attachment

	// Add request body if available
	if ctx.RequestBody != nil {
		if requestJSON, err := safejson.Marshal(ctx.RequestBody); err == nil {
			attachments = append(attachments, sentry.Attachment{
				Filename:    "request.json",
				ContentType: "application/json",
				Payload:     requestJSON,
			})
		}
	}

	// Add response body if available
	if len(ctx.Response) > 0 {
		// Try to pretty print if it's JSON
		var prettyJSON bytes.Buffer
		if err := json.Indent(&prettyJSON, ctx.Response, "", "  "); err == nil {
			attachments = append(attachments, sentry.Attachment{
				Filename:    "response.json",
				ContentType: "application/json",
				Payload:     prettyJSON.Bytes(),
			})
		} else {
			// If not JSON, attach as plain text
			attachments = append(attachments, sentry.Attachment{
				Filename:    "response.txt",
				ContentType: "text/plain",
				Payload:     ctx.Response,
			})
		}
	}

	// Add error context as a separate attachment
	contextMap := map[string]interface{}{
		"timestamp": ctx.Timestamp.Format(time.RFC3339),
		"method":    ctx.Method,
		"endpoint":  ctx.Endpoint,
		"status":    ctx.StatusCode,
		"error":     ctx.Error.Error(),
	}

	if contextJSON, err := safejson.Marshal(contextMap); err == nil {
		attachments = append(attachments, sentry.Attachment{
			Filename:    "error_context.json",
			ContentType: "application/json",
			Payload:     contextJSON,
		})
	}

	return attachments
}

// buildErrorContext creates a map of additional context information
func buildErrorContext(ctx HTTPErrorContext) map[string]sentry.Context {
	return map[string]sentry.Context{
		"HTTP": sentry.Context{
			"method":      ctx.Method,
			"endpoint":    ctx.Endpoint,
			"status_code": ctx.StatusCode,
			"timestamp":   ctx.Timestamp.Format(time.RFC3339),
			"duration_ms": ctx.Duration.Milliseconds(),
		},
	}
}

// ResetErrorCounter resets all error counters to zero.
// This is called by successful actions, to reset the error counters.
func ResetErrorCounter() {
	transientErrorCountMapMux.Lock()
	for k := range transientErrorCountMap {
		delete(transientErrorCountMap, k)
	}
	transientErrorCountMapMux.Unlock()
}
