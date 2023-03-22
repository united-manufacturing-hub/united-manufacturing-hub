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

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/beeker1121/goque"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/heptiolabs/healthcheck"
	jsoniter "github.com/json-iterator/go"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"os"
	"strings"
	"time"
)

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

// getOnMessageReceived gets the function onMessageReceived, that is called everytime a message is received by a specific topic
func getOnMessageReceived(pg *goque.Queue) func(MQTT.Client, MQTT.Message) {

	return func(client MQTT.Client, message MQTT.Message) {
		go func() {
			topic := message.Topic()
			payload := message.Payload()

			if jsoniter.Valid(payload) || strings.HasPrefix(topic, "ia/raw") {
				storeNewMessageIntoQueue(topic, payload, pg)
			} else {
				zap.S().Warnf(
					"kafkaToQueue [INVALID] message not forwarded because the content is not a valid JSON (%s) [%s]",
					topic,
					payload)
			}
		}()
	}
}

// onConnect subscribes once the connection is established. Required to re-subscribe when cleansession is True
func onConnect(c MQTT.Client) {
	optionsReader := c.OptionsReader()
	zap.S().Infof("Connected to MQTT broker (%s)", optionsReader.ClientID())
}

// onConnectionLost outputs warn message
func onConnectionLost(c MQTT.Client, err error) {
	optionsReader := c.OptionsReader()
	zap.S().Warnf("Connection lost, restarting (%v) (%s)", err, optionsReader.ClientID())
	ShutdownApplicationGraceful()
}

// SetupMQTT setups MQTT and connect to the broker
func SetupMQTT(
	certificateName string,
	mqttBrokerURL string,
	mqttTopic string,
	health healthcheck.Handler,
	podName string,
	pg *goque.Queue, password string) {

	opts := MQTT.NewClientOptions()
	opts.AddBroker(mqttBrokerURL)
	opts.SetUsername("MQTT_KAFKA_BRIDGE")
	if password != "" {
		opts.SetPassword(password)
	}
	if certificateName == "NO_CERT" {
		opts.SetClientID(podName)

		if mqttTopic == "" {
			mqttTopic = "$share/MQTT_KAFKA_BRIDGE/ia/#"
		}

		zap.S().Infof("Running in Kubernetes mode (%s) (%s)", podName, mqttTopic)

	} else {
		tlsconfig := newTLSConfig()
		opts.SetClientID(podName).SetTLSConfig(tlsconfig)

		if mqttTopic == "" {
			mqttTopic = "ia/#"
		}

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
	if token := mqttClient.Subscribe(mqttTopic, 1, getOnMessageReceived(pg)); token.Wait() && token.Error() != nil {
		zap.S().Fatalf("Failed to subscribe: %s", token.Error())
	}
	zap.S().Infof("MQTT subscribed (%s)", mqttTopic)

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

var SentMQTTMessages = int64(0)
var MQTTSenderThreads int

func processOutgoingMessages() {
	for i := 0; i < MQTTSenderThreads; i++ {
		go SendMQTTMessages()
	}
}

func SendMQTTMessages() {
	var err error

	var loopsSinceLastMessage = int64(0)
	for !ShuttingDown {
		internal.SleepBackedOff(loopsSinceLastMessage, 1*time.Millisecond, 1*time.Second)
		loopsSinceLastMessage += 1
		if !mqttClient.IsConnected() {
			zap.S().Warnf("MQTT not connected, restarting service")
			ShutdownApplicationGraceful()
			return
		}

		var mqttData *queueObject
		mqttData, err, _ = retrieveMessageFromQueue(mqttOutGoingQueue)
		if err != nil {
			zap.S().Errorf("Failed to dequeue message: %s", err)
			continue
		}

		if mqttData == nil {
			continue
		}

		token := mqttClient.Publish(mqttData.Topic, 1, false, mqttData.Message)

		for i := 0; i < 10; i++ {
			if token.WaitTimeout(10 * time.Second) {
				loopsSinceLastMessage = 0
				break
			}
		}

		// Failed to send MQTT message
		err = token.Error()
		if err != nil {
			zap.S().Warnf("Failed to send MQTT message (%v)", err)
			// Try to re-enqueue the message
			storeMessageIntoQueue(mqttData.Topic, mqttData.Message, mqttOutGoingQueue)
			continue
		}
		SentMQTTMessages++
	}
	zap.S().Infof("MQTT sender thread stopped")
}
