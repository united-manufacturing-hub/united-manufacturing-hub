// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"net/http"

	"github.com/united-manufacturing-hub/umh-utils/env"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

var shutdownEnabled bool
var mqttClient MQTT.Client

func main() {
	// Initialize zap logging
	logLevel, _ := env.GetAsString("LOGGING_LEVEL", false, "PRODUCTION") //nolint:errcheck
	log := logger.New(logLevel)
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)

	internal.Initfgtrace()

	shutdownEnabled = false

	// Loading up user accounts
	accounts := gin.Accounts{}

	zap.S().Debugf("Loading accounts from environment..")

	for i := 1; i <= 100; i++ {
		tempUser, err := env.GetAsString("CUSTOMER_NAME_"+strconv.Itoa(i), false, "")
		if err != nil {
			zap.S().Error(err)
		}
		tempPassword, err := env.GetAsString("CUSTOMER_PASSWORD_"+strconv.Itoa(i), false, "")
		if err != nil {
			zap.S().Error(err)
		}
		if tempUser != "" && tempPassword != "" {
			zap.S().Infof("Added account for " + tempUser)
			accounts[tempUser] = tempPassword
		}
	}
	if len(accounts) == 0 {
		zap.S().Warnf("No customer accounts set up")
	}

	// also add admin access
	RESTUser, err := env.GetAsString("FACTORYINPUT_USER", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	RESTPassword, err := env.GetAsString("FACTORYINPUT_PASSWORD", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	accounts[RESTUser] = RESTPassword

	// get currentVersion
	version, err := env.GetAsString("VERSION", false, "1")
	if err != nil {
		zap.S().Error(err)
	}

	zap.S().Debugf("Starting program..")

	health := healthcheck.NewHandler()
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(10000))
	health.AddReadinessCheck("shutdownEnabled", isShutdownEnabled())
	go func() {
		/* #nosec G114 */
		err = http.ListenAndServe("0.0.0.0:8086", health)
		if err != nil {
			zap.S().Errorf("Failed to bind healthcheck to port", err)
			ShutdownApplicationGraceful()
		}
	}()

	zap.S().Debugf("Healthcheck initialized..")

	// Setup queue
	err = setupQueue()
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
	certificateName, err := env.GetAsString("CERTIFICATE_NAME", false, "USE_TLS")
	if err != nil {
		zap.S().Error(err)
	}
	mqttBrokerURL, err := env.GetAsString("BROKER_URL", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}

	// Setup MQTT
	zap.S().Debugf("Setting up MQTT")
	podName, err := env.GetAsString("MY_POD_NAME", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	mqttPassword, err := env.GetAsString("MQTT_PASSWORD", false, "")
	if err != nil {
		zap.S().Error(err)
	}

	SetupMQTT(certificateName, mqttBrokerURL, podName, mqttPassword)
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

	mqttQueueHandler, err := env.GetAsInt("MQTT_QUEUE_HANDLER", false, 10)
	if err != nil {
		zap.S().Error(err)
	}
	for i := 0; i < mqttQueueHandler; i++ {
		zap.S().Debugf("Starting MQTT handlers")
		go MqttQueueHandler()
	}

	zap.S().Debugf("Started %d MqTTQueueHandlers", mqttQueueHandler)
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
