package main

/*
Important principles: stateless as much as possible
*/

/*
Target architecture:

Incoming REST call --> http.go
There is one function for that specific call. It parses the parameters and executes further functions:
1. One or multiple function getting the data from the database (database.go)
2. Only one function processing everything. In this function no database calls are allowed to be as stateless as possible (dataprocessing.go)
Then the results are bundled together and a return JSON is created.
*/

import (
	"fmt"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/gin-gonic/gin"
	"github.com/heptiolabs/healthcheck"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"go.uber.org/zap"
	"net/http"

	/* #nosec G108 -- Replace with https://github.com/felixge/fgtrace later*/
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

var shutdownEnabled bool
var mqttClient MQTT.Client
var buildtime string

// GetEnv get's env variable, and also outputs warning if not set
func GetEnv(variableName string) (envValue string) {
	if len(variableName) == 0 {
		zap.S().Warnf("Attempting to get env variable without name")
	}
	envValue = os.Getenv(variableName)
	if len(envValue) == 0 {
		zap.S().Warnf("Env variable %s is empty", variableName)
	}
	return
}

func main() {
	// Initialize zap logging
	log := logger.New("LOGGING_LEVEL")
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)
	zap.S().Infof("This is factoryinput build date: %s", buildtime)
	// pprof
	go func() {
		err := http.ListenAndServe("localhost:1337", nil)
		if err != nil {
			zap.S().Errorf("Failed to start pprof", err)
		}
	}()

	shutdownEnabled = false

	// Loading up user accounts
	accounts := gin.Accounts{}

	zap.S().Debugf("Loading accounts from environment..")

	for i := 1; i <= 100; i++ {
		tempUser := os.Getenv("CUSTOMER_NAME_" + strconv.Itoa(i))
		tempPassword := os.Getenv("CUSTOMER_PASSWORD_" + strconv.Itoa(i))
		if tempUser != "" && tempPassword != "" {
			zap.S().Infof("Added account for " + tempUser)
			accounts[tempUser] = tempPassword
		}
	}
	if len(accounts) == 0 {
		zap.S().Warnf("No customer accounts set up")
	}

	// also add admin access
	RESTUser := GetEnv("FACTORYINPUT_USER")
	RESTPassword := GetEnv("FACTORYINPUT_PASSWORD")
	accounts[RESTUser] = RESTPassword

	// get currentVersion
	version := GetEnv("VERSION")

	zap.S().Debugf("Starting program..")

	health := healthcheck.NewHandler()
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(10000))
	health.AddReadinessCheck("shutdownEnabled", isShutdownEnabled())
	go func() {
		err := http.ListenAndServe("0.0.0.0:8086", health)
		if err != nil {
			zap.S().Errorf("Failed to bind healthcheck to port", err)
			ShutdownApplicationGraceful()
		}
	}()

	zap.S().Debugf("Healthcheck initialized..")

	// Setup queue
	err := setupQueue()
	if err != nil {
		zap.S().Errorf("Error setting up remote queue", err)
		return
	}
	defer func() {
		err = closeQueue()
		if err != nil {
			zap.S().Errorf("Failed to close queue, might be corrupted !", err)
		}
	}()

	// Read environment variables
	certificateName := GetEnv("CERTIFICATE_NAME")
	mqttBrokerURL := GetEnv("BROKER_URL")

	// Setup MQTT
	zap.S().Debugf("Setting up MQTT")
	podName := GetEnv("MY_POD_NAME")
	SetupMQTT(certificateName, mqttBrokerURL, podName)
	zap.S().Debugf("Finished setting up MQTT")

	// Setup rest
	zap.S().Debugf("SetupRestAPI")
	go SetupRestAPI(accounts, version)

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

	mqttQueueHandler := GetEnv("MQTT_QUEUE_HANDLER")
	iMqttQueueHandler, err := strconv.Atoi(mqttQueueHandler)
	if err != nil {
		zap.S().Warnf("Failed to read MQTT_QUEUE_HANDLER, defaulting to 10")
		iMqttQueueHandler = 10
	}
	for i := 0; i < iMqttQueueHandler; i++ {
		zap.S().Debugf("Starting MQTT handlers")
		go MqttQueueHandler()
	}

	zap.S().Debugf("Started %d MqTTQueueHandlers", iMqttQueueHandler)
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

// ShutdownApplicationGraceful shutsdown the entire application including MQTT and database
func ShutdownApplicationGraceful() {
	zap.S().Infof("Shutting down application")
	shutdownEnabled = true
	time.Sleep(15 * time.Second) // Wait until all remaining open connections are handled
	ShutdownMQTT()
	time.Sleep(1000 * time.Millisecond)

	// Check if MQTT client is still connected
	for i := 0; i < 10; i++ {
		if mqttClient.IsConnected() {
			zap.S().Warnf("MQTT client is still connected !")
			time.Sleep(10 * time.Second)
		} else {
			break
		}
	}

	if mqttClient.IsConnected() {
		zap.S().Errorf("Graceful shutdown of MQTT client failed !")
	}

	// Close queue

	if err := closeQueue(); err != nil {
		zap.S().Errorf("Failed to shutdown queue", err)
	}

	zap.S().Infof("Successful shutdown. Exiting.")
	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}
