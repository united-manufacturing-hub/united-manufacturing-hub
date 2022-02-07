package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"github.com/heptiolabs/healthcheck"
	"io/ioutil"
	"time"

	"github.com/beeker1121/goque"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
)

// newTLSConfig returns the TLS config for a given clientID and mode
func newTLSConfig(clientID string) *tls.Config {

	// Import trusted certificates from CAfile.pem.
	// Alternatively, manually add CA certificates to
	// default openssl CA bundle.
	certpool := x509.NewCertPool()
	pemCerts, err := ioutil.ReadFile("/SSL_certs/ca.crt")
	if err == nil {
		certpool.AppendCertsFromPEM(pemCerts)
	}

	// Import client certificate/key pair
	cert, err := tls.LoadX509KeyPair("/SSL_certs/tls.crt", "/SSL_certs/tls.key")
	if err != nil {
		panic(err)
	}

	// Just to print out the client certificate..
	cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		panic(err)
	}

	// Create tls.Config with desired tls properties
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
		InsecureSkipVerify: true,
		// Certificates = list of certs client sends to server.
		Certificates: []tls.Certificate{cert},
	}
}

// getOnMessageReceived gets the function onMessageReceived, that is called everytime a message is recieved by a specific topic
func getOnMessageReceived(pg *goque.Queue) func(MQTT.Client, MQTT.Message) {

	return func(client MQTT.Client, message MQTT.Message) {
		topic := message.Topic()
		payload := message.Payload()
		if json.Valid(payload) {
			zap.S().Infof("onMessageReceived", topic, payload)
			go storeNewMessageIntoQueue(topic, payload, pg)
		} else {
			zap.S().Warnf("kafkaToQueue [INVALID] message not forwarded because the content is not a valid JSON", topic, payload)
		}
	}
}

// onConnect subscribes once the connection is established. Required to re-subscribe when cleansession is True
func onConnect(c MQTT.Client) {
	optionsReader := c.OptionsReader()
	zap.S().Infof("Connected to MQTT broker", optionsReader.ClientID())
}

// onConnectionLost outputs warn message
func onConnectionLost(c MQTT.Client, err error) {
	optionsReader := c.OptionsReader()
	zap.S().Warnf("Connection lost, restarting", err, optionsReader.ClientID())
	ShutdownApplicationGraceful()
}

// SetupMQTT setups MQTT and connect to the broker
func SetupMQTT(certificateName string, mqttBrokerURL string, mqttTopic string, health healthcheck.Handler, podName string, pg *goque.Queue) {

	opts := MQTT.NewClientOptions()
	opts.AddBroker(mqttBrokerURL)
	if certificateName == "NO_CERT" {
		opts.SetClientID(podName)
		opts.SetUsername("MQTT_KAFKA_BRIDGE")

		if mqttTopic == "" {
			mqttTopic = "$share/MQTT_KAFKA_BRIDGE/ia/#"
		}

		zap.S().Infof("Running in Kubernetes mode", podName, mqttTopic)

	} else {
		tlsconfig := newTLSConfig(certificateName)
		opts.SetClientID(certificateName).SetTLSConfig(tlsconfig)

		if mqttTopic == "" {
			mqttTopic = "ia/#"
		}

		zap.S().Infof("Running in normal mode", mqttTopic, certificateName)
	}
	opts.SetAutoReconnect(true)
	opts.SetOnConnectHandler(onConnect)
	opts.SetConnectionLostHandler(onConnectionLost)
	opts.SetOrderMatters(false)

	zap.S().Debugf("Broker configured", mqttBrokerURL, certificateName)

	// Start the connection
	mqttClient = MQTT.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	// Subscribe
	if token := mqttClient.Subscribe(mqttTopic, 2, getOnMessageReceived(pg)); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	zap.S().Infof("MQTT subscribed", mqttTopic)

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

func processOutgoingMessages() {
	var err error

	for !ShuttingDown && mqttClient.IsConnected() {
		if mqttOutGoingQueue.Length() == 0 {
			//Skip if empty
			time.Sleep(10 * time.Millisecond)
			continue
		}

		var mqttData queueObject
		mqttData, err = retrieveMessageFromQueue(mqttOutGoingQueue)
		if err != nil {
			return
		}
		if err != nil {
			zap.S().Errorf("Failed to dequeue message: %s", err)
			time.Sleep(1 * time.Second)
			continue
		}

		token := mqttClient.Publish(mqttData.Topic, 2, false, mqttData.Message)

		var sendMQTT = false
		for i := 0; i < 10; i++ {
			sendMQTT = token.WaitTimeout(10 * time.Second)
			if sendMQTT {
				break
			}
		}

		// Failed to send MQTT message (or 10x timeout)
		err = token.Error()
		if err != nil || !sendMQTT {
			zap.S().Warnf("Failed to send MQTT message", err, sendMQTT)
			// Try to re-enqueue the message
			storeMessageIntoQueue(mqttData.Topic, mqttData.Message, mqttOutGoingQueue)
			// After an error, just wait a bit
			time.Sleep(1 * time.Millisecond)
			continue
		}
	}
}
