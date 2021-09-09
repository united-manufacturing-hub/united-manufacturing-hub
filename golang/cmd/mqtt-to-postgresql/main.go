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

var addOrderHandler AddOrderHandler
var addParentToChildHandler AddParentToChildHandler
var addProductHandler AddProductHandler
var addShiftHandler AddShiftHandler
var countHandler CountHandler
var deleteShiftByAssetIdAndBeginTimestampHandler DeleteShiftByAssetIdAndBeginTimestampHandler
var deleteShiftByIdHandler DeleteShiftByIdHandler
var endOrderHandler EndOrderHandler
var maintenanceActivityHandler MaintenanceActivityHandler
var modifyProducedPieceHandler ModifyProducedPieceHandler
var modifyStateHandler ModifyStateHandler
var productTagHandler ProductTagHandler
var recommendationDataHandler RecommendationDataHandler
var scrapCountHandler ScrapCountHandler
var scrapUniqueProductHandler ScrapUniqueProductHandler
var startOrderHandler StartOrderHandler
var productTagStringHandler ProductTagStringHandler
var stateHandler StateHandler
var uniqueProductHandler UniqueProductHandler
var valueDataHandler ValueDataHandler
var valueStringHandler ValueStringHandler

func main() {
	// Setup logger and set as global
	var logger *zap.Logger
	if os.Getenv("LOGGING_LEVEL") == "DEBUG" {
		logger, _ = zap.NewDevelopment()
	} else {

		logger, _ = zap.NewProduction()
	}
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
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(1000000))
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

	addOrderHandler = *NewAddOrderHandler()
	addParentToChildHandler = *NewAddParentToChildHandler()
	addProductHandler = *NewAddProductHandler()
	addShiftHandler = *NewAddShiftHandler()
	countHandler = *NewCountHandler()
	deleteShiftByAssetIdAndBeginTimestampHandler = *NewDeleteShiftByAssetIdAndBeginTimestampHandler()
	deleteShiftByIdHandler = *NewDeleteShiftByIdHandler()
	endOrderHandler = *NewEndOrderHandler()
	maintenanceActivityHandler = *NewMaintenanceActivityHandler()
	modifyProducedPieceHandler = *NewModifyProducedPieceHandler()
	modifyStateHandler = *NewModifyStateHandler()
	productTagHandler = *NewProductTagHandler()
	recommendationDataHandler = *NewRecommendationDataHandler()
	scrapCountHandler = *NewScrapCountHandler()
	scrapUniqueProductHandler = *NewScrapUniqueProductHandler()
	startOrderHandler = *NewStartOrderHandler()
	productTagStringHandler = *NewProductTagStringHandler()
	stateHandler = *NewStateHandler()
	uniqueProductHandler = *NewUniqueProductHandler()
	valueDataHandler = *NewValueDataHandler()
	valueStringHandler = *NewValueStringHandler()

	addOrderHandler.Setup()
	addParentToChildHandler.Setup()
	addProductHandler.Setup()
	addShiftHandler.Setup()
	countHandler.Setup()
	deleteShiftByAssetIdAndBeginTimestampHandler.Setup()
	deleteShiftByIdHandler.Setup()
	endOrderHandler.Setup()
	maintenanceActivityHandler.Setup()
	modifyProducedPieceHandler.Setup()
	modifyStateHandler.Setup()
	productTagHandler.Setup()
	recommendationDataHandler.Setup()
	scrapCountHandler.Setup()
	scrapUniqueProductHandler.Setup()
	startOrderHandler.Setup()
	productTagStringHandler.Setup()
	stateHandler.Setup()
	uniqueProductHandler.Setup()
	valueDataHandler.Setup()
	valueStringHandler.Setup()

	zap.S().Debugf("Setting up MQTT")
	podName := os.Getenv("MY_POD_NAME")
	mqttTopic := os.Getenv("MQTT_TOPIC")
	SetupMQTT(certificateName, mqttBrokerURL, mqttTopic, health, podName)

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

