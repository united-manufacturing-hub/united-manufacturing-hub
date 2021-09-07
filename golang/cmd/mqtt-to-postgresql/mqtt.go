package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

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
	zap.S().Debugf("newTLSConfig")

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

func processMessage(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue, basePrio uint8) {
	zap.S().Debugf("processMessage")
	zap.S().Infof("New MQTT message. Customer: %s | Location: %s | AssetId: %s | payloadType: %s | Payload %s", customerID, location, assetID, payloadType, payload)
	AddAssetIfNotExisting(assetID, location, customerID)

	var err error
	if customerID != "raw" {

		switch payloadType {
		case Prefix.State:
			err = ProcessStateData(customerID, location, assetID, payloadType, payload, pg)
		case Prefix.ProcessValue:
			err = ProcessProcessValueData(customerID, location, assetID, payloadType, payload, pg)
		//TODO is still still needed ?
		case "processvalue":
			{
				zap.S().Warnf("Depreciated")
				err = ProcessProcessValueData(customerID, location, assetID, payloadType, payload, pg)
			}
		case Prefix.ProcessValueString:
			err = ProcessProcessValueString(customerID, location, assetID, payloadType, payload, pg)
		case Prefix.Count:
			err = ProcessCountData(customerID, location, assetID, payloadType, payload, pg)
		case Prefix.ScrapCount:
			err = ProcessScrapCountData(customerID, location, assetID, payloadType, payload, pg)
		case Prefix.Recommendation:
			err = ProcessRecommendationData(customerID, location, assetID, payloadType, payload, pg)
		case Prefix.AddShift:
			err = ProcessAddShift(customerID, location, assetID, payloadType, payload, pg)
		case Prefix.AddMaintenanceActivity:
			err = ProcessAddMaintenanceActivity(customerID, location, assetID, payloadType, payload, pg)
		case Prefix.UniqueProduct:
			err = ProcessUniqueProduct(customerID, location, assetID, payloadType, payload, pg)
		case Prefix.UniqueProductScrap:
			err = ProcessScrapUniqueProduct(customerID, location, assetID, payloadType, payload, pg)
		case Prefix.AddProduct:
			err = ProcessAddProduct(customerID, location, assetID, payloadType, payload, pg)
		case Prefix.AddOrder:
			err = ProcessAddOrder(customerID, location, assetID, payloadType, payload, pg)
		case Prefix.StartOrder:
			err = ProcessStartOrder(customerID, location, assetID, payloadType, payload, pg)
		case Prefix.EndOrder:
			err = ProcessEndOrder(customerID, location, assetID, payloadType, payload, pg)
		case Prefix.ProductTag:
			err = ProcessProductTag(customerID, location, assetID, payloadType, payload, pg)
		case Prefix.ProductTagString:
			err = ProcessProductTagString(customerID, location, assetID, payloadType, payload, pg)
		case Prefix.AddParentToChild:
			err = ProcessAddParentToChild(customerID, location, assetID, payloadType, payload, pg)
		case Prefix.ModifyState:
			err = ProcessModifyState(customerID, location, assetID, payloadType, payload, pg)
		case Prefix.DeleteShiftById:
			err = ProcessDeleteShiftById(customerID, location, assetID, payloadType, payload, pg)
		case Prefix.DeleteShiftByAssetIdAndBeginTimestamp:
			err = ProcessDeleteShiftByAssetIdAndBeginTime(customerID, location, assetID, payloadType, payload, pg)
		case Prefix.ModifyProducesPieces:
			err = ProcessModifyProducesPiece(customerID, location, assetID, payloadType, payload, pg)
		default:
			zap.S().Warnf("Unknown Prefix: %s", payloadType)
		}
	}

	if err == ErrTryLater {
		EnqueueMQTT(customerID, location, assetID, payloadType, payload, pg, basePrio)
	}
}

func RetryMQTT(item QueueObject, prio uint8) {
	zap.S().Debugf("RetryMQTT", prio)
	if prio < 255 {
		prio += 1
	} else {
		prio = 254
		time.Sleep(10 * time.Second)
	}
	var pt MQTTQueueMessage
	err := json.Unmarshal(item.Payload, &pt)
	if err != nil {
		zap.S().Errorf("Failed to unmarshal item", item)
		return
	}
	processMessage(pt.CustomerID, pt.Location, pt.AssetID, pt.PayloadType, pt.Payload, globalDBPQ, prio)
	return
}

func EnqueueMQTT(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue, prio uint8) {
	zap.S().Debugf("EnqueueMQTT")
	newObject := MQTTQueueMessage{
		CustomerID:  customerID,
		Location:    location,
		AssetID:     assetID,
		PayloadType: payloadType,
		Payload:     payload,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}
	err = addRawItemWithPriorityToQueue(pg, Prefix.RawMQTTRequeue, marshal, prio)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return
	}
	return
}

type MQTTQueueMessage struct {
	CustomerID  string
	Location    string
	AssetID     string
	PayloadType string
	Payload     []byte
}

// getOnMessageRecieved gets the function onMessageReceived, that is called everytime a message is recieved by a specific topic
func getOnMessageRecieved(pg *goque.PriorityQueue) func(MQTT.Client, MQTT.Message) {
	zap.S().Debugf("getOnMessageRecieved")

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

		go processMessage(customerID, location, assetID, payloadType, payload, pg, 0)
	}
}

// OnConnect subscribes once the connection is established. Required to re-subscribe when cleansession is True
func OnConnect(c MQTT.Client) {
	zap.S().Debugf("OnConnect")
	optionsReader := c.OptionsReader()
	zap.S().Infof("Connected to MQTT broker", optionsReader.ClientID())
	mqttConnected.Inc()
}

// OnConnectionLost outputs warn message
func OnConnectionLost(c MQTT.Client, err error) {
	zap.S().Debugf("OnConnectionLost")
	optionsReader := c.OptionsReader()
	zap.S().Warnf("Connection lost", err, optionsReader.ClientID())
	mqttConnected.Dec()
}

func checkConnected(c MQTT.Client) healthcheck.Check {
	zap.S().Debugf("checkConnected")
	return func() error {
		if c.IsConnected() {
			return nil
		}
		return fmt.Errorf("not connected")
	}
}

// ShutdownMQTT unsubscribes and closes the MQTT connection
func ShutdownMQTT() {
	zap.S().Debugf("ShutdownMQTT")
	mqttClient.Disconnect(1000)
}

// SetupMQTT setups MQTT and connect to the broker
func SetupMQTT(certificateName string, mqttBrokerURL string, mqttTopic string, health healthcheck.Handler, podName string, pg *goque.PriorityQueue) {
	zap.S().Debugf("SetupMQTT")

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
