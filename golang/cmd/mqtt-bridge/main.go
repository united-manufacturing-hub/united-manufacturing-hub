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

	"go.uber.org/zap"
)

var localMQTTClient MQTT.Client
var remoteMQTTClient MQTT.Client

// this is only used if SSL is not enabled
const localMQTTClientID = "MQTT-BRIDGE-LOCAL"
const remoteMQTTClientID = "MQTT-BRIDGE-REMOTE"

func main() {

	// Setup logger and set as global
	logger, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	dryRun := os.Getenv("DRY_RUN")

	// Read environment variables for remote broker
	remoteCertificateName := os.Getenv("REMOTE_CERTIFICATE_NAME")
	remoteMQTTBrokerURL := os.Getenv("REMOTE_BROKER_URL")
	remoteMQTTTopic := os.Getenv("REMOTE_TOPIC")
	remoteMQTTBrokerSSLEnabled := os.Getenv("REMOTE_BROKER_SSL_ENABLED")

	// Read environment variables for local broker
	localCertificateName := os.Getenv("LOCAL_CERTIFICATE_NAME")
	localMQTTBrokerURL := os.Getenv("LOCAL_BROKER_URL")
	localMQTTTopic := os.Getenv("LOCAL_TOPIC")
	localMQTTBrokerSSLEnabled := os.Getenv("LOCAL_BROKER_SSL_ENABLED")

	zap.S().Debugf("Setting up MQTT")

	remoteMQTTClient = setupMQTT(remoteCertificateName, "remote", remoteMQTTBrokerURL, remoteMQTTTopic, remoteMQTTBrokerSSLEnabled)
	localMQTTClient = setupMQTT(localCertificateName, "local", localMQTTBrokerURL, localMQTTTopic, localMQTTBrokerSSLEnabled)

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

// ShutdownApplicationGraceful shutsdown the entire application including MQTT and database
func ShutdownApplicationGraceful() {
	zap.S().Infof("Shutting down application")

	remoteMQTTClient.Disconnect(1000)
	localMQTTClient.Disconnect(1000)

	time.Sleep(15 * time.Second) // Wait that all data is processed

	zap.S().Infof("Successfull shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}

// publishQueueToBroker starts an endless loop and publishes the given queue element by element to the selected MQTT broker.
// There can be multiple processes running of this function and to identify each of them a prefix is used
func publishQueueToBroker(pq goque.PrefixQueue, client MQTT.Client, prefix string) {
	for True {
		topElement, err := peekFirstElementAndGetID(pq, prefix)
		if err != nil {
			zap.S().Fatalf("Error peeking first element", err)
			return
		}
		err = topElement.ToObject(&currentMessage)

		token := client.Publish(currentMessage.Topic, 2, false, currentMessage.Message)
		token.Wait()

	}
}
