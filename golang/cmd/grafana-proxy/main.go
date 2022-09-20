package main

import (
	"fmt"
	"github.com/heptiolabs/healthcheck"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"go.uber.org/zap"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var shutdownEnabled bool

var buildtime string

func main() {
	// Initialize zap logging
	log := logger.New("LOGGING_LEVEL")
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)
	zap.S().Infof("This is grafana-proxy build date: %s", buildtime)
	// pprof
	go http.ListenAndServe("localhost:1337", nil)

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

	SetupRestAPI()
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
		zap.S().Infof("Received SIGTERM", sig)

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

	zap.S().Infof("Successful shutdown. Exiting.")
	os.Exit(0)
}
