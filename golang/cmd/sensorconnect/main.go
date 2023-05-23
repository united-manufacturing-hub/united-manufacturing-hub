// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/heptiolabs/healthcheck"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"io/fs"
	"net/http"

	"github.com/united-manufacturing-hub/umh-utils/env"
	"sync"
	"time"
)

var mqttClient MQTT.Client

var discoveredDeviceChannel chan DiscoveredDeviceInformation
var forgetDeviceChannel chan DiscoveredDeviceInformation

// var ioDeviceMap map[IoddFilemapKey]IoDevice
var ioDeviceMap sync.Map

var fileInfoSlice []fs.DirEntry

var slowDownMap sync.Map

var kafkaProducerClient *kafka.Producer
var transmitterId string

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
	// Initialize zap logging
	logLevel, _ := env.GetAsString("LOGGING_LEVEL", false, "PRODUCTION") //nolint:errcheck
	log := logger.New(logLevel)
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)

	internal.Initfgtrace()

	// Prometheus
	zap.S().Debugf("Setting up healthcheck")

	health := healthcheck.NewHandler()
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(1000000))
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe("0.0.0.0:8086", health)
		if err != nil {
			zap.S().Errorf("Error starting healthcheck: %s", err)
		}
	}()

	internal.InitCacheWithoutRedis()
	cP = sync.Map{}

	var err error
	isTest, err := env.GetAsBool("TEST", false, false)
	if err != nil {
		zap.S().Error(err)
	}

	useKafka, err = env.GetAsBool("USE_KAFKA", false, true)
	if err != nil {
		zap.S().Error(err)
	}
	useMQTT, err = env.GetAsBool("USE_MQTT", false, false)
	if err != nil {
		zap.S().Error(err)
	}

	if !useKafka && !useMQTT {
		zap.S().Errorf("Neither kafka nor MQTT output enabled, exiting !")
		return
	}

	if useMQTT {
		zap.S().Infof("Starting with MQTT")
		// Read environment variables for MQTT
		var MQTTCertificateName, MQTTBrokerURL, podName, mqttPassword string
		MQTTCertificateName, err = env.GetAsString("MQTT_CERTIFICATE_NAME", true, "")
		if err != nil {
			zap.S().Fatal(err)
		}
		MQTTBrokerURL, err = env.GetAsString("MQTT_BROKER_URL", true, "")
		if err != nil {
			zap.S().Fatal(err)
		}
		podName, err = env.GetAsString("POD_NAME", true, "")
		if err != nil {
			zap.S().Fatal(err)
		}
		mqttPassword, err = env.GetAsString("MQTT_PASSWORD", true, "")
		if err != nil {
			zap.S().Fatal(err)
		}
		SetupMQTT(MQTTCertificateName, MQTTBrokerURL, podName, mqttPassword)
	}

	if useKafka {
		zap.S().Infof("Starting with Kafka")
		// Read environment variables for Kafka
		var KafkaBoostrapServer string
		KafkaBoostrapServer, err = env.GetAsString("KAFKA_BOOTSTRAP_SERVER", true, "")
		if err != nil {
			zap.S().Fatal(err)
		}
		kafkaProducerClient, _ = setupKafka(KafkaBoostrapServer)
	}

	lowestSensorTickTime, err = env.GetAsUint64("LOWER_POLLING_TIME_MS", false, 20)
	if err != nil {
		zap.S().Error(err)
	}

	subTwentyMs, err = env.GetAsBool("SUB_TWENTY_MS", false, false)
	if err != nil {
		zap.S().Error(err)
	}

	if lowestSensorTickTime < 20 {
		if subTwentyMs {
			zap.S().Warnf("Going under 20MS is not recommended with IFM IO-Link Masters")
		} else {
			zap.S().Fatalf("Going under 20MS is not recommended with IFM IO-Link Masters, set SUB_TWENTY_MS to 1 to override")
		}
	}

	upperSensorTickTime, err = env.GetAsUint64("UPPER_POLLING_TIME_MS", false, 1000)
	if err != nil {
		zap.S().Error(err)
	}
	sensorTickTimeSteppingUp, err = env.GetAsUint64("POLLING_SPEED_STEP_UP_MS", false, 20)
	if err != nil {
		zap.S().Error(err)
	}
	sensorTickTimeSteppingDown, err = env.GetAsUint64("POLLING_SPEED_STEP_DOWN_MS", false, 1)
	if err != nil {
		zap.S().Error(err)
	}

	sensorStartSpeed, err = env.GetAsUint64("SENSOR_INITIAL_POLLING_TIME_MS", false, lowestSensorTickTime)
	if err != nil {
		zap.S().Error(err)
	}

	additionalSleepTimePerActivePort, err = env.GetAsFloat64("ADDITIONAL_SLEEP_TIME_PER_ACTIVE_PORT_MS", false, 0)
	if err != nil {
		zap.S().Error(err)
	}

	deviceFinderFrequencyInS, err = env.GetAsUint64("DEVICE_FINDER_TIME_SEC", false, 20)
	if err != nil {
		zap.S().Error(err)
	}

	deviceFinderTimeoutInS, err = env.GetAsUint64("DEVICE_FINDER_TIMEOUT_SEC", false, 1)
	if err != nil {
		zap.S().Error(err)
	}

	if deviceFinderTimeoutInS > deviceFinderFrequencyInS {
		zap.S().Fatal("DEVICE_FINDER_TIMEOUT_SEC should never be greater then DEVICE_FINDER_TIME_SEC")
	}

	maxSensorErrorCount, err = env.GetAsUint64("MAX_SENSOR_ERROR_COUNT", false, 50)
	if err != nil {
		zap.S().Error(err)
	}

	type SlowDownMapJSONElement struct {
		Serialnumber *string `json:"serialnumber,omitempty"`
		URL          *string `json:"url,omitempty"`
		Productcode  *string `json:"productcode,omitempty"`
		SlowdownMS   float64 `json:"slowdown_ms"`
	}
	type SlowDownMapJSON []SlowDownMapJSONElement

	slowDownMap = sync.Map{}
	var slowDownMapJSON SlowDownMapJSON
	err = env.GetAsType("ADDITIONAL_SLOWDOWN_MAP", &slowDownMapJSON, true, SlowDownMapJSON{})
	if err != nil {
		zap.S().Fatal(err)
	}
	for _, element := range slowDownMapJSON {
		if element.Serialnumber != nil {
			slowDownMap.Store(fmt.Sprintf("SN_%s", *element.Serialnumber), element.SlowdownMS)
			zap.S().Debugf(
				"Parsed additional sleep time for %s -> %f",
				*element.Serialnumber,
				element.SlowdownMS)
		}
		if element.Productcode != nil {
			slowDownMap.Store(fmt.Sprintf("PC_%s", *element.Productcode), element.SlowdownMS)
			zap.S().Debugf(
				"Parsed additional sleep time for %s -> %f",
				*element.Productcode,
				element.SlowdownMS)
		}
		if element.URL != nil {
			slowDownMap.Store(fmt.Sprintf("URL_%s", *element.URL), element.SlowdownMS)
			zap.S().Debugf("Parsed additional sleep time for %s -> %f", *element.URL, element.SlowdownMS)
		}
	}

	ipRange, err := env.GetAsString("IP_RANGE", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	zap.S().Infof("Scanning IP range: %s", ipRange)

	transmitterId, err = env.GetAsString("TRANSMITTERID", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}

	relativeDirectoryPath, err := env.GetAsString("IODD_FILE_PATH", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}

	ioDeviceMap = sync.Map{}

	fileInfoSlice = make([]fs.DirEntry, 0)

	tickerSearchForDevices := time.NewTicker(time.Duration(deviceFinderFrequencyInS) * time.Second)
	defer tickerSearchForDevices.Stop()
	updateIoddIoDeviceMapChan = make(chan IoddFilemapKey)

	go ioddDataDaemon(relativeDirectoryPath, isTest)

	discoveredDeviceChannel = make(chan DiscoveredDeviceInformation)
	forgetDeviceChannel = make(chan DiscoveredDeviceInformation)

	go continuousSensorDataProcessingV2()

	time.Sleep(1 * time.Second)
	go continuousDeviceSearch(tickerSearchForDevices, ipRange)
	select {} // block forever
}

