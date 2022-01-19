package main

import (
	"strconv"
)

type SensorDataInformation struct {
	Cid        int                    `json:"cid"`
	SensorData map[string]interface{} `json:"data"`
}

// createSensorDataRequestBody creates the POST request body for ifm gateways. The body is made to simultaneously request sensordata of the ports 1 - numberOfPorts.
func createSensorDataRequestBody(numberOfPorts int) []byte {
	// Payload to send to the gateways
	var payload = []byte(`{
	"code":"request",
	"cid":25,
	"adr":"/getdatamulti",
	"data":{
		"datatosend":[
			"/iolinkmaster/port[1]/iolinkdevice/deviceid",
			"/iolinkmaster/port[1]/iolinkdevice/pdin",
			"/iolinkmaster/port[1]/iolinkdevice/vendorid",
			"/iolinkmaster/port[1]/pin2in"`)
	// repeat for other ports
	for i := 2; i <= numberOfPorts; i++ {
		currentPort := []byte(strconv.Itoa(i))
		payload = append(payload, []byte(`,
			"/iolinkmaster/port[`)...)
		payload = append(payload, currentPort...)
		payload = append(payload, []byte(`]/iolinkdevice/deviceid",
			"/iolinkmaster/port[`)...)
		payload = append(payload, currentPort...)
		payload = append(payload, []byte(`]/iolinkdevice/pdin",
			"/iolinkmaster/port[`)...)
		payload = append(payload, currentPort...)
		payload = append(payload, []byte(`]/iolinkdevice/vendorid",
			"/iolinkmaster/port[`)...)
		payload = append(payload, currentPort...)
		payload = append(payload, []byte(`]/pin2in"`)...)
	}
	payload = append(payload, []byte(`
		]
	}
}`)...)
	return payload
}
