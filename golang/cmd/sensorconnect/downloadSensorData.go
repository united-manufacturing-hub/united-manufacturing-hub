package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"time"
)

type SensorDataInformation struct {
	Cid        int                    `json:"cid"`
	SensorData map[string]interface{} `json:"data"`
}

// GetSensorDataMap returns a map of one IO-Link-Master with the port number as key and the port mode as value
func GetSensorDataMap(currentDeviceInformation DiscoveredDeviceInformation) (map[string]interface{}, error) {
	var val interface{}
	var found bool
	var modeRequestBody []byte

	cacheKey := fmt.Sprintf("GetSensorDataMap%s", currentDeviceInformation.ProductCode)

	val, found = internal.GetMemcached(cacheKey)
	if found {
		modeRequestBody = val.([]byte)
	} else {
		numberOfPorts := findNumberOfPorts(currentDeviceInformation.ProductCode)
		modeRequestBody = createSensorDataRequestBody(numberOfPorts)
		internal.SetMemcachedLong(cacheKey, modeRequestBody, -1)
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
func createSensorDataRequestBody(numberOfPorts int) []byte {
	// Payload to send to the gateways
	var payload = []byte(`{
	"code":"request",
	"cid":25,
	"adr":"/getdatamulti",
	"data":{
		"datatosend":[
			"/iolinkmaster/port[1]/iolinkdevice/deviceid",
			"/iolinkmaster/port[1]/iolinkdevice/pdin",
			"/iolinkmaster/port[1]/iolinkdevice/vendorid",
			"/iolinkmaster/port[1]/pin2in"`)
	// repeat for other ports
	for i := 2; i <= numberOfPorts; i++ {
		currentPort := []byte(strconv.Itoa(i))
		payload = append(payload, []byte(`,
			"/iolinkmaster/port[`)...)
		payload = append(payload, currentPort...)
		payload = append(payload, []byte(`]/iolinkdevice/deviceid",
			"/iolinkmaster/port[`)...)
		payload = append(payload, currentPort...)
		payload = append(payload, []byte(`]/iolinkdevice/pdin",
			"/iolinkmaster/port[`)...)
		payload = append(payload, currentPort...)
		payload = append(payload, []byte(`]/iolinkdevice/vendorid",
			"/iolinkmaster/port[`)...)
		payload = append(payload, currentPort...)
		payload = append(payload, []byte(`]/pin2in"`)...)
	}
	payload = append(payload, []byte(`
		]
	}
}`)...)
	return payload
}

var clientPool map[string]http.Client

// downloadSensorData sends a POST request to the given url with the given payload. It returns the body and an error in case of problems.
func downloadSensorData(url string, payload []byte) (body []byte, err error) {
	// Create Request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		zap.S().Warnf("Failed to create post request for url: %s", url)
		return
	}

	var client http.Client
	var ok bool
	if client, ok = clientPool[url]; !ok {
		client = http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				ForceAttemptHTTP2:     false,
				MaxIdleConns:          100,
				MaxConnsPerHost:       0,
				IdleConnTimeout:       10 * time.Second,
				TLSHandshakeTimeout:   1 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		}
		if clientPool == nil {
			clientPool = make(map[string]http.Client)
		}
		clientPool[url] = client
	}

	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		zap.S().Debugf("Response status not 200 but instead: %d", resp.StatusCode)
		return
	}
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	return
}
