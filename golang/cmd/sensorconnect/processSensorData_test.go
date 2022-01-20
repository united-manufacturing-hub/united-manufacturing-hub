package main

import (
	"fmt"
	"reflect"
	"testing"
)

// only works with functioning test device on correct ip: http://192.168.10.17
func TestExtractIntFromSensorDataMap(t *testing.T) {
	var deviceInfo DiscoveredDeviceInformation
	deviceInfo.ProductCode = "AL1350"
	deviceInfo.Url = "http://192.168.10.17/"
	sensorData, err := GetSensorDataMap(deviceInfo)
	if err != nil {
		t.Error("Problem with GetModeStatusStruct")
	}
	keyDeviceid := "/iolinkmaster/port[" + "2" + "]/iolinkdevice/deviceid"
	testInt := extractIntFromSensorDataMap(keyDeviceid, "data", sensorData)
	fmt.Printf("TestInt: %v", testInt)
	if testInt != 1028 {
		t.Error("Problem with extractIntFromSensorDataMap")
	}
}

func TestExtractInt64FromSensorDataMap(t *testing.T) {
	var deviceInfo DiscoveredDeviceInformation
	deviceInfo.ProductCode = "AL1350"
	deviceInfo.Url = "http://192.168.10.17/"
	sensorData, err := GetSensorDataMap(deviceInfo)
	if err != nil {
		t.Error("Problem with GetModeStatusStruct")
	}
	keyDeviceid := "/iolinkmaster/port[" + "2" + "]/iolinkdevice/deviceid"
	testInt := extractInt64FromSensorDataMap(keyDeviceid, "data", sensorData)
	fmt.Printf("TestInt: %v", testInt)
	if testInt != 1028 {
		t.Error("Problem with extractInt64FromSensorDataMap")
	}
}

func TestExtractByteArrayFromSensorDataMap(t *testing.T) {
	var deviceInfo DiscoveredDeviceInformation
	deviceInfo.ProductCode = "AL1350"
	deviceInfo.Url = "http://192.168.10.17/"
	sensorData, err := GetSensorDataMap(deviceInfo)
	if err != nil {
		t.Error("Problem with GetModeStatusStruct")
	}
	keyDeviceid := "/iolinkmaster/port[" + "2" + "]/iolinkdevice/deviceid"
	testByteArray := extractByteArrayFromSensorDataMap(keyDeviceid, "data", sensorData)
	fmt.Printf("TestInt: %v", testByteArray)
	comparisonArray := []byte("1028")
	if !reflect.DeepEqual(testByteArray, comparisonArray) {
		t.Error("Problem with extractByteArrayFromSensorDataMap")
	}
}

func TestBitConversions(t *testing.T) {
	longHexString := "0000FC000001FF000000FF000161FF000025FF03"
	length := len(longHexString) * 4
	bitString := HexToBin(longHexString)
	fmt.Println(bitString)
	bitStringPadded := zeroPadding(bitString, length)
	fmt.Println(bitStringPadded)
	endHexString := BinToHex(bitStringPadded)
	endHexStringPadded := zeroPadding(endHexString, len(longHexString))
	fmt.Println(endHexStringPadded)
	fmt.Println(longHexString)
	if endHexStringPadded != "0000fc000001ff000000ff000161ff000025ff03" {
		t.Error("Problem with BitConversions")
	}
}
