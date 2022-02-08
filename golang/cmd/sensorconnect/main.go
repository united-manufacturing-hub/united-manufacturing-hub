package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

var mqttClient MQTT.Client

//var discoveredDeviceInformation []DiscoveredDeviceInformation
// key is url
var discoveredDeviceInformation sync.Map

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

var lowestSensorTickTime int

var upperSensorTickTime int

var sensorTickTimeSteppingUp int
var actualSensorTickSpeed int

var sensorTickTimeSteppingDown int

func main() {
	var logger *zap.Logger
	if os.Getenv("DEBUG") == "1" {
		logger, _ = zap.NewDevelopment()
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
		kafkaProducerClient, kafkaAdminClient, _ = setupKafka(KafkaBoostrapServer)
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
	discoveredDeviceInformation = sync.Map{}

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

	tickerSearchForDevices := time.NewTicker(time.Duration(deviceFinderFrequencyInS) * time.Second)
	defer tickerSearchForDevices.Stop()
	updateIoddIoDeviceMapChan := make(chan IoddFilemapKey)

	go continuousDeviceSearch(tickerSearchForDevices, ipRange)

	for GetSyncMapLen(&discoveredDeviceInformation) == 0 {
		zap.S().Infof("No devices discovered yet.")
		time.Sleep(1 * time.Second)
	}

	go ioddDataDaemon(updateIoddIoDeviceMapChan, relativeDirectoryPath)

	for GetSyncMapLen(&ioDeviceMap) == 0 {
		zap.S().Infof("Initial iodd file download not yet complete, awaiting.")
		time.Sleep(1 * time.Second)
	}
	actualSensorTickSpeed = lowestSensorTickTime
	zap.S().Infof("Requesting data every %d ms", actualSensorTickSpeed)
	tickerProcessSensorData := time.NewTicker(time.Duration(actualSensorTickSpeed) * time.Millisecond)
	defer tickerProcessSensorData.Stop()
	go continuousSensorDataProcessing(tickerProcessSensorData, updateIoddIoDeviceMapChan)

	select {} // block forever
}

func continuousSensorDataProcessing(ticker *time.Ticker, updateIoddIoDeviceMapChan chan IoddFilemapKey) {
	zap.S().Debugf("Starting sensor data processing daemon")

	cyclesWithoutError := uint64(0)
	for {

		select {
		case <-ticker.C:
			var err error

			hadError := false
			discoveredDeviceInformation.Range(func(key, rawCurrentDeviceInformation interface{}) bool {
				deviceInfo := rawCurrentDeviceInformation.(DiscoveredDeviceInformation)
				var portModeMap map[int]int

				portModeMap, err = GetPortModeMap(deviceInfo)
				if err != nil {
					zap.S().Warnf("Sensor %s at %s couldn't keep up ! (No datapoint will be read). Error: %s", deviceInfo.SerialNumber, deviceInfo.Url, err.Error())
					hadError = true
					return true
				}

				go downloadSensorDataMapAndProcess(deviceInfo, updateIoddIoDeviceMapChan, portModeMap)
				return true
			})

			// Don't change read speed, if not configured !
			if upperSensorTickTime == lowestSensorTickTime {
				continue
			}

			if hadError {
				cyclesWithoutError = 0
				actualSensorTickSpeed += sensorTickTimeSteppingUp
				if actualSensorTickSpeed > upperSensorTickTime {
					actualSensorTickSpeed = upperSensorTickTime
				}
				zap.S().Debugf("Increased tick time to %d", actualSensorTickSpeed)
				ticker.Reset(time.Duration(actualSensorTickSpeed) * time.Millisecond)
			} else {
				cyclesWithoutError += 1
				if cyclesWithoutError%10 == 0 {
					if actualSensorTickSpeed > lowestSensorTickTime {
						actualSensorTickSpeed -= sensorTickTimeSteppingDown
						if actualSensorTickSpeed < lowestSensorTickTime {
							actualSensorTickSpeed = lowestSensorTickTime
						}
						zap.S().Debugf("Reduced tick time to %d", actualSensorTickSpeed)
						ticker.Reset(time.Duration(actualSensorTickSpeed) * time.Millisecond)
					}
				}
			}
		}
	}
}

func downloadSensorDataMapAndProcess(deviceInfo DiscoveredDeviceInformation, updateIoddIoDeviceMapChan chan IoddFilemapKey, portModeMap map[int]int) {
	var sensorDataMap map[string]interface{}
	var err error
	sensorDataMap, err = GetSensorDataMap(deviceInfo)
	if err != nil {
		return
	}
	processSensorData(deviceInfo, updateIoddIoDeviceMapChan, portModeMap, sensorDataMap)
}

func continuousDeviceSearch(ticker *time.Ticker, ipRange string) {
	zap.S().Debugf("Starting device search daemon")
	for {
		select {
		case <-ticker.C:
			var err error
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
