package main

import (
	"strconv"
	"time"

	"go.uber.org/zap"
)

func processSensorData(sensorDataMap map[string]interface{}, currentDeviceInformation DiscoveredDeviceInformation, portModeMap map[int]int) {
	timestampMs := getUnixTimestampMs()
	for portNumber, portMode := range portModeMap {
		//mqttRawTopic := fmt.Sprintf("ia/raw/%v/%v/X0%v", transmitterId, currentDeviceInformation.SerialNumber, portNumber)
		switch portMode {
		case 1: // digital input
			// get value from sensorDataMap
			portNumberString := strconv.Itoa(portNumber)
			key := "/iolinkmaster/port[" + portNumberString + "]/pin2in"
			valuePin2In := sensorDataMap[key]
			elementPin2InMap := valuePin2In.(map[string]interface{})
			dataPin2In := elementPin2InMap["data"].([]byte)

			// Payload to send to the gateways
			var payload = []byte(`{
				"serial_number":`)
			payload = append(payload, []byte(currentDeviceInformation.SerialNumber)...)
			payload = append(payload, []byte(`-X0`)...)
			payload = append(payload, []byte(strconv.Itoa(portNumber))...)
			payload = append(payload, []byte(`,
			"timestamp_ms:`)...)
			payload = append(payload, []byte(strconv.Itoa(timestampMs))...)
			payload = append(payload, []byte(`,
			"type":DI,
			"connected":connected
			"value":`)...)
			payload = append(payload, dataPin2In...)
			payload = append(payload, []byte(`}`)...)

		case 2: // digital output
			// Todo
			continue
		case 3: // IO-Link
			// check connection status
			portNumberString := strconv.Itoa(portNumber)
			key := "/iolinkmaster/port[" + portNumberString + "]/iolinkdevice/pdin"
			valuePdin := sensorDataMap[key]
			elementPdinMap := valuePdin.(map[string]interface{})
			connectionCode := elementPdinMap["code"].(int)
			if connectionCode != 200 {
				zap.S().Errorf("connection code of port %v not 200 but: %v", portNumber, connectionCode)
				continue
			}

		case 4: // port inactive or problematic (custom port mode: not transmitted from IO-Link-Gateway, but set by sensorconnect)
			continue
		}
	}
}

func getUnixTimestampMs() (timestampMs int) {
	t := time.Now()
	timestampMs = int(t.UnixNano() / 1000000)
	return
}
