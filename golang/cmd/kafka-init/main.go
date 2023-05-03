package main

import (
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"net"
	"time"
)

func main() {
	// Initialize zap logging
	logLevel, _ := env.GetAsString("LOGGING_LEVEL", false, "PRODUCTION")
	log := logger.New(logLevel)
	defer func(log *zap.SugaredLogger) {
		_ = log.Sync()
	}(log)

	internal.Initfgtrace()

	kafkaBroker, err := env.GetAsString("KAFKA_BOOTSTRAP_SERVER", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	zap.S().Infof("kaflaBroker: %s", kafkaBroker)

	timeout := 10 * time.Second
	conn, err := net.DialTimeout("tcp", kafkaBroker, timeout)
	if err != nil {
		zap.S().Errorf("Site unreachable. Error: %v", err)
	} else {
		zap.S().Info("Site reachable")
	}
	defer func(conn net.Conn) {
		err = conn.Close()
		if err != nil {
			zap.S().Errorf("Error closing connection: %s", err)
		}
	}(conn)

	Init(kafkaBroker)
}
