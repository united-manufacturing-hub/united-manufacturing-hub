package main

/*
Important principles: stateless as much as possible
*/

import (
	"github.com/beeker1121/goque"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
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

	//dryRun := os.Getenv("DRY_RUN")

	// Read environment variables for remote broker
	remoteCertificateName := os.Getenv("REMOTE_CERTIFICATE_NAME")
	remoteMQTTBrokerURL := os.Getenv("REMOTE_BROKER_URL")
	remoteMQTTTopic := os.Getenv("REMOTE_TOPIC")
	remoteMQTTBrokerSSLEnabled, err := strconv.ParseBool(os.Getenv("REMOTE_BROKER_SSL_ENABLED"))
	if err != nil {
		zap.S().Errorf("Error parsing bool from environment variable", err)
		return
	}

	// Read environment variables for local broker
	localCertificateName := os.Getenv("LOCAL_CERTIFICATE_NAME")
	localMQTTBrokerURL := os.Getenv("LOCAL_BROKER_URL")
	localMQTTTopic := os.Getenv("LOCAL_TOPIC")
	localMQTTBrokerSSLEnabled, err := strconv.ParseBool(os.Getenv("LOCAL_BROKER_SSL_ENABLED"))
	if err != nil {
		zap.S().Errorf("Error parsing bool from environment variable", err)
		return
	}

	// Setting up queues

	zap.S().Debugf("Setting up queues")

	remotePg, err := setupQueue("remote")
	if err != nil {
		zap.S().Errorf("Error setting up remote queue", err)
		return
	}
	defer closeQueue(remotePg)

	localPg, err := setupQueue("local")
	if err != nil {
		zap.S().Errorf("Error setting up local queue", err)
		return
	}
	defer closeQueue(localPg)

	// Setting up MQTT
	zap.S().Debugf("Setting up MQTT")

	remoteMQTTClient = setupMQTT(remoteCertificateName, "remote", remoteMQTTBrokerURL, remoteMQTTTopic, remoteMQTTBrokerSSLEnabled, remotePg)
	localMQTTClient = setupMQTT(localCertificateName, "local", localMQTTBrokerURL, localMQTTTopic, localMQTTBrokerSSLEnabled, localPg)

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
func publishQueueToBroker(pq *goque.PrefixQueue, client MQTT.Client, prefix string) {
	for {
		// get first element and convert it to object
		topElement, err := pq.Peek([]byte(prefix))
		if err != nil {
			zap.S().Errorf("Error peeking first element", err)
			return
		}

		var currentMessage queueObject
		err = topElement.ToObject(&currentMessage)
		if err != nil {
			zap.S().Fatalf("Error decoding first element", err)
			return
		}

		// Publish element and wait for confirmation
		token := client.Publish(currentMessage.Topic, 1, false, currentMessage.Message)
		token.Wait() // wait indefinete amount of time (librarz will automatically resend)

		// if successfully recieved at broker delete from stack
		topElement, err = pq.Dequeue([]byte(prefix))
		if err != nil {
			zap.S().Fatalf("Error dequeuing element", err)
			return
		}
	}
}
