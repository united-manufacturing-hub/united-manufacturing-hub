package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"os"
	"time"

	"go.uber.org/zap"
)

var discoveredDeviceInformation []DiscoveredDeviceInformation
var portModeMap map[int]int
var sensorDataMap map[string]interface{}
var ioDeviceMap map[IoddFilemapKey]IoDevice
var fileInfoSlice []os.FileInfo

var kafkaProducerClient *kafka.Producer
var kafkaAdminClient *kafka.AdminClient

func main() {
	logger, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	// Read environment variables for Kafka
	KafkaBoostrapServer := os.Getenv("KAFKA_BOOSTRAP_SERVER")
	kafkaProducerClient, kafkaAdminClient, _ = setupKafka(KafkaBoostrapServer)

	ipRange := os.Getenv("IP_RANGE") // 192.168.10.17/32
	zap.S().Infof("Scanning IP range: %s", ipRange)
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

	select {} // block forever
}

func continuousSensorDataProcessing(updateIoddIoDeviceMapChan chan IoddFilemapKey) {
	zap.S().Debugf("Starting sensor data processing daemon")
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

		err = processSensorData(sensorDataMap, deviceInfo, portModeMap, ioDeviceMap, updateIoddIoDeviceMapChan)
		if err != nil {
			zap.S().Errorf("processSensorData produced the error: %v", err)
		}
	}

}

func continuousDeviceSearch(ticker *time.Ticker, updaterChan chan struct{}, ipRange string) {
	zap.S().Debugf("Starting device search daemon")
	for {
		select {
		case <-ticker.C:
			var err error
			discoveredDeviceInformation, err = DiscoverDevices(ipRange)
			zap.S().Debugf("The discovered devices are: %v \n", discoveredDeviceInformation)
			if err != nil {
				zap.S().Errorf("DiscoverDevices produced the error: %v", err)
				continue
			}
		case <-updaterChan:
			return
		}
	}

}
func ioddDataDaemon(updateIoddIoDeviceMapChan chan IoddFilemapKey, updaterChan chan struct{}, relativeDirectoryPath string) {
	zap.S().Debugf("Starting iodd data daemon")
	for {
		select {
		case ioddFilemapKey := <-updateIoddIoDeviceMapChan:
			var err error
			ioDeviceMap, _, err = AddNewDeviceToIoddFilesAndMap(ioddFilemapKey, relativeDirectoryPath, ioDeviceMap, fileInfoSlice)
			zap.S().Debugf("added new ioDevice to map: %v", ioddFilemapKey)
			if err != nil {
				zap.S().Errorf("AddNewDeviceToIoddFilesAndMap produced the error: %v", err)
			}
			continue
		case <-updaterChan:
			return
		}
	}
}

func initializeIoddData(relativeDirectoryPath string) (deviceMap map[IoddFilemapKey]IoDevice, fileInfo []os.FileInfo, err error) {
	deviceMap = make(map[IoddFilemapKey]IoDevice)

	var ifmIoddFilemapKey IoddFilemapKey
	ifmIoddFilemapKey.DeviceId = 1028
	ifmIoddFilemapKey.VendorId = 310
	var siemensIoddFilemapKey IoddFilemapKey
	siemensIoddFilemapKey.DeviceId = 278531
	siemensIoddFilemapKey.VendorId = 42

	deviceMap, fileInfo, err = AddNewDeviceToIoddFilesAndMap(ifmIoddFilemapKey, relativeDirectoryPath, deviceMap, fileInfo)
	if err != nil {
		zap.S().Errorf("AddNewDeviceToIoddFilesAndMap produced the error: %v", err)
	}
	deviceMap, fileInfo, err = AddNewDeviceToIoddFilesAndMap(siemensIoddFilemapKey, relativeDirectoryPath, deviceMap, fileInfo)
	if err != nil {
		zap.S().Errorf("AddNewDeviceToIoddFilesAndMap produced the error: %v", err)
	}

	return
}
