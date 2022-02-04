package main

import (
	"fmt"
	"testing"
)

/*
// integration test based on working ifm sensor on gateway at specific ip address
func TestProcessSensorData(t *testing.T) {
	// first remove all files from specified path
	relativeDirectoryPath := "../sensorconnect/IoddFiles/"
	removeFilesFromDirectory(relativeDirectoryPath)
	ipRange := "192.168.10.17/32" //CIDR Notation for 192.168.10.17 and 192.168.10.18

	deviceInfo, err := DiscoverDevices(ipRange)
	if err != nil {
		t.Error(err)
	}

	sensorDataMap, err := GetSensorDataMap(deviceInfo[0])
	if err != nil {
		t.Error("Problem with GetModeStatusStruct")
	}

	portModeMap, err := GetPortModeMap(deviceInfo[0])
	fmt.Printf("PortModeMap: %d", portModeMap) //"%+v",
	if err != nil {
		t.Error("Problem with GetModeStatusStruct")
	}

	//Declare Variables
	ioDeviceMap := make(map[IoddFilemapKey]IoDevice)
	var fileInfoSlice []os.FileInfo

	var ioddFilemapKey IoddFilemapKey
	//ioddFilemapKey.DeviceId = 278531
	//ioddFilemapKey.VendorId = 42
	ioddFilemapKey.DeviceId = 1028
	ioddFilemapKey.VendorId = 310
	// execute function and check for errors
	ioDeviceMap, _, err = AddNewDeviceToIoddFilesAndMap(ioddFilemapKey, relativeDirectoryPath, ioDeviceMap, fileInfoSlice)
	fmt.Println(ioDeviceMap)
	if err != nil {
		t.Error(err)
	}
	updateIoddIoDeviceMapChan := make(chan IoddFilemapKey)
	updaterChan := make(chan struct{})
	go ioddDataDaemon(updateIoddIoDeviceMapChan, updaterChan, relativeDirectoryPath)
	err = processSensorData(sensorDataMap, deviceInfo[0], nil, nil)
	if err != nil {
		t.Error(err)
	}
}

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
*/
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
