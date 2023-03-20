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
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"
	"os"
)

// newTLSConfig returns the TLS config for a given clientID and mode
func newTLSConfig() *tls.Config {

	// Import trusted certificates from CAfile.pem.
	// Alternatively, manually add CA certificates to
	// default openssl CA bundle.
	certpool := x509.NewCertPool()
	pemCerts, err := os.ReadFile("/SSL_certs/kafka/ca.crt")
	if err == nil {
		certpool.AppendCertsFromPEM(pemCerts)
	}

	// Import client certificate/key pair
	cert, err := tls.LoadX509KeyPair("/SSL_certs/kafka/tls.crt", "/SSL_certs/kafka/tls.key")
	if err != nil {
		zap.S().Fatalf("Failed to load client certificate: %s", err)
	}

	// Just to print out the client certificate..
	cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		zap.S().Fatalf("Failed to parse certificate: %s", err)
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
		InsecureSkipVerify: true,
		// Certificates = list of certs client sends to server.
		Certificates: []tls.Certificate{cert},
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
	zap.S().Fatalf("Connection to MQTT broker lost, restarting ! %#v", optionsReader)
}

// SetupMQTT setups MQTT and connect to the broker
func SetupMQTT(certificateName string, mqttBrokerURL string, podName string, password string) {
	if !useMQTT {
		return
	}

	opts := MQTT.NewClientOptions()
	opts.AddBroker(mqttBrokerURL)
	opts.SetUsername("SENSORCONNECT")
	if password != "" {
		opts.SetPassword(password)
	}

	if certificateName == "NO_CERT" {
		opts.SetClientID(podName)

		zap.S().Infof("Running in Kubernetes mode", podName)

	} else {
		tlsconfig := newTLSConfig()
		opts.SetClientID(certificateName).SetTLSConfig(tlsconfig)

		zap.S().Infof("Running in normal mode", certificateName)
	}
	opts.SetAutoReconnect(true)
	opts.SetOnConnectHandler(onConnect)
	opts.SetConnectionLostHandler(onConnectionLost)
	opts.SetOrderMatters(false)

	zap.S().Debugf("Broker configured", mqttBrokerURL, certificateName)

	// Start the connection
	mqttClient = MQTT.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		zap.S().Fatalf("Failed to connect: %s", token.Error())
	}
}

func SendMQTTMessage(topic string, message []byte) {
	if !useMQTT {
		return
	}

	messageHash := xxh3.Hash(message)
	cacheKey := fmt.Sprintf("SendMQTTMessage%s%d", topic, messageHash)

	_, found := internal.GetMemcached(cacheKey)
	if found {
		zap.S().Debugf("Duplicate message for topic %s, you might want to increase LOWER_POLLING_TIME !", topic)
		return
	}

	mqttClient.Publish(topic, 2, false, message)
	internal.SetMemcached(cacheKey, nil)
}
