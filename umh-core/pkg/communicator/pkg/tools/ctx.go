package tools

import (
	"context"
	"time"
)

// Get10SecondContext returns a context with a 10-second timeout
// This is useful for testing
func Get10SecondContext() (context.Context, context.CancelFunc) {
	return GetXDurationContext(10 * time.Second)
}

// Get1SecondContext returns a context with a 1-second timeout
// This is used for immediate feedback
func Get1SecondContext() (context.Context, context.CancelFunc) {
	return GetXDurationContext(time.Second)
}

// Get3SecondContext returns a context with a 3-second timeout
// This (Or the shorter Get1SecondContext) should be used for most gathering functions
func Get3SecondContext() (context.Context, context.CancelFunc) {
	return GetXDurationContext(time.Second * 3)
}

// GetXDurationContext returns a context with a timeout of the given duration
func GetXDurationContext(duration time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), duration)
}
