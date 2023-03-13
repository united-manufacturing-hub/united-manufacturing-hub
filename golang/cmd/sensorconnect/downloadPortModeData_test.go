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
