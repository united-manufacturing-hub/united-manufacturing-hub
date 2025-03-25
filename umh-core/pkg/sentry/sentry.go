package sentry

import (
	"os"
	"strconv"

	"github.com/Masterminds/semver/v3"
	"github.com/getsentry/sentry-go"
	"go.uber.org/zap"
)

// InitSentry initializes sentry with the given app name and version
func InitSentry(appVersion string) {

	environment := "development"
	sampleRate := 0.3

	version, err := semver.NewVersion(appVersion)
	if err != nil {
		zap.S().Errorf("Failed to parse app version, using default environment (development): %s", err)
	} else {
		if version.Prerelease() == "" {
			environment = "production"
			sampleRate = 0.1
		}
	}

	if rate := os.Getenv("SENTRY_SAMPLE_RATE"); rate != "" {
		if parsedRate, err := strconv.ParseFloat(rate, 64); err == nil {
			sampleRate = parsedRate
		}
	}

	err = sentry.Init(sentry.ClientOptions{
		Dsn:              "https://abc@staging.management.umh.app/sentry",
		Environment:      environment,
		Release:          "umhcore@" + appVersion,
		EnableTracing:    true,
		TracesSampleRate: sampleRate,
		TracesSampler: func(ctx sentry.SamplingContext) float64 {
			// If this transaction has a parent, we should respect the parent's sampling decision
			if ctx.Parent != nil && ctx.Parent.Sampled != sentry.SampledUndefined {
				if ctx.Parent.Sampled == sentry.SampledTrue {
					return 1.0
				}
				return 0.0
			}
			// Otherwise, use our configured sample rate
			return sampleRate
		},
	})
	if err != nil {
		zap.S().Error("Failed to initialize Sentry: %s", err)
		return
	}
}
