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
	"reflect"
	"testing"
)

/*
Test ist dependant on pysical testing environment with device with ip inside of the specified ipRange

	func TestDiscoverDevices(t *testing.T) {
		ipRange := "192.168.10.17/32" //CIDR Notation
		discoveredDeviceInformation, err := DiscoverDevices(ipRange)
		if err != nil {
			t.Error(err)
		}
		fmt.Println(discoveredDeviceInformation)
	}
*/
func TestConvertCidrToIpRange(t *testing.T) {
	cidr := "10.0.0.0/24"
	start, finish, err := ConvertCidrToIpRange(cidr)
	if err != nil {
		t.Error(err)
	}
	if reflect.DeepEqual(start, 167772160) {
		t.Errorf("Wrong calculated ip start value: %v instead of 167772160(correct)", start)
	}
	if reflect.DeepEqual(finish, 167772415) {
		t.Errorf("Wrong calculated ip finish value: %v instead of 167772415(correct)", finish)
	}
}
