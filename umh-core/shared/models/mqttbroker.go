package models

type MQTTBroker struct {
	Ip       string `json:"ip"`
	Port     uint32 `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}
