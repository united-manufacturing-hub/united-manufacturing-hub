package main

import (
	"fmt"
	"testing"
)

func TestProcessSensorData(t *testing.T) {
	timestamp_ms := processSensorData()
	fmt.Println(timestamp_ms)
	if timestamp_ms > 10 {
		t.Error("err")
	}
}
