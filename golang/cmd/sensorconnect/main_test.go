package main

import (
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestMain(m *testing.M) {

	log.Println("Do stuff BEFORE the tests!")
	exitVal := m.Run()
	log.Println("Do stuff AFTER the tests!")

	os.Exit(exitVal)

}

func TestCopiedMain(t *testing.T) {
	ipRange := "192.168.10.17/32"
	relativeDirectoryPath := "../sensorconnect/IoddFiles/"
	var err error
	// creating ioDeviceMap and downloading initial set of iodd files
	ioDeviceMap, fileInfoSlice, err = initializeIoddData(relativeDirectoryPath)
	if err != nil {
		zap.S().Errorf("initializeIoddData produced the error: %v", err)
	}
	tickerSearchForDevices := time.NewTicker(5 * time.Second)
	defer tickerSearchForDevices.Stop()
	updaterChan := make(chan struct{})
	defer close(updaterChan) // close the channel
	updateIoddIoDeviceMapChan := make(chan IoddFilemapKey)

	go continuousDeviceSearch(tickerSearchForDevices, updaterChan, ipRange)
	go ioddDataDaemon(updateIoddIoDeviceMapChan, updaterChan, relativeDirectoryPath)
	go continuousSensorDataProcessing(updateIoddIoDeviceMapChan)
	fmt.Println(discoveredDeviceInformation)

	select {} // block forever
}

func TestContinuousDeviceSearch(t *testing.T) {
	ipRange := "192.168.10.17/32"
	tickerSearchForDevices := time.NewTicker(5 * time.Second)
	defer tickerSearchForDevices.Stop()
	updaterChan := make(chan struct{})
	go continuousDeviceSearch(tickerSearchForDevices, updaterChan, ipRange)
	select {}
}

func TestInitializeIoddData(t *testing.T) {
	relativeDirectoryPath := "../sensorconnect/IoddFiles/"
	deviceMap, fileinfo, err := initializeIoddData(relativeDirectoryPath)
	fmt.Printf("Devicemap: %v", deviceMap)
	fmt.Printf("fileinfo: %v", fileinfo)
	fmt.Printf("err: %v", err)
	t.Error(err)
}
