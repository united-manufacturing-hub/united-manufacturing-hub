package main

import (
	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresq-v2/database"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresq-v2/kafka"

	/* #nosec G108 -- Replace with https://github.com/felixge/fgtrace later*/
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"go.uber.org/zap"
	"net/http"
	_ "net/http/pprof"
)

var buildtime string

func main() {
	InitLoggingMetricsAndHealth()
	database.Init()
	kafka.Init()
}

func InitLoggingMetricsAndHealth() {
	// Initialize zap logging
	log := logger.New("LOGGING_LEVEL")
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)

	zap.S().Infof("This is kafka-to-postgresql build date: %s", buildtime)

	// pprof
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe("localhost:1337", nil)
		if err != nil {
			zap.S().Errorf("Error starting pprof: %s", err)
		}
	}()

	// Prometheus
	metricsPath := "/metrics"
	metricsPort := ":2112"
	zap.S().Debugf("Setting up metrics %s %v", metricsPath, metricsPort)

	http.Handle(metricsPath, promhttp.Handler())
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe(metricsPort, nil)
		if err != nil {
			zap.S().Errorf("Error starting metrics: %s", err)
		}
	}()

	health := healthcheck.NewHandler()
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(10000))
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe("0.0.0.0:8086", health)
		if err != nil {
			zap.S().Errorf("Error starting healthcheck: %s", err)
		}
	}()
}
