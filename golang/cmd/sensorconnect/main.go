package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var mqttClient MQTT.Client

//var discoveredDeviceInformation []DiscoveredDeviceInformation
// key is url
//var discoveredDeviceInformation sync.Map
var discoveredDeviceChannel chan DiscoveredDeviceInformation
var forgetDeviceChannel chan DiscoveredDeviceInformation

//var ioDeviceMap map[IoddFilemapKey]IoDevice
var ioDeviceMap sync.Map

var fileInfoSlice []os.FileInfo

var kafkaProducerClient *kafka.Producer
var kafkaAdminClient *kafka.AdminClient
var transmitterId string

var buildtime string

var useKafka bool
var useMQTT bool

var deviceFinderFrequencyInS = 20
var deviceFinderTimeoutInS = 1

var lowestSensorTickTime int
var upperSensorTickTime int

var sensorTickTimeSteppingUp int
var sensorTickTimeSteppingDown int

var maxSensorErrorCount = uint64(10)

func main() {
	var logger *zap.Logger
	if os.Getenv("DEBUG") == "1" {
		time.Sleep(15 * time.Second)
		logger, _ = zap.NewDevelopment()
		zap.S().Debugf("Starting in DEBUG mode !")
	} else {
		logger, _ = zap.NewProduction()
	}
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	zap.S().Infof("This is sensorconnect build date: %s", buildtime)

	internal.InitMemcache()

	useKafka = os.Getenv("USE_KAFKA") == "1" || strings.ToLower(os.Getenv("USE_KAFKA")) == "true"
	useMQTT = os.Getenv("USE_MQTT") == "1" || strings.ToLower(os.Getenv("USE_MQTT")) == "true"

	if !useKafka && !useMQTT {
		zap.S().Errorf("Neither kafka nor MQTT output enabled, exiting !")
		return
	}

	if useMQTT {
		zap.S().Infof("Starting with MQTT")
		// Read environment variables for MQTT
		MQTTCertificateName := os.Getenv("MQTT_CERTIFICATE_NAME")
		MQTTBrokerURL := os.Getenv("MQTT_BROKER_URL")
		podName := os.Getenv("MY_POD_NAME")
		SetupMQTT(MQTTCertificateName, MQTTBrokerURL, podName)
	}

	if useKafka {
		zap.S().Infof("Starting with Kafka")
		// Read environment variables for Kafka
		KafkaBoostrapServer := os.Getenv("KAFKA_BOOSTRAP_SERVER")
		kafkaProducerClient, kafkaAdminClient = setupKafka(KafkaBoostrapServer)
	}

	var err error
	lowestSensorTickTime, err = strconv.Atoi(os.Getenv("LOWER_SENSOR_TICK_TIME_MS"))
	if err != nil {
		zap.S().Errorf("Couldn't convert LOWER_SENSOR_TICK_TIME_MS env to int, defaulting to 100")
		lowestSensorTickTime = 100
	}
	upperSensorTickTime, err = strconv.Atoi(os.Getenv("UPPER_SENSOR_TICK_TIME_MS"))
	if err != nil {
		zap.S().Errorf("Couldn't convert UPPER_SENSOR_TICK_TIME_MS env to int, defaulting to 100")
		upperSensorTickTime = 100
	}
	sensorTickTimeSteppingUp, err = strconv.Atoi(os.Getenv("SENSOR_TICK_STEP_MS_UP"))
	if err != nil {
		zap.S().Errorf("Couldn't convert SENSOR_TICK_STEP_MS_UP env to int, defaulting to 10")
		sensorTickTimeSteppingUp = 10
	}
	sensorTickTimeSteppingDown, err = strconv.Atoi(os.Getenv("SENSOR_TICK_STEP_MS_DOWN"))
	if err != nil {
		zap.S().Errorf("Couldn't convert SENSOR_TICK_STEP_MS_UP env to int, defaulting to 10")
		sensorTickTimeSteppingDown = 10
	}

	ipRange := os.Getenv("IP_RANGE")
	zap.S().Infof("Scanning IP range: %s", ipRange)

	transmitterId = os.Getenv("TRANSMITTERID")

	relativeDirectoryPath := os.Getenv("IODD_FILE_PATH")
	// creating ioDeviceMap and downloading initial set of iodd files

	ioDeviceMap = sync.Map{}

	fileInfoSlice, err = initializeIoddData(relativeDirectoryPath)

	if err != nil {
		zap.S().Errorf("initializeIoddData produced the error: %v", err)
	}

	if os.Getenv("DEVICE_FINDER_FREQUENCY_IN_SEC") != "" {
		deviceFinderFrequencyInS, err = strconv.Atoi(os.Getenv("DEVICE_FINDER_FREQUENCY_IN_SEC"))
		if err != nil {
			zap.S().Errorf("Couldn't convert DEVICE_FINDER_FREQUENCY_IN_SEC env to int, defaulting to 20 sec")

		}
	}

	if os.Getenv("DEVICE_FINDER_TIMEOUT_SEC") != "" {
		deviceFinderTimeoutInS, err = strconv.Atoi(os.Getenv("DEVICE_FINDER_TIMEOUT_SEC"))
		if err != nil {
			zap.S().Errorf("Couldn't convert DEVICE_FINDER_TIMEOUT_SEC env to int, defaulting to 1 sec")

		}
	}

	if deviceFinderTimeoutInS > deviceFinderFrequencyInS {
		panic("DEVICE_FINDER_TIMEOUT_SEC should never be greater then DEVICE_FINDER_FREQUENCY_IN_SEC")
	}

	if os.Getenv("MAX_SENSOR_ERROR_COUNT") != "" {
		var maxSC int
		maxSC, err = strconv.Atoi(os.Getenv("MAX_SENSOR_ERROR_COUNT"))
		if err != nil {
			zap.S().Errorf("Couldn't convert MAX_SENSOR_ERROR_COUNT env to int, defaulting to 10")
		}
		maxSensorErrorCount = uint64(maxSC)
	}

	tickerSearchForDevices := time.NewTicker(time.Duration(deviceFinderFrequencyInS) * time.Second)
	defer tickerSearchForDevices.Stop()
	updateIoddIoDeviceMapChan := make(chan IoddFilemapKey)

	go ioddDataDaemon(updateIoddIoDeviceMapChan, relativeDirectoryPath)

	for GetSyncMapLen(&ioDeviceMap) == 0 {
		zap.S().Infof("Initial iodd file download not yet complete, awaiting.")
		time.Sleep(1 * time.Second)
	}

	discoveredDeviceChannel = make(chan DiscoveredDeviceInformation)
	forgetDeviceChannel = make(chan DiscoveredDeviceInformation)

	go continuousSensorDataProcessingV2(updateIoddIoDeviceMapChan)

	time.Sleep(1 * time.Second)
	go continuousDeviceSearch(tickerSearchForDevices, ipRange)
	select {} // block forever
}

