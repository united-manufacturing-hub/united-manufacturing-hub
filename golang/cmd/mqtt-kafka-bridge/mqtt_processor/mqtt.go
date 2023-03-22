package mqtt_processor

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/heptiolabs/healthcheck"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/mqtt-kafka-bridge/message"
	"go.uber.org/zap"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
)

var mqttClient MQTT.Client
var sChan chan bool

var sentMessages atomic.Uint64
var receivedMessages atomic.Uint64

func Init(mqttToKafkaChan chan kafka.Message, shutdownChan chan bool) {
	if mqttClient != nil {
		return
	}
	sChan = shutdownChan
	certificateName, MQTTCertificateNameEnvSet := os.LookupEnv("MQTT_CERTIFICATE_NAME")
	if !MQTTCertificateNameEnvSet {
		zap.S().Fatal("Mqtt certificate name (MQTT_CERTIFICATE_NAME) must be set")
	}
	mqttBrokerURL, MQTTBrokerURLEnvSet := os.LookupEnv("MQTT_BROKER_URL")
	if !MQTTBrokerURLEnvSet {
		zap.S().Fatal("Mqtt broker url (MQTT_BROKER_URL) must be set")
	}
	mqttTopic, MQTTTopicEnvSet := os.LookupEnv("MQTT_TOPIC")
	if !MQTTTopicEnvSet {
		zap.S().Fatal("Mqtt topic (MQTT_TOPIC) must be set")
	}
	podName, podNameEnvSet := os.LookupEnv("MY_POD_NAME")
	if !podNameEnvSet {
		zap.S().Fatal("Pod name (MY_POD_NAME) must be set")
	}
	password, mqttPasswordEnvSet := os.LookupEnv("MQTT_PASSWORD")
	if !mqttPasswordEnvSet {
		zap.S().Fatal("Mqtt password (MQTT_PASSWORD) must be set")
	}

	opts := MQTT.NewClientOptions()
	opts.AddBroker(mqttBrokerURL)
	opts.SetUsername("MQTT_KAFKA_BRIDGE")
	if password != "" {
		opts.SetPassword(password)
	}

	// Check if topic is using $share
	if strings.Index(mqttTopic, "$share") != 0 {
		// Add $share/MQTT_KAFKA_BRIDGE/ to topic
		mqttTopic = "$share/MQTT_KAFKA_BRIDGE/" + mqttTopic
	}

	if certificateName == "NO_CERT" {
		opts.SetClientID(podName)

		zap.S().Infof("Running in Kubernetes mode (%s) (%s)", podName, mqttTopic)

	} else {
		tlsconfig := newTLSConfig()
		opts.SetClientID(podName).SetTLSConfig(tlsconfig)

		zap.S().Infof("Running in normal mode (%s) (%s) (%s)", mqttTopic, certificateName, podName)

	}
	opts.SetAutoReconnect(true)
	opts.SetOnConnectHandler(onConnect)
	opts.SetConnectionLostHandler(onConnectionLost)
	opts.SetOrderMatters(false)

	zap.S().Debugf("Broker configured (%s) (%s) (%s)", mqttBrokerURL, certificateName, podName)

	// Start the connection
	mqttClient = MQTT.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		zap.S().Fatalf("Failed to connect: %s", token.Error())
	}

	// Subscribe
	if token := mqttClient.Subscribe(mqttTopic, 1, getOnMessageReceived(mqttToKafkaChan)); token.Wait() && token.Error() != nil {
		zap.S().Fatalf("Failed to subscribe: %s", token.Error())
	}
	zap.S().Infof("MQTT subscribed (%s)", mqttTopic)

	health := healthcheck.NewHandler()
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(1000000))
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe("0.0.0.0:8086", health)
		if err != nil {
			zap.S().Fatalf("Error starting healthcheck %v", err)
		}
	}()

	// Implement a custom check with a 50 millisecond timeout.
	health.AddReadinessCheck("mqtt-check", checkConnected(mqttClient))
}

