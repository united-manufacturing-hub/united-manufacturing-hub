package main

import (
	"fmt"
	"time"
)

func processSensorData() (timestamp_ms int) {
	t := time.Now()
	timestamp_ms = int(t.UnixNano() / 1000000)
	fmt.Println(timestamp_ms)
	return
}
