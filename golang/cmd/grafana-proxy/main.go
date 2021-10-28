package main

import (
	"fmt"
	"github.com/heptiolabs/healthcheck"
	"go.uber.org/zap"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var shutdownEnabled bool

func main() {

	// Setup logger and set as global
	var logger *zap.Logger
	var logLevel = os.Getenv("LOGGING_LEVEL")
	switch logLevel {
	case "DEVELOPMENT":
		logger, _ = zap.NewDevelopment()
	default:
		logger, _ = zap.NewProduction()
	}
	zap.ReplaceGlobals(logger)
	defer func(logger *zap.Logger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(logger)

	FactoryInputAPIKey = os.Getenv("FACTORYINPUT_KEY")
	FactoryInputUser = os.Getenv("FACTORYINPUT_USER")
	FactoryInputBaseURL = os.Getenv("FACTORYINPUT_BASE_URL")
	FactoryInsightBaseUrl = os.Getenv("FACTORYINSIGHT_BASE_URL")

	if len(FactoryInputAPIKey) == 0 {
		zap.S().Error("Factoryinput API Key not set")
		return
	}
	if FactoryInputUser == "" {
		zap.S().Error("Factoryinput user not set")
		return
	}
	if FactoryInputBaseURL == "" {
		zap.S().Error("Factoryinput base url not set")
		return
	}
	if FactoryInsightBaseUrl == "" {
		zap.S().Error("Factoryinsight base url not set")
		return
	}

	if !strings.HasSuffix(FactoryInputBaseURL, "/") {
		FactoryInputBaseURL = fmt.Sprintf("%s/", FactoryInputBaseURL)
	}
	if !strings.HasSuffix(FactoryInsightBaseUrl, "/") {
		FactoryInsightBaseUrl = fmt.Sprintf("%s/", FactoryInsightBaseUrl)
	}

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

	var jaegerHost string
	var jaegerPort string
	if os.Getenv("DISABLE_JAEGER") == "1" || os.Getenv("DISABLE_JAEGER") == "true" {
		jaegerHost = ""
		jaegerPort = ""
	} else {
		jaegerHost = os.Getenv("JAEGER_HOST")
		jaegerPort = os.Getenv("JAEGER_PORT")

		if jaegerHost == "" || jaegerPort == "" {
			zap.S().Warn("Jaeger not configured correctly")
		}
	}

	SetupRestAPI(jaegerHost, jaegerPort)
	zap.S().Infof("Ready to proxy connections")

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

	zap.S().Infof("Successfull shutdown. Exiting.")
	os.Exit(0)
}
