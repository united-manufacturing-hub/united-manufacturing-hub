package main

import (
	"testing"
)

/*
only works with functioning test device on correct ip: http://192.168.10.17

	func TestGetPortModeMap(t *testing.T) {
		var deviceInfo DiscoveredDeviceInformation
		deviceInfo.ProductCode = "AL1350"
		deviceInfo.Url = "http://192.168.10.17/"
		sensorData, err := GetPortModeMap(deviceInfo)
		fmt.Printf("PortModeMap: %d", sensorData) //"%+v",
		if err != nil {
			t.Error("Problem with GetModeStatusStruct")
		}
	}
*/
func TestExtractIntFromString(t *testing.T) {
	testString := "/iolinkmaster/port[23]/mode"
	answerInt, err := extractIntFromString(testString)
	if answerInt != 23 {
		t.Errorf("wrong number extracted: %v", answerInt)
	}
	if err != nil {
		t.Errorf("error detected %v", err)
	}
	testProblemString := "/234iolinkmaster/port[23]/mode"
	answerProblemInt, err := extractIntFromString(testProblemString)
	if answerProblemInt != -1 {
		t.Errorf("wrong number extracted: %v", answerInt)
	}
	if err == nil {
		t.Errorf("no error detected")
	}
}
