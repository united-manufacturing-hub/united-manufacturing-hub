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
