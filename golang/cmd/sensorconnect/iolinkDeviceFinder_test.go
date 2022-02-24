package main

import (
	"reflect"
	"testing"
)

/* Test ist dependant on pysical testing environment with device with ip inside of the specified ipRange
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