// Set to true to stop processDBQueue goroutines
var shuttingDown = false

// ShutdownApplicationGraceful shuts down the entire application including MQTT and database
func ShutdownApplicationGraceful() {
	if shuttingDown {
		//Already shutting down
		return
	}
	shuttingDown = true
	zap.S().Debugf(
		`
   _____   _               _         _                                                         _   _                  _     _                    _____                                 __           _ 
  / ____| | |             | |       | |                                /\                     | | (_)                | |   (_)                  / ____|                               / _|         | |
 | (___   | |__    _   _  | |_    __| |   ___   __      __  _ __      /  \     _ __    _ __   | |  _    ___    __ _  | |_   _    ___    _ __   | |  __   _ __    __ _    ___    ___  | |_   _   _  | |
  \___ \  | '_ \  | | | | | __|  / _| |  / _ \  \ \ /\ / / | '_ \    / /\ \   | '_ \  | '_ \  | | | |  / __|  / _| | | __| | |  / _ \  | '_ \  | | |_ | | '__|  / _| |  / __|  / _ \ |  _| | | | | | |
  ____) | | | | | | |_| | | |_  | (_| | | (_) |  \ V  V /  | | | |  / ____ \  | |_) | | |_) | | | | | | (__  | (_| | | |_  | | | (_) | | | | | | |__| | | |    | (_| | | (__  |  __/ | |   | |_| | | |
 |_____/  |_| |_|  \__,_|  \__|  \__,_|  \___/    \_/\_/   |_| |_| /_/    \_\ | .__/  | .__/  |_| |_|  \___|  \__,_|  \__| |_|  \___/  |_| |_|  \_____| |_|     \__,_|  \___|  \___| |_|    \__,_| |_|
                                                                              | |     | |                                                                                                             
                                                                              |_|     |_|
`)

	zap.S().Infof("Shutting down application")
	ShutdownMQTT()

	time.Sleep(15 * time.Second) // Wait that all data is processed

	err := addOrderHandler.Shutdown()
	if err != nil {
		return
	}
	err = addParentToChildHandler.Shutdown()
	if err != nil {
		return
	}
	err = addProductHandler.Shutdown()
	if err != nil {
		return
	}
	err = addShiftHandler.Shutdown()
	if err != nil {
		return
	}
	err = countHandler.Shutdown()
	if err != nil {
		return
	}
	err = deleteShiftByAssetIdAndBeginTimestampHandler.Shutdown()
	if err != nil {
		return
	}
	err = deleteShiftByIdHandler.Shutdown()
	if err != nil {
		return
	}
	err = endOrderHandler.Shutdown()
	if err != nil {
		return
	}
	err = maintenanceActivityHandler.Shutdown()
	if err != nil {
		return
	}
	err = modifyProducedPieceHandler.Shutdown()
	if err != nil {
		return
	}
	err = modifyStateHandler.Shutdown()
	if err != nil {
		return
	}
	err = productTagHandler.Shutdown()
	if err != nil {
		return
	}
	err = recommendationDataHandler.Shutdown()
	if err != nil {
		return
	}
	err = scrapCountHandler.Shutdown()
	if err != nil {
		return
	}
	err = scrapUniqueProductHandler.Shutdown()
	if err != nil {
		return
	}
	err = startOrderHandler.Shutdown()
	if err != nil {
		return
	}
	err = productTagStringHandler.Shutdown()
	if err != nil {
		return
	}
	err = stateHandler.Shutdown()
	if err != nil {
		return
	}
	err = uniqueProductHandler.Shutdown()
	if err != nil {
		return
	}
	err = valueDataHandler.Shutdown()
	if err != nil {
		return
	}
	err = valueStringHandler.Shutdown()
	if err != nil {
		return
	}

	time.Sleep(15 * time.Second) // Wait that all data is processed

	ShutdownDB()

	zap.S().Infof("Successfull shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}
