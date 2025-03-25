package mgmtconfig

type Brokers struct {
	MQTT  MQTTBroker  `json:"mqtt"`
	Kafka KafkaBroker `json:"kafka"`
}
