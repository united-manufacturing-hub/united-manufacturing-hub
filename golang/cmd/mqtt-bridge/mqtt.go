package main

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"

	"github.com/beeker1121/goque"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
)

// newTLSConfig returns the TLS config for a given clientID and mode
func newTLSConfig(clientID string, mode string) *tls.Config {

	// Import trusted certificates from CAfile.pem.
	// Alternatively, manually add CA certificates to
	// default openssl CA bundle.
	certpool := x509.NewCertPool()
	pemCerts, err := ioutil.ReadFile("/SSL_certs/" + mode + "/ca.crt")
	if err == nil {
		certpool.AppendCertsFromPEM(pemCerts)
	}

	// Import client certificate/key pair
	cert, err := tls.LoadX509KeyPair("/SSL_certs/"+mode+"/tls.crt", "/SSL_certs/"+mode+"/tls.key")
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

// getOnMessageRecieved gets the function onMessageReceived, that is called everytime a message is recieved by a specific topic
func getOnMessageRecieved(mode string, pg *goque.Queue) func(MQTT.Client, MQTT.Message) {

	return func(client MQTT.Client, message MQTT.Message) {

		topic := message.Topic()
		payload := message.Payload()

		zap.S().Infof("onMessageReceived", mode, topic, payload)

		go storeMessageIntoQueue(topic, payload, mode, pg)
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

// setupMQTT setups MQTT and connect to the broker
func setupMQTT(clientID string, mode string, mqttBrokerURL string, subMQTTTopic string, SSLEnabled bool, pg *goque.Queue, subscribeToTopic bool) (MQTTClient MQTT.Client) {

	opts := MQTT.NewClientOptions()
	opts.AddBroker(mqttBrokerURL)

	if SSLEnabled {
		tlsconfig := newTLSConfig(clientID, mode)
		opts.SetClientID(clientID).SetTLSConfig(tlsconfig)
	} else {
		opts.SetClientID(clientID)
	}

	opts.SetAutoReconnect(true)
	opts.SetOnConnectHandler(onConnect)
	opts.SetConnectionLostHandler(onConnectionLost)

	zap.S().Infof("MQTT connection configured", clientID, mode, mqttBrokerURL, subMQTTTopic, SSLEnabled)

	// Start the connection
	MQTTClient = MQTT.NewClient(opts)
	if token := MQTTClient.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	// Can be deactivated, e.g. if one does not want to recieve all data from remote broker
	if subscribeToTopic {

		zap.S().Infof("MQTT subscribed", mode, subMQTTTopic)
		// subscribe (important: cleansession needs to be false, otherwise it must be specified in OnConnect
		if token := MQTTClient.Subscribe(subMQTTTopic+"/#", 2, getOnMessageRecieved(mode, pg)); token.Wait() && token.Error() != nil {
			panic(token.Error())
		}
	}

	return
}
