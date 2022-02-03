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
var transmitterId string

var buildtime string

func main() {
	logger, _ := zap.NewDevelopment()
	//logger, _ := zap.NewProduction()
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	zap.S().Infof("This is sensorconnect build date: %s", buildtime)

	// Read environment variables for Kafka
	KafkaBoostrapServer := os.Getenv("KAFKA_BOOSTRAP_SERVER")
	kafkaProducerClient, kafkaAdminClient, _ = setupKafka(KafkaBoostrapServer)

	ipRange := os.Getenv("IP_RANGE")
	zap.S().Infof("Scanning IP range: %s", ipRange)

	transmitterId = os.Getenv("TRANSMITTERID")

	relativeDirectoryPath := os.Getenv("IODD_FILE_PATH")
	var err error
	// creating ioDeviceMap and downloading initial set of iodd files
	ioDeviceMap, fileInfoSlice, err = initializeIoddData(relativeDirectoryPath)
	zap.S().Debugf("ioDeviceMap len: %v", ioDeviceMap)
	if err != nil {
		zap.S().Errorf("initializeIoddData produced the error: %v", err)
	}
	tickerSearchForDevices := time.NewTicker(5 * time.Second)
	defer tickerSearchForDevices.Stop()
	updateIoddIoDeviceMapChan := make(chan IoddFilemapKey)

	go continuousDeviceSearch(tickerSearchForDevices, ipRange)

	time.Sleep(5 * time.Second)
	go ioddDataDaemon(updateIoddIoDeviceMapChan, relativeDirectoryPath)
	go continuousSensorDataProcessing(updateIoddIoDeviceMapChan)

	select {} // block forever
}

func continuousSensorDataProcessing(updateIoddIoDeviceMapChan chan IoddFilemapKey) {
	zap.S().Debugf("Starting sensor data processing daemon")
	for {
		var err error
		if len(discoveredDeviceInformation) == 0 {
			zap.S().Debugf("No devices !")
			time.Sleep(1 * time.Second)
			continue
		}
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
}

func continuousDeviceSearch(ticker *time.Ticker, ipRange string) {
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
		}
	}

}
func ioddDataDaemon(updateIoddIoDeviceMapChan chan IoddFilemapKey, relativeDirectoryPath string) {
	zap.S().Debugf("Starting iodd data daemon")
	for {
		select {
		case ioddFilemapKey := <-updateIoddIoDeviceMapChan:
			var err error
			zap.S().Debugf("[PRE-AddNewDeviceToIoddFilesAndMap] ioDeviceMap len: %v", ioDeviceMap)
			ioDeviceMap, _, err = AddNewDeviceToIoddFilesAndMap(ioddFilemapKey, relativeDirectoryPath, ioDeviceMap, fileInfoSlice)
			zap.S().Debugf("[POST-AddNewDeviceToIoddFilesAndMap] ioDeviceMap len: %v", ioDeviceMap)
			zap.S().Debugf("added new ioDevice to map: %v", ioddFilemapKey)
			if err != nil {
				zap.S().Errorf("AddNewDeviceToIoddFilesAndMap produced the error: %v", err)
			}
			continue
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
