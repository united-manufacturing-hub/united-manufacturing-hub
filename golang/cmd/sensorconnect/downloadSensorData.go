package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"io/ioutil"
	"net/http"
	"time"
)

type SensorDataInformation struct {
	Cid        int                    `json:"cid"`
	SensorData map[string]interface{} `json:"data"`
}

// GetSensorDataMap returns a map of one IO-Link-Master with the port number as key and sensor data as value
func GetSensorDataMap(currentDeviceInformation DiscoveredDeviceInformation) (map[string]interface{}, error) {
	var val interface{}
	var found bool
	var modeRequestBody []byte

	cacheKey := fmt.Sprintf("GetSensorDataMap%s:%s:%s", currentDeviceInformation.ProductCode, currentDeviceInformation.SerialNumber, currentDeviceInformation.Url)

	val, found = internal.GetMemcached(cacheKey)
	if found {
		modeRequestBody = val.([]byte)
	} else {
		usedPortsAndModes, err := GetUsedPortsAndModeCached(currentDeviceInformation)
		if err != nil {
			return nil, err
		}
		if len(usedPortsAndModes) == 0 {
			// No devices connected, just return early
			return make(map[string]interface{}), nil
		}
		modeRequestBody, err = createSensorDataRequestBody(usedPortsAndModes)
		if err != nil {
			return nil, err
		}
		internal.SetMemcachedLong(cacheKey, modeRequestBody, 20*time.Second)
	}

	respBody, err := downloadSensorData(currentDeviceInformation.Url, modeRequestBody)
	if err != nil {
		return nil, err
	}
	tmpSensorDataMap, err := unmarshalSensorData(respBody)
	return tmpSensorDataMap, err
}

// unmarshalModeInformation receives the response of the IO-Link-Master regarding its port modes. The function now processes the response and returns a port, portmode map.
func unmarshalSensorData(dataRaw []byte) (map[string]interface{}, error) {
	dataUnmarshaled := SensorDataInformation{}
	if err := json.Unmarshal(dataRaw, &dataUnmarshaled); err != nil {
		return nil, err
	}
	sensorDataMap := make(map[string]interface{}) //key: portNumber, value: portMode
	for key, element := range dataUnmarshaled.SensorData {
		// create map with key e.g. "/iolinkmaster/port[1]/mode" and retuned data genecically in interface{}
		sensorDataMap[key] = element
	}
	return sensorDataMap, nil
}

// createSensorDataRequestBody creates the POST request body for ifm gateways. The body is made to simultaneously request sensordata of the ports 1 - numberOfPorts.
func createSensorDataRequestBody(connectedDeviceInfo map[int]ConnectedDeviceInfo) (payload []byte, err error) {
	// Payload to send to the gateways
	//cid can be any number
	payload = []byte(`{
	"code":"request",
	"cid":25,
	"adr":"/getdatamulti",
	"data":{
		"datatosend":[`)

	for port, info := range connectedDeviceInfo {
		if !info.Connected {
			continue
		}

		var query []byte
		switch info.Mode {
		// DI
		case 1:
			{
				query = []byte(fmt.Sprintf("\"/iolinkmaster/port[%d]/pin2in\",\n", port))
			}
			// DO
		case 2:
			{
				return nil, errors.New("DO is currently not supported")
			}
			// IO-Link
		case 3:
			{
				query = []byte(fmt.Sprintf("\"/iolinkmaster/port[%d]/iolinkdevice/pdin\",\n", port))
			}
		default:
			{
				return nil, errors.New(fmt.Sprintf("Invalid IO-Link port mode: %d for %v", info.Mode, info))
			}
		}
		payload = append(payload, query...)
	}
	// remove last , from payload
	payload = payload[:len(payload)-2]

	//closes datatosend, data and root object
	payload = append(payload, []byte(`
			]
		}
	}`)...)

	return
}

// downloadSensorData sends a POST request to the given url with the given payload. It returns the body and an error in case of problems.
func downloadSensorData(url string, payload []byte) (body []byte, err error) {
	// Create Request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		zap.S().Warnf("Failed to create post request for url: %s", url)
		return
	}
	client := GetHTTPClient(url)
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		zap.S().Debugf("Response status not 200 but instead: %d", resp.StatusCode)
		zap.S().Debugf("Payload was: %v", payload)
		zap.S().Debugf("Url was: %s", url)
		return
	}
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	return
}