func checkConnected(c MQTT.Client) healthcheck.Check {
	return func() error {
		if c.IsConnected() {
			return nil
		}
		return fmt.Errorf("not connected")
	}
}

func getOnMessageReceived(pg chan kafka.Message) MQTT.MessageHandler {
	return func(client MQTT.Client, msg MQTT.Message) {
		topic := msg.Topic()
		payload := msg.Payload()

		if message.IsValidMQTTMessage(topic, payload) {
			topic = strings.ReplaceAll(topic, "$share/MQTT_KAFKA_BRIDGE/", "")
			pg <- kafka.Message{
				Topic: topic,
				Value: payload,
			}
			receivedMessages.Add(1)
		}
	}
}

func onConnectionLost(_ MQTT.Client, _ error) {
	sChan <- true
}

func onConnect(client MQTT.Client) {
	optionsReader := client.OptionsReader()
	zap.S().Infof("Connected to MQTT broker (%s)", optionsReader.ClientID())
}

// newTLSConfig returns the TLS config for a given clientID and mode
func newTLSConfig() *tls.Config {

	// Import trusted certificates from CAfile.pem.
	// Alternatively, manually add CA certificates to
	// default openssl CA bundle.
	certpool := x509.NewCertPool()
	pemCerts, err := os.ReadFile("/SSL_certs/mqtt/ca.crt")
	if err == nil {
		ok := certpool.AppendCertsFromPEM(pemCerts)
		if !ok {
			zap.S().Errorf("Failed to parse root certificate")
		}
	} else {
		zap.S().Errorf("Error reading CA certificate: %s", err)
	}

	zap.S().Debugf("CA cert: %s", pemCerts)

	// Import client certificate/key pair
	cert, err := tls.LoadX509KeyPair("/SSL_certs/mqtt/tls.crt", "/SSL_certs/mqtt/tls.key")
	if err != nil {
		// Read /SSL_certs/mqtt/tls.crt
		var file []byte
		file, err = os.ReadFile("/SSL_certs/mqtt/tls.crt")
		if err != nil {
			zap.S().Errorf("Error reading client certificate: %s", err)
		}
		zap.S().Fatalf("Error reading client certificate: %s (File: %s)", err, file)
	}

	zap.S().Debugf("Client cert: %v", cert)

	// Just to print out the client certificate..
	cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		zap.S().Fatalf("Error parsing client certificate: %s", err)
	}

	skipVerify := os.Getenv("INSECURE_SKIP_VERIFY") == "true"

	// Create tls.Config with desired tls properties
	/* #nosec G402 -- Remote verification is not yet implemented*/
	return &tls.Config{
		// RootCAs = certs used to verify server cert.
		RootCAs: certpool,
		// ClientAuth = whether to request cert from server.
		// Since the server is set up for SSL, this happens
		// anyways.
		// ClientAuth: tls.NoClientCert,
		// ClientCAs = certs used to validate client cert.
		// ClientCAs: nil,
		// InsecureSkipVerify = verify that cert contents
		// match server. IP matches what is in cert etc.
		/* #nosec G402 -- Remote verification is not yet implemented*/
		InsecureSkipVerify: skipVerify,
		// Certificates = list of certs client sends to server.
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}
}

func Shutdown() {
	zap.S().Infof("Shutdown requested")
	mqttClient.Disconnect(250)
	zap.S().Infof("MQTT disconnected")
}

func Start(kafkaToMqttChan chan kafka.Message) {
	go start(kafkaToMqttChan)
}

func start(kafkaToMqttChan chan kafka.Message) {
	for {
		msg := <-kafkaToMqttChan
		// Change MQTT to Kafka topic format
		msg.Topic = strings.ReplaceAll(msg.Topic, ".", "/")
		mqttClient.Publish(msg.Topic, 1, false, msg.Value)
		sentMessages.Add(1)
	}
}

func GetStats() (s uint64, r uint64) {
	return sentMessages.Load(), receivedMessages.Load()
}
