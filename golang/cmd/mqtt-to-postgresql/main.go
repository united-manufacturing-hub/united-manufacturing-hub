package main

/*
Important principles: stateless as much as possible
*/

import (
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

func main() {

	// Setup logger and set as global
	logger, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	// Read environment variables
	certificateName := os.Getenv("CERTIFICATE_NAME")
	mqttBrokerURL := os.Getenv("BROKER_URL")

	PQHost := os.Getenv("POSTGRES_HOST")
	PQPort := 5432
	PQUser := os.Getenv("POSTGRES_USER")
	PQPassword := os.Getenv("POSTGRES_PASSWORD")
	PWDBName := os.Getenv("POSTGRES_DATABASE")
	SSLMODE := os.Getenv("POSTGRES_SSLMODE")

	zap.S().Debugf("######################################################################################## Starting program..", PQHost, PQUser, PWDBName)

	// Prometheus
	metricsPath := "/metrics"
	metricsPort := ":2112"
	zap.S().Debugf("Setting up metrics", metricsPath, metricsPort)

	http.Handle(metricsPath, promhttp.Handler())
	go http.ListenAndServe(metricsPort, nil)

	// Prometheus
	zap.S().Debugf("Setting up healthcheck")

	health := healthcheck.NewHandler()
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(100))
	go http.ListenAndServe("0.0.0.0:8086", health)

	dryRun := os.Getenv("DRY_RUN")

	// Redis cache
	redisURI := os.Getenv("REDIS_URI")
	redisURI2 := os.Getenv("REDIS_URI2")
	redisURI3 := os.Getenv("REDIS_URI3")
	redisPassword := os.Getenv("REDIS_PASSWORD")
	redisDB := 0 // default database

	internal.InitCache(redisURI, redisURI2, redisURI3, redisPassword, redisDB, dryRun)

	zap.S().Debugf("Setting up database")

	SetupDB(PQUser, PQPassword, PWDBName, PQHost, PQPort, health, SSLMODE, dryRun)
	// Setting up queues
	zap.S().Debugf("Setting up queues")

	pg, err := setupQueue()
	if err != nil {
		zap.S().Errorf("Error setting up remote queue", err)
		return
	}
	defer closeQueue(pg)

	zap.S().Debugf("Setting up MQTT")
	podName := os.Getenv("MY_POD_NAME")
	mqttTopic := os.Getenv("MQTT_TOPIC")
	SetupMQTT(certificateName, mqttBrokerURL, mqttTopic, health, podName, pg)

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

	// Start queue processing goroutines
	go reportQueueLength(pg)
	go storeIntoDatabaseRoutineAddMaintenanceActivity(pg)
	go storeIntoDatabaseRoutineAddOrder(pg)
	go storeIntoDatabaseRoutineAddProduct(pg)
	go storeIntoDatabaseRoutineCount(pg)
	go storeIntoDatabaseRoutineEndOrder(pg)
	go storeIntoDatabaseRoutineProcessValue(pg)
	go storeIntoDatabaseRoutineProcessValueFloat64(pg)
	go storeIntoDatabaseRoutineRecommendation(pg)
	go storeIntoDatabaseRoutineScrapCount(pg)
	go storeIntoDatabaseRoutineShift(pg)
	go storeIntoDatabaseRoutineStartOrder(pg)
	go storeIntoDatabaseRoutineState(pg)
	go storeIntoDatabaseRoutineUniqueProduct(pg)
	go storeIntoDatabaseRoutineUniqueProductScrap(pg)
	go storeIntoDatabaseRoutineProductTag(pg)
	go storeIntoDatabaseRoutineProductTagString(pg)
	go storeIntoDatabaseRoutineAddParentToChild(pg)
	select {} // block forever
}

// ShutdownApplicationGraceful shutsdown the entire application including MQTT and database
func ShutdownApplicationGraceful() {
	zap.S().Infof("Shutting down application")
	ShutdownMQTT()

	time.Sleep(15 * time.Second) // Wait that all data is processed

	ShutdownDB()

	zap.S().Infof("Successfull shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}
