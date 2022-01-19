package main

import (
	"fmt"
	"testing"
)

func TestUnmarshalModeInformation(t *testing.T) {

}
func TestCreateModeRequestBody(t *testing.T) {
	testOutputThreePorts := createModeRequestBody(3)
	fmt.Println(string(testOutputThreePorts[:]))
	if testOutputThreePorts == nil {
		t.Error("err")
	}
}

func TestFindNumberOfPorts(t *testing.T) {
	numberOfPorts := findNumberOfPorts("AL1352")
	if numberOfPorts != 8 {
		t.Error("Incorrect number of Ports returned.")
	}
	numberOfPorts = findNumberOfPorts("TestDefault")
	if numberOfPorts != 4 {
		t.Error("Incorrect number of Ports for default returned.")
	}
}
func TestGetModeStatusStruct(t *testing.T) {
	var deviceInfo DiscoveredDeviceInformation
	deviceInfo.ProductCode = "AL1350"
	deviceInfo.Url = "http://192.168.10.17/"
	sensorData := GetModeStatusStruct(deviceInfo)
	fmt.Println(sensorData) //"%+v",
	t.Error("Incorrect number of Ports for default returned.")
}
