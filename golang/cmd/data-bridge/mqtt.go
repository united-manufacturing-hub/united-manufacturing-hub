package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/eclipse/paho.golang/packets"
	"github.com/eclipse/paho.golang/paho"
	"github.com/goccy/go-json"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"golang.org/x/crypto/sha3"
)

type mqttClient struct {
	client *paho.Client
	topic  string
}

func newMqttClient(broker, topic, psw, serialNumber string, enableSsl bool) (mc *mqttClient, err error) {
	podName, err := env.GetAsString("POD_NAME", true, "")
	if err != nil {
		return nil, err
	}

	if !isValidMqttTopic(topic) {
		return nil, fmt.Errorf("invalid MQTT topic: %s", topic)
	}
	hasher := sha3.New256()
	hasher.Write([]byte(serialNumber))
	mc.topic = fmt.Sprintf("$share/DATA_BRIDGE_%s/%s", hex.EncodeToString(hasher.Sum(nil)), topic)

	config := paho.ClientConfig{
		ClientID:      podName,
		OnClientError: func(err error) { zap.S().Errorf("server requested disconnect: %s\n", err) },
		OnServerDisconnect: func(d *paho.Disconnect) {
			if d.Properties != nil {
				zap.S().Errorf("server requested disconnect: %s\n", d.Properties.ReasonString)
			} else {
				zap.S().Errorf("server requested disconnect; reason code: %d\n", d.ReasonCode)
			}
		},
	}

	mc.client, err = establishBrokerConnection(context.Background(), broker, psw, enableSsl, config)
	return mc, err
}

// newTLSConfig returns the TLS config for a given clientID and mode
func newTLSConfig() tls.Config {

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

	skipVerify, err := env.GetAsBool("INSECURE_SKIP_VERIFY", false, true)
	if err != nil {
		zap.S().Error(err)
	}

	// Create tls.Config with desired tls properties
	/* #nosec G402 -- Remote verification is not yet implemented*/
	return tls.Config{
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

// establishBrokerConnection - establishes a connection with the broker retrying until successful or the
// context is cancelled (in which case nil will be returned).
func establishBrokerConnection(ctx context.Context, broker, psw string, enableSsl bool, cfg paho.ClientConfig) (cli *paho.Client, err error) {
	for {
		connCtx, cancelConnCtx := context.WithTimeout(ctx, 1*time.Minute)

		if enableSsl {
			tlsCfg := newTLSConfig()
			cfg.Conn, err = attemptTLSConnection(connCtx, broker, &tlsCfg)
		} else {
			cfg.Conn, err = attemptTCPConnection(connCtx, broker)
		}

		if err == nil {
			cli := paho.NewClient(cfg)

			cp := paho.Connect{
				ClientID:     cfg.ClientID,
				KeepAlive:    30,
				CleanStart:   true,
				Username:     "DATA_BRIDGE",
				UsernameFlag: true,
			}
			if len(psw) > 0 {
				cp.Password = []byte(psw)
				cp.PasswordFlag = true
			}

			_, err = cli.Connect(connCtx, &cp) // will return an error if the connection is unsuccessful (checks the reason code)
			if err == nil {                    // Successfully connected
				cancelConnCtx()
				return cli, nil
			}
		}
		cancelConnCtx()

		// Possible failure was due to outer context being cancelled
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Delay before attempting another connection
		select {
		case <-time.After(10 * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func attemptTCPConnection(ctx context.Context, address string) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", address)
	return packets.NewThreadSafeConn(conn), err
}

func attemptTLSConnection(ctx context.Context, address string, tlsConfig *tls.Config) (net.Conn, error) {
	d := tls.Dialer{
		Config: tlsConfig,
	}
	conn, err := d.DialContext(ctx, "tcp", address)
	return packets.NewThreadSafeConn(conn), err
}

func (m *mqttClient) getProducerStats() (sent uint64) {
	panic("not implemented")
}

func (m *mqttClient) getConsumerStats() (received uint64) {
	panic("not implemented")
}

func (m *mqttClient) startProducing(msgChan chan kafka.Message, split int) {
	go func() {
		for {
			msg := <-msgChan

			if strings.HasPrefix(msg.Topic, "$share") {
				msg.Topic = string(regexp.MustCompile(`\$share\/data-bridge-(.*?)\/`).ReplaceAll([]byte(msg.Topic), []byte("")))
			}

			msg.Topic = strings.ReplaceAll(msg.Topic, ".", "/")

		}
	}()
}

func (m *mqttClient) startConsuming(msgChan chan kafka.Message) {
	go func() {
		m.client.Router = paho.NewSingleHandlerRouter(func(p *paho.Publish) {
			msg := kafka.Message{
				Topic: p.Topic,
				Value: p.Payload,
			}

			msgChan <- msg
		})

		// Note: the SubscribeOptions map will become a slice in the next
		// release of the eclipse/paho.golang library (currently 0.11)
		if _, err := m.client.Subscribe(context.Background(), &paho.Subscribe{
			Subscriptions: map[string]paho.SubscribeOptions{
				m.topic: {
					QoS: 1,
				},
			},
		}); err != nil {
			zap.S().Errorf("error subscribing to topic %s: %s", m.topic, err)
			return
		}
		zap.S().Infof("subscribed to topic %s", m.topic)

		// TODO: block this function somehow, so it continues to read messages
	}()
}

func (m *mqttClient) shutdown() error {
	zap.S().Infof("Shutting down mqtt client")
	return m.client.Disconnect(&paho.Disconnect{ReasonCode: packets.DisconnectNormalDisconnection})
}

func isValidMqttMessage(message kafka.Message) bool {
	if strings.HasPrefix(message.Topic, "/") {
		zap.S().Warnf("Invalid MQTT topic: %s", message.Topic)
		return false
	}

	if strings.HasSuffix(message.Topic, "/") {
		zap.S().Warnf("Invalid MQTT topic: %s", message.Topic)
		return false
	}

	if !json.Valid(message.Value) {
		zap.S().Warnf("Not a valid json in message: %s", message.Topic)
		return false
	}

	if internal.IsSameOrigin(&message) {
		zap.S().Warnf("Message from same origin: %s", message.Topic)
		return false
	}

	if internal.IsInTrace(&message) {
		zap.S().Warnf("Message in trace: %s", message.Topic)
		return false
	}

	return true
}

func isValidMqttTopic(topic string) bool {
	if strings.HasPrefix(topic, "/") ||
		strings.HasSuffix(topic, "/") ||
		!regexp.MustCompile(`^(?!\$share)[\w/#+]+$`).MatchString(topic) {
		fmt.Printf("Invalid MQTT topic: %s\n", topic)
		return false
	}
	return true
}
