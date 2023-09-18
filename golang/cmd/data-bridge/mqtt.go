package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync/atomic"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/goccy/go-json"
	lru "github.com/hashicorp/golang-lru"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"go.uber.org/zap"
	"golang.org/x/crypto/sha3"
)

type mqttClient struct {
	client MQTT.Client
	topic  string
	sent   atomic.Uint64
	recv   atomic.Uint64
}

var arc *lru.ARCCache

func newMqttClient(broker, topic, serialNumber string) (mc *mqttClient, err error) {
	mc = &mqttClient{}

	enableSsl, err := env.GetAsBool("MQTT_ENABLE_TLS", false, false)
	if err != nil {
		zap.S().Error(err)
	}
	psw, err := env.GetAsString("MQTT_PASSWORD", false, "")
	if err != nil {
		zap.S().Error(err)
	}
	podName, err := env.GetAsString("POD_NAME", true, "")
	if err != nil {
		return nil, err
	}

	opts := MQTT.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetUsername("DATA_BRIDGE")
	if psw != "" {
		opts.SetPassword(psw)
	}

	topic, err = toMqttTopic(topic)
	if err != nil {
		return nil, err
	}
	hasher := sha3.New256()
	hasher.Write([]byte(serialNumber))
	mc.topic = fmt.Sprintf("$share/DATA_BRIDGE_%s/%s", hex.EncodeToString(hasher.Sum(nil)), topic)

	opts.SetClientID(podName)

	if enableSsl {
		opts.SetTLSConfig(newTLSConfig())
	}

	opts.SetAutoReconnect(true)
	opts.SetOrderMatters(false)

	opts.SetOnConnectHandler(func(client MQTT.Client) {
		zap.S().Infof("connected to MQTT broker %s", broker)
	})
	opts.SetConnectionLostHandler(func(client MQTT.Client, err error) {
		// sending os.Exit here will trigger the graceful shutdown
		zap.S().Fatalf("connection lost to MQTT broker %s: %v", broker, err)
	})

	mc.client = MQTT.NewClient(opts)
	if token := mc.client.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	var arcSize int
	arcSize, err = env.GetAsInt("MESSSAGE_LRU_SIZE", false, 1_000_000)
	if err != nil {
		zap.S().Error(err)
	}
	arc, err = lru.NewARC(arcSize)
	if err != nil {
		zap.S().Error(err)
	}

	return
}

func (m *mqttClient) getProducerStats() (messages uint64) {
	return m.sent.Load()
}

func (m *mqttClient) getConsumerStats() (messages uint64) {
	return m.recv.Load()
}

func (m *mqttClient) startProducing(messageChan chan kafka.Message, split int) {
	go func() {
		for {
			msg := <-messageChan

			var err error
			msg.Topic, err = toMqttTopic(msg.Topic)
			if err != nil {
				zap.S().Warnf("skipping message: %s", err)
				continue
			}

			if !isValidMqttMessage(msg) {
				zap.S().Warnf("skipping message: %s", msg.Topic)
				continue
			}

			m.client.Publish(msg.Topic, 1, false, msg.Value)
			m.sent.Add(1)
		}
	}()
}

func (m *mqttClient) startConsuming(messageChan chan kafka.Message) {
	go func() {
		if token := m.client.Subscribe(m.topic, 1, func(client MQTT.Client, msg MQTT.Message) {
			messageChan <- kafka.Message{
				Topic: msg.Topic(),
				Value: msg.Payload(),
			}
			m.recv.Add(1)
		}); token.Wait() && token.Error() != nil {
			zap.S().Fatalf("failed to subscribe: %s", token.Error())
		}
	}()
}

func (m *mqttClient) shutdown() error {
	zap.S().Infof("disconnecting from MQTT broker")
	m.client.Disconnect(250)
	return nil
}

func isValidMqttMessage(msg kafka.Message) bool {
	if !json.Valid(msg.Value) {
		zap.S().Warnf("not a valid json in message: %s", msg.Topic, string(msg.Value))
		return false
	}

	// Check if message is known
	hasher := sha3.New512()
	_, _ = hasher.Write([]byte(msg.Topic))
	_, _ = hasher.Write(msg.Value)
	hash := hasher.Sum(nil)
	// hash to string
	hashStr := string(hash)

	// Uses Get to re-validate the entry
	if _, ok := arc.Get(hashStr); ok {
		zap.S().Warnf("message already processed: %s", msg.Topic)
		return false
	}
	arc.Add(hashStr, true)

	return true
}

func isValidMqttTopic(topic string) bool {
	return regexp.MustCompile(`^\w[\w/#+]+[\w#]$`).MatchString(topic)
}

func toMqttTopic(topic string) (string, error) {
	if strings.HasPrefix(topic, "$share") {
		topic = string(regexp.MustCompile(`\$share\/DATA_BRIDGE_(.*?)\/`).ReplaceAll([]byte(topic), []byte("")))
	}
	if isValidKafkaTopic(topic) {
		topic = strings.ReplaceAll(topic, ".*", "#")
		topic = strings.ReplaceAll(topic, ".", "/")
		return topic, nil
	} else if isValidMqttTopic(topic) {
		return topic, nil
	}

	return "", fmt.Errorf("invalid topic: %s", topic)
}

func newTLSConfig() *tls.Config {

	// Import trusted certificates from CAfile.pem.
	// Alternatively, manually add CA certificates to
	// default openssl CA bundle.
	certpool := x509.NewCertPool()
	pemCerts, err := os.ReadFile("/SSL_certs/mqtt/ca.crt")
	if err == nil {
		ok := certpool.AppendCertsFromPEM(pemCerts)
		if !ok {
			zap.S().Errorf("failed to parse root certificate")
		}
	} else {
		zap.S().Errorf("error reading CA certificate: %s", err)
	}

	zap.S().Debugf("CA cert: %s", pemCerts)

	// Import client certificate/key pair
	cert, err := tls.LoadX509KeyPair("/SSL_certs/mqtt/tls.crt", "/SSL_certs/mqtt/tls.key")
	if err != nil {
		// Read /SSL_certs/mqtt/tls.crt
		var file []byte
		file, err = os.ReadFile("/SSL_certs/mqtt/tls.crt")
		if err != nil {
			zap.S().Errorf("error reading client certificate: %s", err)
		}
		zap.S().Fatalf("error reading client certificate: %s (File: %s)", err, file)
	}

	zap.S().Debugf("client cert: %v", cert)

	// Just to print out the client certificate..
	cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		zap.S().Fatalf("error parsing client certificate: %s", err)
	}

	skipVerify, err := env.GetAsBool("INSECURE_SKIP_VERIFY", false, true)
	if err != nil {
		zap.S().Error(err)
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
		InsecureSkipVerify: skipVerify,
		// Certificates = list of certs client sends to server.
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}
}
