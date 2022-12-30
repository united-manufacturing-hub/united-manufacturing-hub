package main

import (
	"time"

	"go.uber.org/zap"
)

func CalculateMessageProcessingLag(originalTimestamp uint64) {

	// Convert the timestamp to a time.Time object
	originalTimestampUnix := time.Unix(0, int64(originalTimestamp*uint64(1000000)))

	// Get the current Unix timestamp
	currentTimestampUnix := time.Now().UnixNano()

	// Calculate the difference between the two timestamps
	processingLag := currentTimestampUnix - originalTimestampUnix.UnixNano()

	// Convert the processing lag to milliseconds
	processingLagMilliseconds := processingLag / 1000000

	// If the processing lag is greater than 10000 milliseconds, log it
	if processingLagMilliseconds > 10000 {
		zap.S().Warnf("Message processing lag is greater than 10 seconds: %d. This might indicate a problem in the network connectivity and/or data processing", processingLagMilliseconds)
	}

	// Print the processing lag as debug
	zap.S().Debugf("Message processing lag: %d", processingLagMilliseconds)
}
