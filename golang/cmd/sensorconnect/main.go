package main

import (
	"encoding/json"
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

var slowDownMap sync.Map

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

var updateIoddIoDeviceMapChan chan IoddFilemapKey

var subTwentyMs bool

var additionalSleepTimePerActivePort float64

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
	cP = sync.Map{}

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
	lowestSensorTickTime, err = strconv.ParseUint(os.Getenv("LOWER_POLLING_TIME_MS"), 10, 64)
	if err != nil {
		zap.S().Errorf("Couldn't convert LOWER_POLLING_TIME env to int, defaulting to 20. err: %s", err.Error())
		lowestSensorTickTime = 20
	}

	subTwentyMs = os.Getenv("SUB_TWENTY_MS") == "1"

	if lowestSensorTickTime < 20 {
		if subTwentyMs {
			zap.S().Warnf("Going under 20MS is not recommendet with IFM IO-Link Masters")
		} else {
			panic("LOWER_POLLING_TIME under 20 can IFM IO-Link Master failures, set SUB_TWENTY_MS to 1 to continue !")
		}
	}

	upperSensorTickTime, err = strconv.ParseUint(os.Getenv("UPPER_POLLING_TIME_MS"), 10, 64)
	if err != nil {
		zap.S().Errorf("Couldn't convert UPPER_POLLING_TIME env to int, defaulting to 1000. err: %s", err.Error())
		upperSensorTickTime = 1000
	}
	sensorTickTimeSteppingUp, err = strconv.ParseUint(os.Getenv("POLLING_SPEED_STEP_UP_MS"), 10, 64)
	if err != nil {
		zap.S().Errorf("Couldn't convert POLLING_SPEED_STEP_UP_MS env to int, defaulting to 20. err: %s", err.Error())
		sensorTickTimeSteppingUp = 20
	}
	sensorTickTimeSteppingDown, err = strconv.ParseUint(os.Getenv("POLLING_SPEED_STEP_DOWN_MS"), 10, 64)
	if err != nil {
		zap.S().Errorf("Couldn't convert POLLING_SPEED_STEP_UP_MS env to int, defaulting to 1. err: %s", err.Error())
		sensorTickTimeSteppingDown = 1
	}

	sensorStartSpeed, err = strconv.ParseUint(os.Getenv("SENSOR_INITIAL_POLLING_TIME_MS"), 10, 64)
	if err != nil {
		zap.S().Errorf("Couldn't convert SENSOR_INITIAL_POLLING_TIME_MS env to int, defaulting to LOWER_POLLING_TIME. err: %s", err.Error())
		sensorStartSpeed = lowestSensorTickTime
	}

	additionalSleepTimePerActivePort, err = strconv.ParseFloat(os.Getenv("ADDITIONAL_SLEEP_TIME_PER_ACTIVE_PORT_MS"), 64)
	if err != nil {
		zap.S().Errorf("Couldn't convert ADDITIONAL_SLEEP_TIME_PER_ACTIVE_PORT_MS env to float, defaulting to 0. err: %s", err.Error())
		additionalSleepTimePerActivePort = 0
	}

	deviceFinderFrequencyInS, err = strconv.ParseUint(os.Getenv("DEVICE_FINDER_TIME_SEC"), 10, 64)
	if err != nil {
		zap.S().Errorf("Couldn't convert DEVICE_FINDER_TIME_SEC env to int, defaulting to 20 sec. err: %s", err.Error())
		deviceFinderFrequencyInS = 20

	}

	deviceFinderTimeoutInS, err = strconv.ParseUint(os.Getenv("DEVICE_FINDER_TIMEOUT_SEC"), 10, 64)
	if err != nil {
		zap.S().Errorf("Couldn't convert DEVICE_FINDER_TIMEOUT_SEC env to int, defaulting to 1 sec. err: %s", err.Error())
		deviceFinderTimeoutInS = 1

	}

	if deviceFinderTimeoutInS > deviceFinderFrequencyInS {
		panic("DEVICE_FINDER_TIMEOUT_SEC should never be greater then DEVICE_FINDER_TIME_SEC")
	}

	maxSensorErrorCount, err = strconv.ParseUint(os.Getenv("MAX_SENSOR_ERROR_COUNT"), 10, 64)
	if err != nil {
		zap.S().Errorf("Couldn't convert MAX_SENSOR_ERROR_COUNT env to int, defaulting to 50. err: %s", err.Error())
		maxSensorErrorCount = 50
	}

	type SlowDownMapJSONElement struct {
		Serialnumber *string `json:"serialnumber,omitempty"`
		SlowdownMS   float64 `json:"slowdown_ms"`
		URL          *string `json:"url,omitempty"`
		Productcode  *string `json:"productcode,omitempty"`
	}
	type SlowDownMapJSON []SlowDownMapJSONElement

	slowdownMapRaw := os.Getenv("ADDITIONAL_SLOWDOWN_MAP")
	slowDownMap = sync.Map{}
	if slowdownMapRaw != "" {
		var r SlowDownMapJSON
		err = json.Unmarshal([]byte(slowdownMapRaw), &r)
		if err != nil {
			zap.S().Errorf("Failed to convert ADDITIONAL_SLOWDOWN_MAP json to go struct. err: %s", err.Error())
		} else {
			for _, element := range r {
				if element.Serialnumber != nil {
					slowDownMap.Store(fmt.Sprintf("SN_%s", *element.Serialnumber), element.SlowdownMS)
					zap.S().Debugf("Parsed additional sleep time for %s -> %f", *element.Serialnumber, element.SlowdownMS)
				}
				if element.Productcode != nil {
					slowDownMap.Store(fmt.Sprintf("PC_%s", *element.Productcode), element.SlowdownMS)
					zap.S().Debugf("Parsed additional sleep time for %s -> %f", *element.Productcode, element.SlowdownMS)
				}
				if element.URL != nil {
					slowDownMap.Store(fmt.Sprintf("URL_%s", *element.URL), element.SlowdownMS)
					zap.S().Debugf("Parsed additional sleep time for %s -> %f", *element.URL, element.SlowdownMS)
				}
			}
		}
	}

	ipRange := os.Getenv("IP_RANGE")
	zap.S().Infof("Scanning IP range: %s", ipRange)

	transmitterId = os.Getenv("TRANSMITTERID")

	relativeDirectoryPath := os.Getenv("IODD_FILE_PATH")

	ioDeviceMap = sync.Map{}

	fileInfoSlice = make([]os.FileInfo, 0)

	tickerSearchForDevices := time.NewTicker(time.Duration(deviceFinderFrequencyInS) * time.Second)
	defer tickerSearchForDevices.Stop()
	updateIoddIoDeviceMapChan = make(chan IoddFilemapKey)

	go ioddDataDaemon(relativeDirectoryPath)

	discoveredDeviceChannel = make(chan DiscoveredDeviceInformation)
	forgetDeviceChannel = make(chan DiscoveredDeviceInformation)

	go continuousSensorDataProcessingV2()

	time.Sleep(1 * time.Second)
	go continuousDeviceSearch(tickerSearchForDevices, ipRange)
	select {} // block forever
}

