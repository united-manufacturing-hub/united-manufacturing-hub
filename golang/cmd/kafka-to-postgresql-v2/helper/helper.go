package helper

import (
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
)

func InitLogging() {
	logLevel, _ := env.GetAsString("LOGGING_LEVEL", false, "PRODUCTION") //nolint:errcheck
	_ = logger.New(logLevel)
}

func InitTestLogging() {
	_ = logger.New("DEVELOPMENT")
}
