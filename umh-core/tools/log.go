package tools

import (
	"fmt"

	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"go.uber.org/zap"
)

// InitLogging initializes the global logger using zap
func InitLogging() {
	logLevel, _ := env.GetAsString("LOGGING_LEVEL", false, "PRODUCTION") //nolint:errcheck
	log := logger.New(logLevel)
	defer func(zlogger *zap.SugaredLogger) {
		err := logger.Sync(zlogger)
		if err != nil {
			fmt.Printf("Failed to sync logger: %v\n", err)
		}
	}(log)
}