//continuousSensorDataProcessingV2 Spawns go routines for new devices, who will download,process and send data from sensors to Kafka/MQTT
func continuousSensorDataProcessingV2() {
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
					go continuousSensorDataProcessingDeviceDaemon(ddi)
					activeDevices[ddi.Url] = true
				}
			}
		case fddi := <-forgetDeviceChannel:
			{
				delete(activeDevices, fddi.Url)
			}
		}
	}
}

//continuousSensorDataProcessingDeviceDaemon Downloads sensordata, processes it and then sends it to Kafka/MQTT.
// Has a backoff mechanism to prevent overloading io-link masters
func continuousSensorDataProcessingDeviceDaemon(deviceInfo DiscoveredDeviceInformation) {
	zap.S().Infof("Started new daemon for device %v", deviceInfo)

	var errorCount uint64
	// Ramp up speed
	sleepDuration := sensorStartSpeed
	var errChan chan error
	errChan = make(chan error, 512)

	for {
		errorCountChange := uint64(0)
		hadError := false
		var err error
		var portModeMap map[int]ConnectedDeviceInfo

		portModeMap, err = GetUsedPortsAndModeCached(deviceInfo)
		if err != nil {
			zap.S().Warnf("[GPMM] Sensor %s at %s couldn't keep up ! (No datapoint will be read). Error: %s", deviceInfo.SerialNumber, deviceInfo.Url, err.Error())
			hadError = true
			errorCount += 1
			errorCountChange += 1
		}

		hasIFMK := true
		for port, info := range portModeMap {
			if !info.Connected {
				continue
			}

			iofm := IoddFilemapKey{
				VendorId: int64(info.VendorId),
				DeviceId: int(info.DeviceId),
			}
			if _, ok := ioDeviceMap.Load(iofm); !ok {
				updateIoddIoDeviceMapChan <- iofm
				hasIFMK = false
				zap.S().Debugf("IoddFileMap for port %d is missing: %v", port, info)
			}
		}

		if !hasIFMK {
			time.Sleep(5 * time.Second)
			continue
		}

		if !hadError {
			go downloadSensorDataMapAndProcess(deviceInfo, portModeMap, errChan)
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
		lowestSleepTime := time.Duration(lowestSensorTickTime) * time.Millisecond
		sleepTime := time.Duration(sleepDuration) * time.Millisecond
		if sleepTime < lowestSleepTime {
			sleepTime = lowestSleepTime
		}

		activePorts := 0
		for _, info := range portModeMap {
			if info.Connected {
				activePorts += 1
			}
		}
		add := time.Duration(float64(activePorts)*additionalSleepTimePerActivePort) * time.Millisecond
		sleepTime += add

		//Lookup slowdown map
		if value, ok := slowDownMap.Load(fmt.Sprintf("SN_%s", deviceInfo.SerialNumber)); ok {
			sleepTime += time.Duration(value.(float64)) * time.Millisecond
		}
		if value, ok := slowDownMap.Load(fmt.Sprintf("PC_%s", deviceInfo.ProductCode)); ok {
			sleepTime += time.Duration(value.(float64)) * time.Millisecond
		}
		if value, ok := slowDownMap.Load(fmt.Sprintf("URL_%s", deviceInfo.Url)); ok {
			sleepTime += time.Duration(value.(float64)) * time.Millisecond
		}
		time.Sleep(sleepTime)
	}
}

//downloadSensorDataMapAndProcess downloads sensor data and processes it
func downloadSensorDataMapAndProcess(deviceInfo DiscoveredDeviceInformation, portModeMap map[int]ConnectedDeviceInfo, errChan chan error) {
	var sensorDataMap map[string]interface{}
	var err error
	sensorDataMap, err = GetSensorDataMap(deviceInfo)
	if err != nil {
		errChan <- err
		return
	}
	go processSensorData(deviceInfo, portModeMap, sensorDataMap)
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
func ioddDataDaemon(relativeDirectoryPath string) {
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
			internal.SetMemcached(cacheKey, nil)
			var err error
			zap.S().Debugf("Addining new device to iodd files and map (%v)", ioddFilemapKey)
			_, err = AddNewDeviceToIoddFilesAndMap(ioddFilemapKey, relativeDirectoryPath, fileInfoSlice)
			zap.S().Debugf("added new ioDevice to map: %v", ioddFilemapKey)
			if err != nil {
				zap.S().Errorf("AddNewDeviceToIoddFilesAndMap produced the error: %v", err)
				continue
			}
		}
	}
}
