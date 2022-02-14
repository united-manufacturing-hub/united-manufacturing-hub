package main

import (
	"fmt"
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

var deviceFinderFrequencyInS uint64
var deviceFinderTimeoutInS uint64

var lowestSensorTickTime uint64
var upperSensorTickTime uint64

var sensorTickTimeSteppingUp uint64
var sensorTickTimeSteppingDown uint64
var sensorStartSpeed uint64

var maxSensorErrorCount uint64

var subTwentyMs bool

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
	lowestSensorTickTime, err = strconv.ParseUint(os.Getenv("LOWER_SENSOR_TICK_TIME_MS"), 10, 64)
	if err != nil {
		zap.S().Errorf("Couldn't convert LOWER_SENSOR_TICK_TIME_MS env to int, defaulting to 20")
		lowestSensorTickTime = 20
	}

	subTwentyMs = os.Getenv("SUB_TWENTY_MS") == "1"

	if lowestSensorTickTime < 20 {
		if subTwentyMs {
			zap.S().Warnf("Going under 20MS is not recommendet with IFM IO-Link Masters")
		} else {
			panic("LOWER_SENSOR_TICK_TIME_MS under 20 can IFM IO-Link Master failures, set SUB_TWENTY_MS to 1 to continue !")
		}
	}

	upperSensorTickTime, err = strconv.ParseUint(os.Getenv("UPPER_SENSOR_TICK_TIME_MS"), 10, 64)
	if err != nil {
		zap.S().Errorf("Couldn't convert UPPER_SENSOR_TICK_TIME_MS env to int, defaulting to 1000")
		upperSensorTickTime = 1000
	}
	sensorTickTimeSteppingUp, err = strconv.ParseUint(os.Getenv("SENSOR_TICK_STEP_MS_UP"), 10, 64)
	if err != nil {
		zap.S().Errorf("Couldn't convert SENSOR_TICK_STEP_MS_UP env to int, defaulting to 20")
		sensorTickTimeSteppingUp = 20
	}
	sensorTickTimeSteppingDown, err = strconv.ParseUint(os.Getenv("SENSOR_TICK_STEP_MS_DOWN"), 10, 64)
	if err != nil {
		zap.S().Errorf("Couldn't convert SENSOR_TICK_STEP_MS_UP env to int, defaulting to 1")
		sensorTickTimeSteppingDown = 1
	}

	sensorStartSpeed, err = strconv.ParseUint(os.Getenv("SENSOR_START_SPEED"), 10, 64)
	if err != nil {
		zap.S().Errorf("Couldn't convert SENSOR_START_SPEED env to int, defaulting to LOWER_SENSOR_TICK_TIME_MS")
		sensorStartSpeed = lowestSensorTickTime
	}

	ipRange := os.Getenv("IP_RANGE")
	zap.S().Infof("Scanning IP range: %s", ipRange)

	transmitterId = os.Getenv("TRANSMITTERID")

	relativeDirectoryPath := os.Getenv("IODD_FILE_PATH")
	// creating ioDeviceMap and downloading initial set of iodd files

	ioDeviceMap = sync.Map{}

	//fileInfoSlice, err = initializeIoddData(relativeDirectoryPath)
	fileInfoSlice = make([]os.FileInfo, 0)

	if err != nil {
		zap.S().Errorf("initializeIoddData produced the error: %v", err)
	}

	deviceFinderFrequencyInS, err = strconv.ParseUint(os.Getenv("DEVICE_FINDER_FREQUENCY_IN_SEC"), 10, 64)
	if err != nil {
		zap.S().Errorf("Couldn't convert DEVICE_FINDER_FREQUENCY_IN_SEC env to int, defaulting to 20 sec")
		deviceFinderFrequencyInS = 20

	}

	deviceFinderTimeoutInS, err = strconv.ParseUint(os.Getenv("DEVICE_FINDER_TIMEOUT_SEC"), 10, 64)
	if err != nil {
		zap.S().Errorf("Couldn't convert DEVICE_FINDER_TIMEOUT_SEC env to int, defaulting to 1 sec")
		deviceFinderTimeoutInS = 1

	}

	if deviceFinderTimeoutInS > deviceFinderFrequencyInS {
		panic("DEVICE_FINDER_TIMEOUT_SEC should never be greater then DEVICE_FINDER_FREQUENCY_IN_SEC")
	}

	maxSensorErrorCount, err = strconv.ParseUint(os.Getenv("MAX_SENSOR_ERROR_COUNT"), 10, 64)
	if err != nil {
		zap.S().Errorf("Couldn't convert MAX_SENSOR_ERROR_COUNT env to int, defaulting to 50")
		maxSensorErrorCount = 50
	}

	tickerSearchForDevices := time.NewTicker(time.Duration(deviceFinderFrequencyInS) * time.Second)
	defer tickerSearchForDevices.Stop()
	updateIoddIoDeviceMapChan := make(chan IoddFilemapKey)

	go ioddDataDaemon(updateIoddIoDeviceMapChan, relativeDirectoryPath)

	discoveredDeviceChannel = make(chan DiscoveredDeviceInformation)
	forgetDeviceChannel = make(chan DiscoveredDeviceInformation)

	go continuousSensorDataProcessingV2(updateIoddIoDeviceMapChan)

	time.Sleep(1 * time.Second)
	go continuousDeviceSearch(tickerSearchForDevices, ipRange)
	select {} // block forever
}

