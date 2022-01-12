package main

import (
	"fmt"
	"testing"
)

func TestDiscoverDevices(t *testing.T) {
	ipRange := "192.168.10.17/32" //CIDR Notation for 192.168.10.17 and 192.168.10.18
	discoveredDeviceInformation, err := DiscoverDevices(ipRange)
	if err != nil {
		t.Error(err)
	}
	fmt.Println(discoveredDeviceInformation)
}
