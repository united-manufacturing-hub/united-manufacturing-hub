package main

import (
	"fmt"
	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus"
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
	"time"
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
	metricsPort := "2112"
	zap.S().Debugf("Setting up metrics %s %v", metricsPath, metricsPort)

	http.Handle(metricsPath, promhttp.Handler())
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe(fmt.Sprintf("0.0.0.0:%s", metricsPort), nil)
		if err != nil {
			zap.S().Errorf("Error starting metrics: %s", err)
		}
	}()
	registerCustomMetrics()
}

var (
	// Define custom metrics as package-level variables
	lruHitGauge                           = prometheus.NewGauge(prometheus.GaugeOpts{Name: "lru_hit_percentage", Help: "LRU Hit Percentage"})
	numericalChannelFillGauge             = prometheus.NewGauge(prometheus.GaugeOpts{Name: "numerical_channel_fill_percentage", Help: "Numerical Channel Fill Percentage"})
	stringChannelFillGauge                = prometheus.NewGauge(prometheus.GaugeOpts{Name: "string_channel_fill_percentage", Help: "String Channel Fill Percentage"})
	databaseInsertionsCounter             = prometheus.NewGauge(prometheus.GaugeOpts{Name: "database_insertions", Help: "Total Database Insertions"})
	averageCommitDurationGauge            = prometheus.NewGauge(prometheus.GaugeOpts{Name: "average_commit_duration_milliseconds", Help: "Average Commit Duration in Milliseconds"})
	numericalValuesReceivedPerSecondGauge = prometheus.NewGauge(prometheus.GaugeOpts{Name: "numerical_values_received_per_second", Help: "Numerical Values Received Per Second"})
	stringValuesReceivedPerSecondGauge    = prometheus.NewGauge(prometheus.GaugeOpts{Name: "string_values_received_per_second", Help: "String Values Received Per Second"})
)

func registerCustomMetrics() {
	prometheus.MustRegister(lruHitGauge)
	prometheus.MustRegister(numericalChannelFillGauge)
	prometheus.MustRegister(stringChannelFillGauge)
	prometheus.MustRegister(databaseInsertionsCounter)
	prometheus.MustRegister(averageCommitDurationGauge)
	prometheus.MustRegister(numericalValuesReceivedPerSecondGauge)
	prometheus.MustRegister(stringValuesReceivedPerSecondGauge)

	// Update metrics in a separate go routine
	go func() {
		ticker10Seconds := time.NewTicker(10 * time.Second)
		for {
			metrics := postgresql.GetOrInit().GetMetrics()
			lruHitGauge.Set(metrics.LRUHitPercentage)
			numericalChannelFillGauge.Set(metrics.NumericalChannelFillPercentage)
			stringChannelFillGauge.Set(metrics.StringChannelFillPercentage)
			databaseInsertionsCounter.Set(float64(metrics.DatabaseInsertions))
			averageCommitDurationGauge.Set(metrics.AverageCommitDurationInMilliseconds)
			numericalValuesReceivedPerSecondGauge.Set(metrics.NumericalValuesReceivedPerSecond)
			stringValuesReceivedPerSecondGauge.Set(metrics.StringValuesReceivedPerSecond)
			<-ticker10Seconds.C
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
