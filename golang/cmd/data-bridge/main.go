package main

import (
	"net/http"

	"github.com/heptiolabs/healthcheck"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

func main() {
	var err error
	logLevel, _ := env.GetAsString("LOGGING_LEVEL", false, "PRODUCTION") //nolint:errcheck
	log := logger.New(logLevel)
	defer func(logger *zap.SugaredLogger) {
		err = logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)

	internal.Initfgtrace()

	zap.S().Debug("Checking environment variables")
	serialNumber, err := env.GetAsString("SERIAL_NUMBER", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	microserviceName, err := env.GetAsString("MICROSERVICE_NAME", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}

	mode, err := env.GetAsInt("MODE", true, -1)
	if err != nil {
		zap.S().Fatal(err)
	}
	if mode != 0 && mode != 1 {
		zap.S().Fatal("invalid MODE")
	}
	bridgeMode, err := env.GetAsInt("BRIDGE_MODE", true, -1)
	if err != nil {
		zap.S().Fatal(err)
	}
	if bridgeMode != 0 && bridgeMode != 1 {
		zap.S().Fatal("invalid BRIDGE_MODE")
	}

	brokerA, err := env.GetAsString("BROKER_A", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	brokerB, err := env.GetAsString("BROKER_B", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}

	topic, err := env.GetAsString("TOPIC", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	split, err := env.GetAsInt("SPLIT", true, -1)
	if err != nil {
		zap.S().Fatal(err)
	}

	partitons, err := env.GetAsInt("PARTITIONS", false, 6)
	if err != nil {
		zap.S().Error(err)
	}
	if partitons < 1 {
		zap.S().Fatal("PARTITIONS must be at least 1")
	}
	replicationFactor, err := env.GetAsInt("REPLICATION_FACTOR", false, 1)
	if err != nil {
		zap.S().Error(err)
	}
	if replicationFactor % 2 == 0 {
		zap.S().Fatal("REPLICATION_FACTOR must be odd")
	}

	zap.S().Debug("Starting healthcheck")
	health := healthcheck.NewHandler()
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(1000000))
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe("0.0.0.0:8086", health)
		if err != nil {
			zap.S().Errorf("Error starting healthcheck: %s", err)
		}
	}()

	gs := internal.NewGracefulShutdown(func() error {
		// close the clients
		return nil
	})
}
