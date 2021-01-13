package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"os"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"

	"regexp" // pattern matching
)

var mqttClient MQTT.Client

// Prometheus metrics
var (
	mqttTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "mqtttopostgres_total",
			Help: "The total number of incoming MQTT messages",
		},
	)
	mqttConnected = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "mqtttopostgres_up",
			Help: "Connection with MQTT broker",
		},
	)
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

func processMessage(customerID string, location string, assetID string, payloadType string, payload []byte) {
	AddAssetIfNotExisting(assetID, location, customerID)

	if customerID != "raw" {

		// TODO Add more logic to paste it into database
		switch payloadType {
		case "state":
			go ProcessStateData(customerID, location, assetID, payloadType, payload)
			break
		case "processValue":
			go ProcessProcessValueData(customerID, location, assetID, payloadType, payload)
			break
		case "processvalue":
			go ProcessProcessValueData(customerID, location, assetID, payloadType, payload)
			break
		case "count":
			go ProcessCountData(customerID, location, assetID, payloadType, payload)
			break
		case "recommendation":
			go ProcessRecommendationData(customerID, location, assetID, payloadType, payload)
			break
		case "addShift":
			go ProcessAddShift(customerID, location, assetID, payloadType, payload)
			break
		case "addMaintenanceActivity":
			go ProcessAddMaintenanceActivity(customerID, location, assetID, payloadType, payload)
			break
		case "uniqueProduct":
			go ProcessUniqueProduct(customerID, location, assetID, payloadType, payload)
			break
		case "addProduct":
			go ProcessAddProduct(customerID, location, assetID, payloadType, payload)
			break
		case "addOrder":
			go ProcessAddOrder(customerID, location, assetID, payloadType, payload)
			break
		case "startOrder":
			go ProcessStartOrder(customerID, location, assetID, payloadType, payload)
			break
		case "endOrder":
			go ProcessEndOrder(customerID, location, assetID, payloadType, payload)
			break
		}
	}
}

func onMessageReceived(client MQTT.Client, message MQTT.Message) {

	//Check whether topic has the correct structure
	rp := regexp.MustCompile("ia/([\\w]*)/([\\w]*)/([\\w]*)/([\\w]*)")

	res := rp.FindStringSubmatch(message.Topic())
	if res == nil {
		//zap.S().Infof("onMessageReceived","unknown topic:  %s\tMessage: %s", message.Topic(), message.Payload())
		return
	}

	customerID := res[1]
	location := res[2]
	assetID := res[3]
	payloadType := res[4]
	payload := message.Payload()

	zap.S().Debugf("onMessageReceived", customerID, location, assetID, payloadType)
	mqttTotal.Inc()

	go processMessage(customerID, location, assetID, payloadType, payload)
}

// OnConnect subscribes once the connection is established. Required to re-subscribe when cleansession is True
func OnConnect(c MQTT.Client) {
	zap.S().Infof("Connected to MQTT broker")

	certificateName := os.Getenv("CERTIFICATE_NAME")

	mqttTopic := os.Getenv("CUSTOM_MQTT_TOPIC")

	// if custom MQTT topic not sert
	if mqttTopic == "" {
		if certificateName == "NO_CERT" {
			mqttTopic = "$share/MQTT_TO_POSTGRESQL/ia/#"
			zap.S().Infof("Subscribing in Kubernetes mode", mqttTopic)
		} else {
			mqttTopic = "ia/#"
			zap.S().Infof("Running in normal mode", mqttTopic)
		}
	}

	if token := c.Subscribe(mqttTopic, 2, onMessageReceived); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	mqttConnected.Inc()
	zap.S().Debugf("mqttConnected.Inc()")
}

// OnConnectionLost outputs warn message
func OnConnectionLost(c MQTT.Client, err error) {
	zap.S().Warnf("Connection lost", err)
	mqttConnected.Dec()
	zap.S().Debugf("mqttConnected.Dec()")
}

// OnReconnecting outputs info message
func OnReconnecting(c MQTT.Client, opts *MQTT.ClientOptions) {
	zap.S().Infof("Reconnecting...")
}

func checkConnected(c MQTT.Client) healthcheck.Check {
	return func() error {
		if c.IsConnected() {
			return nil
		}
		return fmt.Errorf("not connected")
	}
}

// ShutdownMQTT unsubscribes and closes the MQTT connection
func ShutdownMQTT() {
	mqttClient.Disconnect(1000)
}

// SetupMQTT setups MQTT and connect to the broker
func SetupMQTT(certificateName string, mqttBrokerURL string, health healthcheck.Handler, podName string) {

	opts := MQTT.NewClientOptions()
	opts.AddBroker(mqttBrokerURL)
	if certificateName == "NO_CERT" {
		opts.SetClientID(podName)
		opts.SetUsername("MQTT_TO_POSTGRESQL")
		zap.S().Infof("Running in Kubernetes mode", podName)
	} else {
		tlsconfig := newTLSConfig(certificateName)
		zap.S().Infof("Running in normal mode", certificateName)
		opts.SetClientID(certificateName).SetTLSConfig(tlsconfig)
	}
	opts.SetAutoReconnect(true)
	opts.SetOnConnectHandler(OnConnect)
	opts.SetConnectionLostHandler(OnConnectionLost)

	zap.S().Debugf("Broker configured", mqttBrokerURL, certificateName)

	// Start the connection
	mqttClient = MQTT.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	// Implement a custom check with a 50 millisecond timeout.
	health.AddReadinessCheck("mqtt-check", checkConnected(mqttClient))
}
