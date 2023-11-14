package main

import (
	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/postgresql"
	"go.uber.org/zap"
	"net/http"
)

func main() {
	InitLogging()
	InitPrometheus()
	InitHealthCheck()
	err := postgresql.Init()
	if err !=nil{
		zap.S().Fatalf("Failed to setup postgresql: %v"
	}

}

func InitLogging() {
	logLevel, _ := env.GetAsString("LOGGING_LEVEL", false, "PRODUCTION") //nolint:errcheck
	_ = logger.New(logLevel)
}

func InitPrometheus() {
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
}

func InitHealthCheck() {

	zap.S().Debugf("Setting up healthcheck")

	health := healthcheck.NewHandler()
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(1000000))
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe("0.0.0.0:8086", health)
		if err != nil {
			zap.S().Errorf("Error starting healthcheck: %s", err)
		}
	}()

}
