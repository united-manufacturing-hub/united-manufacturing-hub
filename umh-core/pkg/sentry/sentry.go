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
	if appVersion == "" {
		appVersion = "0.0.0"
	}

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

func createSentryEvent(level sentry.Level, err error) *sentry.Event {
	event := sentry.NewEvent()
	event.Level = level
	event.Message = err.Error()
	event.SetException(err, 1)

	// Capture all goroutines and convert them to Sentry threads
	if level == sentry.LevelFatal || level == sentry.LevelError {
		threads, stacktrace := captureGoroutinesAsThreads()
		event.Threads = threads
		event.Attachments = append(event.Attachments, &sentry.Attachment{
			Filename:    "stacktrace.txt",
			ContentType: "text/plain",
			Payload:     stacktrace,
		})
	}

	return event
}

// Helper function to send an event to Sentry
func sendSentryEvent(event *sentry.Event) {
	localHub := sentry.CurrentHub().Clone()
	localHub.CaptureEvent(event)
}