//continuousSensorDataProcessingV2 Spawns go routines for new devices, who will download,process and send data from sensors to Kafka/MQTT
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

//continuousSensorDataProcessingDeviceDaemon Downloads sensordata, processes it and then sends it to Kafka/MQTT.
// Has a backoff mechanism to prevent overloading io-link masters
func continuousSensorDataProcessingDeviceDaemon(deviceInfo DiscoveredDeviceInformation, updateIoddIoDeviceMapChan chan IoddFilemapKey) {
	zap.S().Infof("Started new daemon for device %v", deviceInfo)

	var errorCount uint64
	// Ramp up speed
	sleepDuration := lowestSensorTickTime
	var errChan chan error
	errChan = make(chan error, 512)

	for {
		errorCountChange := uint64(0)
		hadError := false
		start := time.Now()
		var err error
		var portModeMap map[int]int

		portModeMap, err = GetPortModeMap(deviceInfo)
		if err != nil {
			zap.S().Warnf("[GPMM] Sensor %s at %s couldn't keep up ! (No datapoint will be read). Error: %s", deviceInfo.SerialNumber, deviceInfo.Url, err.Error())
			hadError = true
			errorCount += 1
			errorCountChange += 1
		}
		if !hadError {
			go downloadSensorDataMapAndProcess(deviceInfo, updateIoddIoDeviceMapChan, portModeMap, errChan)
		}

		var hasData = true
		for {
			select {
			case ex := <-errChan:
				{
					errorCount += 1
					errorCountChange += 1
					zap.S().Warnf("[DSDMAP] Sensor %s at %s couldn't keep up ! (No datapoint will be read). Error: %s", deviceInfo.SerialNumber, deviceInfo.Url, ex.Error())
					hadError = true
				}
			default:
				hasData = false
			}
			if !hasData {
				break
			}
		}

		if errorCount >= maxSensorErrorCount {
			zap.S().Errorf("Sensor [%v] is unavailable !", deviceInfo)
			forgetDeviceChannel <- deviceInfo
			return
		}

		if hadError {
			sleepDuration += sensorTickTimeSteppingUp * errorCountChange
			if sleepDuration > upperSensorTickTime {
				sleepDuration = upperSensorTickTime
			}
		} else {
			sleepDuration -= sensorTickTimeSteppingDown
			if sleepDuration < lowestSensorTickTime {
				sleepDuration = lowestSensorTickTime
			}
		}
		elapsed := time.Since(start)
		sd := time.Duration(sleepDuration) * time.Millisecond
		sleepTime := sd - elapsed

		if sleepTime < time.Duration(lowestSensorTickTime)*time.Millisecond {
			sleepTime = time.Duration(lowestSensorTickTime) * time.Millisecond
		}

		zap.S().Debugf("Sleep time: %s for sensor %s", sleepTime.String(), deviceInfo.Url)
		time.Sleep(sleepTime)
	}
}

//downloadSensorDataMapAndProcess downloads sensor data and processes it
func downloadSensorDataMapAndProcess(deviceInfo DiscoveredDeviceInformation, updateIoddIoDeviceMapChan chan IoddFilemapKey, portModeMap map[int]int, errChan chan error) {
	var sensorDataMap map[string]interface{}
	var err error
	sensorDataMap, err = GetSensorDataMap(deviceInfo)
	if err != nil {
		errChan <- err
		return
	}
	go processSensorData(deviceInfo, updateIoddIoDeviceMapChan, portModeMap, sensorDataMap)
}

//continuousDeviceSearch Searches for devices everytime ticker is triggered
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

//ioddDataDaemon Starts a demon to download IODD files
func ioddDataDaemon(updateIoddIoDeviceMapChan chan IoddFilemapKey, relativeDirectoryPath string) {
	zap.S().Debugf("Starting iodd data daemon")

	for {
		select {
		case ioddFilemapKey := <-updateIoddIoDeviceMapChan:

			cacheKey := fmt.Sprintf("ioddDataDaemon%d:%d", ioddFilemapKey.DeviceId, ioddFilemapKey.VendorId)

			// Prevents download attempt spam
			_, found := internal.GetMemcached(cacheKey)
			if found {
				continue
			}
			var err error
			_, err = AddNewDeviceToIoddFilesAndMap(ioddFilemapKey, relativeDirectoryPath, fileInfoSlice)
			zap.S().Debugf("added new ioDevice to map: %v", ioddFilemapKey)
			if err != nil {
				zap.S().Errorf("AddNewDeviceToIoddFilesAndMap produced the error: %v", err)
				continue
			}
			internal.SetMemcached(cacheKey, nil)
		}
	}
}

//initializeIoddData Downloads common IODD files (this is mostly used for debugging)
func initializeIoddData(relativeDirectoryPath string) (fileInfo []os.FileInfo, err error) {

	//1028 is https://ioddfinder.io-link.com/productvariants/search/20582
	var ifmIoddFilemapKey IoddFilemapKey
	ifmIoddFilemapKey.DeviceId = 1028
	ifmIoddFilemapKey.VendorId = 310
	// 278531 is https://ioddfinder.io-link.com/productvariants/search/1804
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

//GetSyncMapLen retrieves length of a sync.Map
func GetSyncMapLen(p *sync.Map) int {
	mapLen := 0
	p.Range(func(key, value interface{}) bool {
		mapLen += 1
		return true
	})

	return mapLen
}