func continuousSensorDataProcessingV2(updateIoddIoDeviceMapChan chan IoddFilemapKey) {
	zap.S().Debugf("Starting sensor data processing daemon v2")

	// The boolean value is not used
	var activeDevices map[string]bool
	activeDevices = make(map[string]bool)

	for {
		select {
		case ddi := <-discoveredDeviceChannel:
			{
				if _, ok := activeDevices[ddi.Url]; !ok {
					zap.S().Debugf("New device ! %v", ddi)
					go continuousSensorDataProcessingDeviceDaemon(ddi, updateIoddIoDeviceMapChan)
					activeDevices[ddi.Url] = true
				}
			}
		case fddi := <-forgetDeviceChannel:
			{
				zap.S().Debugf("Got FDDI: %v", fddi)
				delete(activeDevices, fddi.Url)
			}
		}
	}
}

func continuousSensorDataProcessingDeviceDaemon(deviceInfo DiscoveredDeviceInformation, updateIoddIoDeviceMapChan chan IoddFilemapKey) {
	zap.S().Infof("Started new daemon for device %v", deviceInfo)

	var errorcount uint64
	sleepDuration := lowestSensorTickTime
	for {
		start := time.Now()
		var err error
		var portModeMap map[int]int
		hadError := false

		portModeMap, err = GetPortModeMap(deviceInfo)
		if err != nil {
			zap.S().Warnf("[GPMM] Sensor %s at %s couldn't keep up ! (No datapoint will be read). Error: %s", deviceInfo.SerialNumber, deviceInfo.Url, err.Error())
			hadError = true
		}

		if !hadError {
			err = downloadSensorDataMapAndProcess(deviceInfo, updateIoddIoDeviceMapChan, portModeMap)
			if err != nil {
				zap.S().Warnf("[DSDMAP] Sensor %s at %s couldn't keep up ! (No datapoint will be read). Error: %s", deviceInfo.SerialNumber, deviceInfo.Url, err.Error())
				hadError = true
			}
		}

		if hadError {
			errorcount += 1
		} else {
			if errorcount > 0 {
				errorcount -= 1
			}
		}

		if errorcount >= maxSensorErrorCount {
			zap.S().Errorf("Sensor [%v] is unavailable !", deviceInfo)
			forgetDeviceChannel <- deviceInfo
			return
		}

		if hadError {
			sleepDuration += sensorTickTimeSteppingUp
			if sleepDuration > upperSensorTickTime {
				sleepDuration = upperSensorTickTime
			}
			zap.S().Debugf("Sensor [%v] sleep duration increased !", deviceInfo.Url)
		} else {
			sleepDuration -= sensorTickTimeSteppingDown
			if sleepDuration < lowestSensorTickTime {
				sleepDuration = lowestSensorTickTime
			}
		}
		elapsed := time.Since(start)
		sd := time.Duration(sleepDuration) * time.Millisecond
		sleepTime := sd - elapsed
		if sleepTime < time.Duration(0) {
			zap.S().Warnf("[CSDPDD] Can't keep up, sensor %s is to fast ! Elapsed: %s vs Sleep %s", deviceInfo.Url, elapsed.String(), sd.String())
		}
		time.Sleep(sleepTime)
	}
}

func downloadSensorDataMapAndProcess(deviceInfo DiscoveredDeviceInformation, updateIoddIoDeviceMapChan chan IoddFilemapKey, portModeMap map[int]int) (err error) {
	var sensorDataMap map[string]interface{}
	sensorDataMap, err = GetSensorDataMap(deviceInfo)
	if err != nil {
		return err
	}
	go processSensorData(deviceInfo, updateIoddIoDeviceMapChan, portModeMap, sensorDataMap)
	return nil
}

func continuousDeviceSearch(ticker *time.Ticker, ipRange string) {
	zap.S().Debugf("Starting device search daemon")
	err := DiscoverDevices(ipRange)
	zap.S().Debugf("Initial discovery completed")
	for {
		select {
		case <-ticker.C:
			zap.S().Debugf("Starting device scan..")
			err = DiscoverDevices(ipRange)
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
