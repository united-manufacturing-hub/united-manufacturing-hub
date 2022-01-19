package main

import (
	"fmt"
	"testing"
)

func TestCreateSensorDataRequestBody(t *testing.T) {
	testOutputThreePorts := createSensorDataRequestBody(3)
	fmt.Println(string(testOutputThreePorts[:]))
	if testOutputThreePorts != nil {
		t.Error("err")
	}
}