// continuousSensorDataProcessingV2 Spawns go routines for new devices, who will download,process and send data from sensors to Kafka/MQTT
func continuousSensorDataProcessingV2() {
	zap.S().Debugf("Starting sensor data processing daemon v2")

	// The boolean value is not used
	activeDevices := make(map[string]bool)

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

// continuousSensorDataProcessingDeviceDaemon Downloads sensordata, processes it and then sends it to Kafka/MQTT.
// Has a backoff mechanism to prevent overloading io-link masters
func continuousSensorDataProcessingDeviceDaemon(deviceInfo DiscoveredDeviceInformation) {
	zap.S().Infof("Started new daemon for device %v", deviceInfo)

	var errorCount uint64
	// Ramp up speed
	sleepDuration := sensorStartSpeed
	errChan := make(chan error, 512)

	for {
		errorCountChange := uint64(0)
		hadError := false
		var err error
		var portModeMap map[int]ConnectedDeviceInfo

		portModeMap, err = GetUsedPortsAndModeCached(deviceInfo)
		if err != nil {
			zap.S().Warnf(
				"[GPMM] Sensor %s at %s couldn't keep up ! (No datapoint will be read). Error: %s",
				deviceInfo.SerialNumber,
				deviceInfo.Url,
				err.Error())
			hadError = true
			errorCount += 1
			errorCountChange += 1
		}

		hasIFMK := true
		for port, info := range portModeMap {
			if !info.Connected {
				continue
			}

			if info.VendorId == 0 && info.DeviceId == 0 {
				// Digital sensor
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
					zap.S().Warnf(
						"[DSDMAP] Sensor %s at %s couldn't keep up ! (No datapoint will be read). Error: %s",
						deviceInfo.SerialNumber,
						deviceInfo.Url,
						ex.Error())
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

		// Lookup slowdown map
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

// downloadSensorDataMapAndProcess downloads sensor data and processes it
func downloadSensorDataMapAndProcess(
	deviceInfo DiscoveredDeviceInformation,
	portModeMap map[int]ConnectedDeviceInfo,
	errChan chan error) {
	var sensorDataMap map[string]interface{}
	var err error
	sensorDataMap, err = GetSensorDataMap(deviceInfo)
	if err != nil {
		errChan <- err
		return
	}
	go processSensorData(deviceInfo, portModeMap, sensorDataMap)
}

// continuousDeviceSearch Searches for devices everytime ticker is triggered
func continuousDeviceSearch(ticker *time.Ticker, ipRange string) {
	zap.S().Debugf("Starting device search daemon")
	err := DiscoverDevices(ipRange)
	if err != nil {
		zap.S().Errorf("Error while searching for devices: %s", err.Error())
	}
	zap.S().Debugf("Initial discovery completed")
	for {
		<-ticker.C
		zap.S().Debugf("Starting device scan..")
		err = DiscoverDevices(ipRange)
		if err != nil {
			zap.S().Errorf("DiscoverDevices produced the error: %v", err)
			continue
		}
	}

}

// ioddDataDaemon Starts a demon to download IODD files
func ioddDataDaemon(relativeDirectoryPath string, isTest bool) {
	zap.S().Debugf("Starting iodd data daemon")

	for {
		ioddFilemapKey := <-updateIoddIoDeviceMapChan
		cacheKey := fmt.Sprintf("ioddDataDaemon%d:%d", ioddFilemapKey.DeviceId, ioddFilemapKey.VendorId)
		// Prevents download attempt spam
		_, found := internal.GetMemcached(cacheKey)
		if found {
			continue
		}
		internal.SetMemcached(cacheKey, nil)
		var err error
		zap.S().Debugf("Addining new device to iodd files and map (%v)", ioddFilemapKey)
		_, err = AddNewDeviceToIoddFilesAndMap(ioddFilemapKey, relativeDirectoryPath, fileInfoSlice, isTest)
		zap.S().Debugf("added new ioDevice to map: %v", ioddFilemapKey)
		if err != nil {
			zap.S().Errorf("AddNewDeviceToIoddFilesAndMap produced the error: %v", err)
			continue
		}

	}
}
