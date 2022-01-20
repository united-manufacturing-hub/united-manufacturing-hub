package main

import (
	"os"
	"time"

	"go.uber.org/zap"
)

var discoveredDeviceInformation []DiscoveredDeviceInformation
var portModeMap map[int]int
var sensorDataMap map[string]interface{}
var ioDeviceMap map[IoddFilemapKey]IoDevice
var fileInfoSlice []os.FileInfo

func main() {
	ipRange := "192.168.10.17/32"
	relativeDirectoryPath := "../sensorconnect/IoddFiles/"

	// creating ioDeviceMap and downloading initial set of iodd files
	ioDeviceMap, fileInfoSlice = initializeIoddData(relativeDirectoryPath)

	tickerSearchForDevices := time.NewTicker(5 * time.Second)
	defer tickerSearchForDevices.Stop()
	updaterChan := make(chan struct{})
	defer close(updaterChan) // close the channel

	go continuousDeviceSearch(tickerSearchForDevices, updaterChan, ipRange)
	go continuousSensorDataProcessing()

	select {} // block forever
}

func continuousSensorDataProcessing() {
	var err error
	for _, deviceInfo := range discoveredDeviceInformation {
		portModeMap, err = GetPortModeMap(deviceInfo)
		if err != nil {
			zap.S().Errorf("GetPortModeMap produced the error: %v", err)
		}
		sensorDataMap, err = GetSensorDataMap(deviceInfo)
		if err != nil {
			zap.S().Errorf("GetSensorDataMap produced the error: %v", err)
		}

	}

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

func initializeIoddData(relativeDirectoryPath string) (deviceMap map[IoddFilemapKey]IoDevice, fileInfo []os.FileInfo) {
	// todo
	return
}
