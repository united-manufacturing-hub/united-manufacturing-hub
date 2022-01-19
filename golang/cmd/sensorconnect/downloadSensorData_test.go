package main

import (
	"fmt"
	"testing"
)

func TestCreateRequestBody(t *testing.T) {
	testOutputThreePorts := createRequestBody(3)
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
