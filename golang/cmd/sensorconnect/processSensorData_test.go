// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"math"
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
		var fileInfoSlice []fs.DirEntry

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

// IO-Link Interface and System Specification V1.1.4 F.2.2
func TestBooleanTTConversions(t *testing.T) {
	tests := []struct {
		binary   string
		expected bool
	}{
		{
			binary:   "1",
			expected: true,
		},
		{
			binary:   "0",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("When binary is: %s, Then convertBinaryValue returns: %t", tt.binary, tt.expected), func(t *testing.T) {
			result := convertBinaryValue(tt.binary, "BooleanT")
			if result != tt.expected {
				t.Errorf("Expected %t, got %t", tt.expected, result)
			}
		})
	}
}

// IO-Link Interface and System Specification V1.1.4 F.2.5
func TestFloat32TConversions(t *testing.T) {
	tests := []struct {
		binary   string
		expected float32
	}{
		{
			binary:   "01000000010000000000000000000000",
			expected: 3.0,
		},
		{
			binary:   "11000000010000000000000000000000",
			expected: -3.0,
		},
		{
			binary:   "00000000000000000000000000000000",
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("When binary is: %s, Then convertBinaryValue returns: %f", tt.binary, tt.expected), func(t *testing.T) {
			result := convertBinaryValue(tt.binary, "Float32T")
			if result != tt.expected {
				t.Errorf("Expected %f, got %f", tt.expected, result)
			}
		})
	}
}

// IO-Link Interface and System Specification V1.1.4 F.2.3
func TestUIntegerTConversions(t *testing.T) {
	tests := []struct {
		binary   string
		expected uint64
	}{
		{
			binary:   "0001101",
			expected: 13,
		},
		{
			binary:   "10110111001011010",
			expected: 93786,
		},
		{
			binary:   "11111111111111111111111111111111",
			expected: math.MaxUint32,
		},
		{
			binary:   "1111111111111111111111111111111111111111111111111111111111111111",
			expected: math.MaxUint64,
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("When binary is: %s, Then convertBinaryValue returns: %v", tt.binary, tt.expected), func(t *testing.T) {
			result := convertBinaryValue(tt.binary, "UIntegerT")
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// IO-Link Interface and System Specification V1.1.4 F.2.4
func TestIntegerTConversions(t *testing.T) {
	tests := []struct {
		binary   string
		expected int
	}{
		{
			binary:   "10000000",
			expected: math.MinInt8,
		},
		{
			binary:   "01111111",
			expected: math.MaxInt8,
		},
		{
			binary:   "1000000000000000",
			expected: math.MinInt16,
		},
		{
			binary:   "10000000000000000000000000000000",
			expected: math.MinInt32,
		},
		{
			binary:   "1000000000000000000000000000000000000000000000000000000000000000",
			expected: math.MinInt64,
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("When binary is: %s, Then convertBinaryValue returns: %v", tt.binary, tt.expected), func(t *testing.T) {
			result := convertBinaryValue(tt.binary, "IntegerT")
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}
