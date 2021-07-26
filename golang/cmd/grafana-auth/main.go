package main

import (
	"fmt"
	"github.com/heptiolabs/healthcheck"
	"go.uber.org/zap"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var shutdownEnabled bool

func main() {

	// Setup logger and set as global
	logger, _ := zap.NewProduction()
	zap.ReplaceGlobals(logger)
	defer func(logger *zap.Logger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(logger)

	FactoryInsightAPIKey := os.Getenv("FACTORYINSIGHT_KEY")
	FactoryInputUser := os.Getenv("FACTORYINSIGHT_USER")

	health := healthcheck.NewHandler()
	shutdownEnabled = false
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(100))
	health.AddReadinessCheck("shutdownEnabled", isShutdownEnabled())
	go func() {
		err := http.ListenAndServe("0.0.0.0:8086", health)
		if err != nil {
			panic(err)
		}
	}()

	jaegerHost := os.Getenv("JAEGER_HOST")
	jaegerPort := os.Getenv("JAEGER_PORT")

	SetupRestAPI(FactoryInputUser, FactoryInsightAPIKey, jaegerHost, jaegerPort)

	zap.S().Debugf("Jaeger & REST API initialized..", jaegerHost, jaegerPort)

	// Allow graceful shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)

	go func() {
		// before you trapped SIGTERM your process would
		// have exited, so we are now on borrowed time.
		//
		// Kubernetes sends SIGTERM 30 seconds before
		// shutting down the pod.

		sig := <-sigs

		// Log the received signal
		zap.S().Infof("Recieved SIGTERM", sig)

		// ... close TCP connections here.
		ShutdownApplicationGraceful()

	}()

	select {} // block forever
}

func isShutdownEnabled() healthcheck.Check {
	return func() error {
		if shutdownEnabled {
			return fmt.Errorf("shutdown")
		}
		return nil
	}
}

func ShutdownApplicationGraceful() {
	zap.S().Infof("Shutting down application")
	shutdownEnabled = true

	time.Sleep(5 * time.Second)

	zap.S().Infof("Successfull shutdown. Exiting.")
	os.Exit(0)
}
