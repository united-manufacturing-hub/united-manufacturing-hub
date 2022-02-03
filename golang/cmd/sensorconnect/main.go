package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
)

var discoveredDeviceInformation []DiscoveredDeviceInformation

//var ioDeviceMap map[IoddFilemapKey]IoDevice
var ioDeviceMap sync.Map

var fileInfoSlice []os.FileInfo

var kafkaProducerClient *kafka.Producer
var kafkaAdminClient *kafka.AdminClient
var transmitterId string

var buildtime string

func main() {
	time.Sleep(30 * time.Second)
	logger, _ := zap.NewDevelopment()
	//logger, _ := zap.NewProduction()
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	zap.S().Infof("This is sensorconnect build date: %s", buildtime)

	internal.InitMemcache()

	// Read environment variables for Kafka
	KafkaBoostrapServer := os.Getenv("KAFKA_BOOSTRAP_SERVER")
	kafkaProducerClient, kafkaAdminClient, _ = setupKafka(KafkaBoostrapServer)

	ipRange := os.Getenv("IP_RANGE")
	zap.S().Infof("Scanning IP range: %s", ipRange)

	transmitterId = os.Getenv("TRANSMITTERID")

	relativeDirectoryPath := os.Getenv("IODD_FILE_PATH")
	var err error
	// creating ioDeviceMap and downloading initial set of iodd files

	ioDeviceMap = sync.Map{}

	fileInfoSlice, err = initializeIoddData(relativeDirectoryPath)

	if err != nil {
		zap.S().Errorf("initializeIoddData produced the error: %v", err)
	}
	tickerSearchForDevices := time.NewTicker(5 * time.Second)
	defer tickerSearchForDevices.Stop()
	updateIoddIoDeviceMapChan := make(chan IoddFilemapKey)

	go continuousDeviceSearch(tickerSearchForDevices, ipRange)

	for len(discoveredDeviceInformation) == 0 {
		zap.S().Infof("No devices discovered yet.")
		time.Sleep(1 * time.Second)
	}
	go ioddDataDaemon(updateIoddIoDeviceMapChan, relativeDirectoryPath)

	for {
		for GetSyncMapLen(&ioDeviceMap) == 0 {
			zap.S().Infof("Initial iodd file download not yet complete, awaiting.")
			time.Sleep(1 * time.Second)
		}
		break
	}

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
			var portModeMap map[int]int
			var sensorDataMap map[string]interface{}

			portModeMap, err = GetPortModeMap(deviceInfo)
			if err != nil {
				zap.S().Errorf("GetPortModeMap produced the error: %v", err)
			}
			sensorDataMap, err = GetSensorDataMap(deviceInfo)
			if err != nil {
				zap.S().Errorf("GetSensorDataMap produced the error: %v", err)
			}

			processSensorData(deviceInfo, updateIoddIoDeviceMapChan, portModeMap, sensorDataMap)
		}
	}
}

func continuousDeviceSearch(ticker *time.Ticker, ipRange string) {
	zap.S().Debugf("Starting device search daemon")
	for {
		select {
		case <-ticker.C:
			var err error
			zap.S().Debugf("Starting device scan..")
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
			_, err = AddNewDeviceToIoddFilesAndMap(ioddFilemapKey, relativeDirectoryPath, fileInfoSlice)
			zap.S().Debugf("added new ioDevice to map: %v", ioddFilemapKey)
			if err != nil {
				zap.S().Errorf("AddNewDeviceToIoddFilesAndMap produced the error: %v", err)
			}
			continue
		}
	}
}

func initializeIoddData(relativeDirectoryPath string) (fileInfo []os.FileInfo, err error) {

	var ifmIoddFilemapKey IoddFilemapKey
	ifmIoddFilemapKey.DeviceId = 1028
	ifmIoddFilemapKey.VendorId = 310
	var siemensIoddFilemapKey IoddFilemapKey
	siemensIoddFilemapKey.DeviceId = 278531
	siemensIoddFilemapKey.VendorId = 42

	fileInfo, err = AddNewDeviceToIoddFilesAndMap(ifmIoddFilemapKey, relativeDirectoryPath, fileInfo)
	if err != nil {
		zap.S().Errorf("AddNewDeviceToIoddFilesAndMap produced the error: %v", err)
	}
	fileInfo, err = AddNewDeviceToIoddFilesAndMap(siemensIoddFilemapKey, relativeDirectoryPath, fileInfo)
	if err != nil {
		zap.S().Errorf("AddNewDeviceToIoddFilesAndMap produced the error: %v", err)
	}

	return
}

func GetSyncMapLen(p *sync.Map) int {
	mapLen := 0
	p.Range(func(key, value interface{}) bool {
		mapLen += 1
		return true
	})

	return mapLen
}
