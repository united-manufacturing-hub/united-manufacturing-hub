package main

import (
	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/postgresql"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/worker"
	"go.uber.org/zap"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	InitLogging()
	InitPrometheus()
	_ = postgresql.GetOrInit()
	_ = kafka.GetOrInit()
	InitHealthCheck()
	_ = worker.GetOrInit()

	awaitShutdown()
	// We should never get to this await, but better to have it then to always close the program
	select {}
}

func awaitShutdown() {
	// Allow graceful shutdown
	sigs := make(chan os.Signal, 1)
	// It's important to handle both signals, allowing Kafka to shut down gracefully !
	// If this is not possible, it will attempt to rebalance itself, which will increase startup time
	signal.Notify(sigs, syscall.SIGTERM)

	sig := <-sigs
	// Log the received signal
	zap.S().Infof("Received SIG %v", sig)

	zap.S().Debugf("Shutting down kafka")
	kafka.GetOrInit().Close()
	os.Exit(0)
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

	health.AddReadinessCheck("database", postgresql.GetHealthCheck())
	health.AddLivenessCheck("database", postgresql.GetHealthCheck())
	health.AddReadinessCheck("kafka", kafka.GetReadinessCheck())
	health.AddLivenessCheck("kafka", kafka.GetLivenessCheck())
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe("0.0.0.0:8086", health)
		if err != nil {
			zap.S().Errorf("Error starting healthcheck: %s", err)
		}
	}()

}
