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
	"errors"
	"fmt"
	"github.com/beeker1121/goque"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
	"os"
	"time"
)

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
		InsecureSkipVerify: true,
		// Certificates = list of certs client sends to server.
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
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
	zap.S().Warnf("Connection lost", err, optionsReader.ClientID())
}

// ShutdownMQTT unsubscribes and closes the MQTT connection
func ShutdownMQTT() {
	mqttClient.Disconnect(1000)
}

// SetupMQTT setups MQTT and connect to the broker
func SetupMQTT(certificateName string, mqttBrokerURL string, podName string, password string) MQTT.Client {

	opts := MQTT.NewClientOptions()
	opts.AddBroker(mqttBrokerURL)
	opts.SetUsername("FACTORYINPUT")
	if password != "" {
		opts.SetPassword(password)
	}

	if certificateName == "NO_CERT" {
		opts.SetClientID(podName)

		zap.S().Infof("Running in Kubernetes mode", podName)

	} else {
		tlsconfig := newTLSConfig()
		opts.SetClientID(podName).SetTLSConfig(tlsconfig)

		zap.S().Infof("Running in normal mode", certificateName)
	}
	opts.SetAutoReconnect(true)
	opts.SetOnConnectHandler(onConnect)
	opts.SetConnectionLostHandler(onConnectionLost)

	zap.S().Debugf("Broker configured", mqttBrokerURL, certificateName)

	// Start the connection
	mqttClient = MQTT.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		zap.S().Fatalf("Failed to connect: %s", token.Error())
	}

	return mqttClient
}

func MqttQueueHandler() {
	if shutdownEnabled {
		zap.S().Warnf("SHUTDOWN ENABLED !")
	}
	for !shutdownEnabled {
		mqttData, err := dequeueMQTT()

		if err != nil {
			if errors.Is(err, goque.ErrEmpty) {
				// Queue is empty, just sleep a bit to prevent CPU load
				time.Sleep(1 * time.Second)
				continue
			} else {
				// This can only be thrown, by Dequeue (in this case the Queue is corrupt)
				// or by json.Unmarshal, in which case the MQTT message is corrupt
				// In neither case it should be re-inserted
				zap.S().Errorf("Error dequeueing MQTT message", err)
				ShutdownApplicationGraceful()
				return
			}
		}

		token := mqttClient.Publish(
			fmt.Sprintf(
				"ia/%s/%s/%s/%s",
				mqttData.Customer,
				mqttData.Location,
				mqttData.Asset,
				mqttData.Value), 2, false, mqttData.JSONData)

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
			err = enqueueMQTT(mqttData)
			if err != nil {
				// This is bad !
				zap.S().Errorf("Error re-enqueue MQTT message", err, mqttData)
				ShutdownApplicationGraceful()
				return
			}
			// After an error, just wait a bit
			time.Sleep(1 * time.Millisecond)
			continue
		}
	}
}
