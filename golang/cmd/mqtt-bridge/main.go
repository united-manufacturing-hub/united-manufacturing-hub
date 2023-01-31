package main

/*
Important principles: stateless as much as possible
*/

import (
	"github.com/beeker1121/goque"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var localMQTTClient MQTT.Client
var remoteMQTTClient MQTT.Client

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

	zap.S().Infof("This is mqtt-bridge build date: %s", buildtime)

	internal.Initfgtrace()

	// dryRun := os.Getenv("DRY_RUN")

	// Read environment variables for remote broker
	remoteCertificateName, remotecertificateNameEnvSet := os.LookupEnv("REMOTE_CERTIFICATE_NAME")
	if !remotecertificateNameEnvSet {
		zap.S().Fatal("Remote certificate name (REMOTE_CERTIFICATE_NAME) must be set")
	}
	remoteMQTTBrokerURL, remoteMQTTBrokerURLEnvSet := os.LookupEnv("REMOTE_BROKER_URL")
	if !remoteMQTTBrokerURLEnvSet {
		zap.S().Fatal("Remote MQTT Broker URL (REMOTE_BROKER_URL) must be set")
	}
	remoteSubMQTTTopic, remoteSubMQTTTopicEnvSet := os.LookupEnv("REMOTE_SUB_TOPIC")
	if !remoteSubMQTTTopicEnvSet {
		zap.S().Fatal("Remote sub mqtt topic (REMOTE_SUB_TOPIC) must be set")
	}
	remotePubMQTTTopic, remotePubMQTTTopicEnvSet := os.LookupEnv("REMOTE_PUB_TOPIC")
	if !remotePubMQTTTopicEnvSet {
		zap.S().Fatal("Remote pub mqtt topic (REMOTE_PUB_TOPIC) must be set")
	}
	remoteMQTTPassword, remoteMQTTPasswordEnvSet := os.LookupEnv("REMOTE_BROKER_PASSWORD")
	if !remoteMQTTPasswordEnvSet {
		zap.S().Fatal("Remote mqtt password (REMOTE_BROKER_PASSWORD) must be set")
	}
	remoteMQTTBrokerSSLEnabled, err := strconv.ParseBool(os.Getenv("REMOTE_BROKER_SSL_ENABLED"))
	if err != nil {
		zap.S().Errorf("Error parsing bool from environment variable", err)
		return
	}

	// Read environment variables for local broker
	localCertificateName, localCertificateNameEnvSet := os.LookupEnv("LOCAL_CERTIFICATE_NAME")
	if !localCertificateNameEnvSet {
		zap.S().Fatal("Local certificate name (LOCAL_CERTIFICATE_NAME) must be set")
	}
	localMQTTBrokerURL, localMQTTBrokerURLEnvSet := os.LookupEnv("LOCAL_BROKER_URL")
	if !localMQTTBrokerURLEnvSet {
		zap.S().Fatal("Local mqtt broker URL (LOCAL_BROKER_URL) must be set")
	}
	localSubMQTTTopic, localSubMQTTTopicsEnvSet := os.LookupEnv("LOCAL_SUB_TOPIC")
	if !localSubMQTTTopicsEnvSet {
		zap.S().Fatal("Local sub mqtt topic (LOCAL_SUB_TOPIC) must be set")
	}
	localPubMQTTTopic, localPubMQTTTopicEnvSet := os.LookupEnv("LOCAL_PUB_TOPIC")
	if !localPubMQTTTopicEnvSet {
		zap.S().Fatal("Local pub mqtt topic (LOCAL_PUB_TOPIC) must be set")
	}
	localMQTTPassword, localMQTTPasswordEnvSet := os.LookupEnv("LOCAL_BROKER_PASSWORD")
	if !localMQTTPasswordEnvSet {
		zap.S().Fatal("Local mqtt password (LOCAL_BROKER_PASSWORD) must be set")
	}
	localMQTTBrokerSSLEnabled, err := strconv.ParseBool(os.Getenv("LOCAL_BROKER_SSL_ENABLED"))
	if err != nil {
		zap.S().Errorf("Error parsing bool from environment variable", err)
		return
	}

	BRIDGE_ONE_WAY, err := strconv.ParseBool(os.Getenv("BRIDGE_ONE_WAY"))
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
	defer func(pq *goque.Queue) {
		err = closeQueue(pq)
		if err != nil {
			zap.S().Errorf("Error closing remote queue", err)
		}
	}(remotePg)

	localPg, err := setupQueue("local")
	if err != nil {
		zap.S().Errorf("Error setting up local queue", err)
		return
	}
	defer func(pq *goque.Queue) {
		err = closeQueue(pq)
		if err != nil {
			zap.S().Errorf("Error closing local queue", err)
		}
	}(localPg)

	// Setting up MQTT
	zap.S().Debugf("Setting up MQTT")

	remoteMQTTClient = setupMQTT(
		remoteCertificateName,
		"remote",
		remoteMQTTBrokerURL,
		remoteSubMQTTTopic,
		remoteMQTTBrokerSSLEnabled,
		remotePg,
		!BRIDGE_ONE_WAY, remoteMQTTPassword) // make remote subscription dependent on variable
	localMQTTClient = setupMQTT(
		localCertificateName,
		"local",
		localMQTTBrokerURL,
		localSubMQTTTopic,
		localMQTTBrokerSSLEnabled,
		localPg,
		true, localMQTTPassword) // always subscribe to local

	// Setting up endless loops to send out messages
	go publishQueueToBroker(remotePg, localMQTTClient, "local", remoteSubMQTTTopic, localPubMQTTTopic)
	go publishQueueToBroker(localPg, remoteMQTTClient, "remote", localSubMQTTTopic, remotePubMQTTTopic)

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

// ShutdownApplicationGraceful shutsdown the entire application including MQTT and database
func ShutdownApplicationGraceful() {
	zap.S().Infof("Shutting down application")

	remoteMQTTClient.Disconnect(1000)
	localMQTTClient.Disconnect(1000)

	time.Sleep(15 * time.Second) // Wait that all data is processed

	zap.S().Infof("Successful shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}

// publishQueueToBroker starts an endless loop and publishes the given queue element by element to the selected MQTT broker.
// There can be multiple processes running of this function and to identify each of them a prefix is used
func publishQueueToBroker(
	pq *goque.Queue,
	client MQTT.Client,
	prefix string,
	subMQTTTopic string,
	pubMQTTTopic string) {
	for {
		if pq.Length() == 0 {
			time.Sleep(1 * time.Millisecond) // wait 1 ms to avoid high cpu usage
			continue
		}

		// get first element and convert it to object
		topElement, err := pq.Peek()
		if err != nil {
			zap.S().Errorf("Error peeking first element", err, prefix, pq.Length())
			return
		}

		var currentMessage queueObject
		err = topElement.ToObject(&currentMessage)
		if err != nil {
			zap.S().Fatalf("Error decoding first element", err)
			return
		}

		// Publish element and wait for confirmation

		if !strings.HasPrefix(currentMessage.Topic, subMQTTTopic) {
			zap.S().Errorf("Received unexpected message", currentMessage.Topic, subMQTTTopic)
			return
		}

		// see also documentation on the entire topic handling
		topic := pubMQTTTopic + strings.TrimPrefix(currentMessage.Topic, subMQTTTopic)

		token := client.Publish(topic, 1, false, currentMessage.Message)
		token.Wait() // wait indefinite amount of time (librarz will automatically resend)

		// if successfully received at broker delete from stack
		_, err = pq.Dequeue()
		if err != nil {
			zap.S().Fatalf("Error dequeuing element", err)
			return
		}
	}
}
