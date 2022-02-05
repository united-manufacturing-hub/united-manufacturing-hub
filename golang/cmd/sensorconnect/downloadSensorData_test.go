package main

import (
	"fmt"
	"testing"
)

func TestCreateSensorDataRequestBody(t *testing.T) {
	testOutputThreePorts := createSensorDataRequestBody(3)
	fmt.Println(string(testOutputThreePorts[:]))
	if testOutputThreePorts == nil {
		t.Error("err")
	}
}

/* only works with functioning test device on correct ip: http://192.168.10.17
func TestGetSensorDataMap(t *testing.T) {
	var deviceInfo DiscoveredDeviceInformation
	deviceInfo.ProductCode = "AL1350"
	deviceInfo.Url = "http://192.168.10.17/"
	sensorData, err := GetSensorDataMap(deviceInfo)
	fmt.Printf("PortModeMap: %v", sensorData)
	if err == nil {
		t.Error("Problem with GetModeStatusStruct")
	}
}
*/
