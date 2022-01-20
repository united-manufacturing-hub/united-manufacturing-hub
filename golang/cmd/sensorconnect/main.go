package main

import (
	"time"

	"go.uber.org/zap"
)

var discoveredDeviceInformation []DiscoveredDeviceInformation

func main() {
	ipRange := "192.168.10.17/32"

	tickerSearchForDevices := time.NewTicker(5 * time.Second)
	defer tickerSearchForDevices.Stop()
	updaterChan := make(chan struct{})
	defer close(updaterChan) // close the channel

	go continuousDeviceSearch(tickerSearchForDevices, updaterChan, ipRange)

	select {} // block forever
}
func continuousDeviceSearch(ticker *time.Ticker, updaterChan chan struct{}, ipRange string) {
	for {
		select {
		case <-ticker.C:
			var err error
			discoveredDeviceInformation, err = DiscoverDevices(ipRange)
			if err != nil {
				zap.S().Errorf("DiscoverDevices produced the error: %v", err)
				continue
			}
		case <-updaterChan:
			return
		}
	}

}
