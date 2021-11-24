package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
	"io/ioutil"
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

func processMessage(customerID string, location string, assetID string, payloadType string, payload []byte) (err error) {
	zap.S().Debugf("New MQTT message. Customer: %s | Location: %s | AssetId: %s | payloadType: %s | Payload %s", customerID, location, assetID, payloadType, payload)
	err = AddAssetIfNotExisting(assetID, location, customerID, 0)
	if err != nil {
		switch GetPostgresErrorRecoveryOptions(err) { //{
		case Unrecoverable:
			ShutdownApplicationGraceful()
		case TryAgain:
			storedRawMQTTHandler.EnqueueMQTT(customerID, location, assetID, payload, payloadType, 0)
		case DiscardValue:
			// Discarding value, by doing nothing
		}

		return
	}

	if customerID != "raw" {
		if payloadType == "" {
			return
		}

		switch payloadType {
		case Prefix.State:
			stateHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)
		case Prefix.ProcessValue:
			valueDataHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)
		//TODO is still still needed ?
		case "processvalue":
			{
				zap.S().Warnf("Deprecated")
				valueDataHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)
			}
		case Prefix.ProcessValueString:
			valueStringHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)
		case Prefix.Count:
			countHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)
		case Prefix.ScrapCount:
			scrapCountHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)

		case Prefix.Recommendation:
			recommendationDataHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)

		case Prefix.AddShift:
			addShiftHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)

		case Prefix.AddMaintenanceActivity:
			maintenanceActivityHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)

		case Prefix.UniqueProduct:
			uniqueProductHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)

		case Prefix.UniqueProductScrap:
			scrapUniqueProductHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)

		case Prefix.AddProduct:
			addProductHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)

		case Prefix.AddOrder:
			addOrderHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)

		case Prefix.StartOrder:
			startOrderHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)

		case Prefix.EndOrder:
			endOrderHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)

		case Prefix.ProductTag:
			productTagHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)

		case Prefix.ProductTagString:
			productTagStringHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)

		case Prefix.AddParentToChild:
			addParentToChildHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)

		case Prefix.ModifyState:
			modifyStateHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)

		case Prefix.DeleteShiftById:
			deleteShiftByIdHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)

		case Prefix.DeleteShiftByAssetIdAndBeginTimestamp:
			deleteShiftByAssetIdAndBeginTimestampHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)

		case Prefix.ModifyProducesPieces:
			modifyProducedPieceHandler.EnqueueMQTT(customerID, location, assetID, payload, 0)

		default:
			zap.S().Warnf("Unknown Prefix: %s", payloadType)
		}

	}

	return
}

var rp = regexp.MustCompile(`ia/([\w]*)/([\w]*)/([\w]*)/([\w]*)`)

// getOnMessageReceived gets the function onMessageReceived, that is called everytime a message is received by a specific topic
func getOnMessageReceived() func(MQTT.Client, MQTT.Message) {
	return func(client MQTT.Client, message MQTT.Message) {
		//Check whether topic has the correct structure
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

		go func() {
			//This is "fine" here, cause it will re-queue
			_ = processMessage(customerID, location, assetID, payloadType, payload)
		}()
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
func SetupMQTT(certificateName string, mqttBrokerURL string, mqttTopic string, health healthcheck.Handler, podName string) {

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
	opts.SetOrderMatters(false)

	zap.S().Debugf("Broker configured", mqttBrokerURL, certificateName)

	// Start the connection
	mqttClient = MQTT.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	// Subscribe
	if token := mqttClient.Subscribe(mqttTopic, 2, getOnMessageReceived()); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	zap.S().Infof("MQTT subscribed", mqttTopic)

	// Implement a custom check with a 50 millisecond timeout.
	health.AddReadinessCheck("mqtt-check", checkConnected(mqttClient))
}
