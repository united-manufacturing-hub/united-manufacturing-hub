package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"

	"github.com/beeker1121/goque"
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

func processMessage(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PrefixQueue) {
	AddAssetIfNotExisting(assetID, location, customerID)

	if customerID != "raw" {

		switch payloadType {
		case "state":
			ProcessStateData(customerID, location, assetID, payloadType, payload, pg)
		case "processValue":
			ProcessProcessValueData(customerID, location, assetID, payloadType, payload, pg)
		case "processvalue":
			ProcessProcessValueData(customerID, location, assetID, payloadType, payload, pg)
		case "count":
			ProcessCountData(customerID, location, assetID, payloadType, payload, pg)
		case "scrapCount":
			ProcessScrapCountData(customerID, location, assetID, payloadType, payload, pg)
		case "recommendation":
			ProcessRecommendationData(customerID, location, assetID, payloadType, payload, pg)
		case "addShift":
			ProcessAddShift(customerID, location, assetID, payloadType, payload, pg)
		case "addMaintenanceActivity":
			ProcessAddMaintenanceActivity(customerID, location, assetID, payloadType, payload, pg)
		case "uniqueProduct":
			ProcessUniqueProduct(customerID, location, assetID, payloadType, payload, pg)
		case "scrapUniqueProduct":
			ProcessScrapUniqueProduct(customerID, location, assetID, payloadType, payload, pg)
		case "addProduct":
			ProcessAddProduct(customerID, location, assetID, payloadType, payload, pg)
		case "addOrder":
			ProcessAddOrder(customerID, location, assetID, payloadType, payload, pg)
		case "startOrder":
			ProcessStartOrder(customerID, location, assetID, payloadType, payload, pg)
		case "endOrder":
			ProcessEndOrder(customerID, location, assetID, payloadType, payload, pg)
		case "productTag":
			ProcessProductTag(customerID, location, assetID, payloadType, payload, pg)
		case "productTagString":
			ProcessProductTagString(customerID, location, assetID, payloadType, payload, pg)
		case "addParentToChild":
			ProcessAddParentToChild(customerID, location, assetID, payloadType, payload, pg)
		case "modifyState":
			ProcessModifyState(customerID, location, assetID, payloadType, payload, pg)
		case "deleteShiftById":
			ProcessDeleteShiftById(customerID, location, assetID, payloadType, payload, pg)
		case "deleteShiftByAssetIdAndBeginTimestamp":
			ProcessDeleteShiftByAssetIdAndBeginTime(customerID, location, assetID, payloadType, payload, pg)
		case "modifyProducedPieces":
			ProcessModifyProducesPiece(customerID, location, assetID, payloadType, payload, pg)
		}
	}
}

// getOnMessageRecieved gets the function onMessageReceived, that is called everytime a message is recieved by a specific topic
func getOnMessageRecieved(pg *goque.PrefixQueue) func(MQTT.Client, MQTT.Message) {

	return func(client MQTT.Client, message MQTT.Message) {

		//Check whether topic has the correct structure
		rp := regexp.MustCompile(`ia/([\w]*)/([\w]*)/([\w]*)/([\w]*)`)

		res := rp.FindStringSubmatch(message.Topic())
		if res == nil {
			return
		}

		customerID := res[1]
		location := res[2]
		assetID := res[3]
		payloadType := res[4]
		payload := message.Payload()

		mqttTotal.Inc()

		go processMessage(customerID, location, assetID, payloadType, payload, pg)
	}
}

// OnConnect subscribes once the connection is established. Required to re-subscribe when cleansession is True
func OnConnect(c MQTT.Client) {
	optionsReader := c.OptionsReader()
	zap.S().Infof("Connected to MQTT broker", optionsReader.ClientID())
	mqttConnected.Inc()
}

// OnConnectionLost outputs warn message
func OnConnectionLost(c MQTT.Client, err error) {
	optionsReader := c.OptionsReader()
	zap.S().Warnf("Connection lost", err, optionsReader.ClientID())
	mqttConnected.Dec()
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
func SetupMQTT(certificateName string, mqttBrokerURL string, mqttTopic string, health healthcheck.Handler, podName string, pg *goque.PrefixQueue) {

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
	opts.SetOnConnectHandler(OnConnect)
	opts.SetConnectionLostHandler(OnConnectionLost)

	zap.S().Debugf("Broker configured", mqttBrokerURL, certificateName)

	// Start the connection
	mqttClient = MQTT.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	// Subscribe
	if token := mqttClient.Subscribe(mqttTopic, 2, getOnMessageRecieved(pg)); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	zap.S().Infof("MQTT subscribed", mqttTopic)

	// Implement a custom check with a 50 millisecond timeout.
	health.AddReadinessCheck("mqtt-check", checkConnected(mqttClient))
}
