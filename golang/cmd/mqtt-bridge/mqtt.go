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
	pemCerts, err := ioutil.ReadFile("/SSL_certs/" + mode + "/intermediate_CA.pem")
	if err == nil {
		certpool.AppendCertsFromPEM(pemCerts)
	}

	// Import client certificate/key pair
	cert, err := tls.LoadX509KeyPair("/SSL_certs/"+mode+"/"+clientID+".pem", "/SSL_certs/"+mode+"/"+clientID+"-privkey.pem")
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
func getOnMessageRecieved(mode string, pg *goque.PrefixQueue) func(MQTT.Client, MQTT.Message) {

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
func setupMQTT(clientID string, mode string, mqttBrokerURL string, MQTTTopic string, SSLEnabled bool, pg *goque.PrefixQueue) (MQTTClient MQTT.Client) {

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

	zap.S().Infof("MQTT connection configured", clientID, mode, mqttBrokerURL, MQTTTopic, SSLEnabled)

	// Start the connection
	MQTTClient = MQTT.NewClient(opts)
	if token := MQTTClient.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	// subscribe (important: cleansession needs to be false, otherwise it must be specified in OnConnect
	if token := MQTTClient.Subscribe(MQTTTopic, 2, getOnMessageRecieved(mode, pg)); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	return
}
