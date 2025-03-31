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
)

// InitSentryTracer initializes Sentry tracing
// It returns a TracerProvider that should be shut down when the application exits
func InitSentryTracer(env, appVersion string) error {
	sampleRate := 0.2 // 20% of transactions. Although it is set 0.2, it will also respect the parent-based sampling.
	if env == "production" {
		sampleRate = 0.2 // 20% of transactions in production
	}

	if env == "staging" {
		sampleRate = 0.1 // 10% of transactions in staging
	}

	if env == "development" {
		sampleRate = 1.0 // 100% of transactions in development
	}

	return sentry.Init(sentry.ClientOptions{
		Dsn:              "https://f399a5e3cdc262339e1fc36e541bb6f9@o4507265932394496.ingest.de.sentry.io/4507265944453200",
		Debug:            true,
		AttachStacktrace: true,
		ServerName:       "backend",
		Release:          appVersion,
		Environment:      env, // TODO: Disable this in production once we see traces for backend is working
		// Reduce the sample rate to avoid rate limiting
		TracesSampleRate: sampleRate,
		EnableTracing:    true,
	})
}
