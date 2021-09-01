package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/beeker1121/goque"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
	"io/ioutil"
	"time"
)

func newTLSConfig(certificateName string) *tls.Config {

	// Import trusted certificates from CAfile.pem.
	// Alternatively, manually add CA certificates to
	// default openssl CA bundle.
	certpool := x509.NewCertPool()
	pemCerts, err := ioutil.ReadFile("/SSL_certs/intermediate_CA.pem")
	if err == nil {
		certpool.AppendCertsFromPEM(pemCerts)
	}

	// Import client certificate/key pair
	cert, err := tls.LoadX509KeyPair("/SSL_certs/"+certificateName+".pem", "/SSL_certs/"+certificateName+"-privkey.pem")
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
func SetupMQTT(certificateName string, mqttBrokerURL string, mqttTopic string, podName string) MQTT.Client {

	opts := MQTT.NewClientOptions()
	opts.AddBroker(mqttBrokerURL)
	if certificateName == "NO_CERT" {
		opts.SetClientID(podName)
		opts.SetUsername("MQTT_TO_POSTGRESQL")

		if mqttTopic == "" {
			mqttTopic = "$share/MQTT_TO_POSTGRESQL/ia/#"
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

	zap.S().Debugf("Broker configured", mqttBrokerURL, certificateName)

	// Start the connection
	mqttClient = MQTT.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	zap.S().Infof("MQTT subscribed", mqttTopic)

	return mqttClient
}

func MqttQueueHandler() {
	for !shutdownEnabled {
		if mqttClient.IsConnected() {
			zap.S().Errorf("MQTT client disconnected, while program is running, waiting for it to re-connect")
			time.Sleep(1 * time.Second)
			continue
		}

		mqttData, err := dequeueMQTT()

		if err != nil {
			if err == goque.ErrEmpty {
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

		token := mqttClient.Publish(fmt.Sprintf("ia/%s/%s/%s/%s", mqttData.Customer, mqttData.Location, mqttData.Asset, mqttData.Value), 2, false, mqttData.JSONData)

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
