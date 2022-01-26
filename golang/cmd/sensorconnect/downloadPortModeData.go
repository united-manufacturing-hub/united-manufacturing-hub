package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"

	"go.uber.org/zap"
)

type ModeInformation struct {
	Cid      int                    `json:"cid"`
	ModeData map[string]interface{} `json:"data"`
}

// GetPortModeMap returns a map of one IO-Link-Master with the port number as key and the port mode as value
func GetPortModeMap(currentDeviceInformation DiscoveredDeviceInformation) (map[int]int, error) {

	numberOfPorts := findNumberOfPorts(currentDeviceInformation.ProductCode)
	modeRequestBody := createModeRequestBody(numberOfPorts)
	respBody, err := downloadModeStatus(currentDeviceInformation.Url, modeRequestBody)
	if err != nil {
		zap.S().Errorf("download of response from url %s failed.", currentDeviceInformation.Url)
		return nil, err
	}
	modeMap, err := unmarshalModeInformation(respBody)
	return modeMap, err
}

// unmarshalModeInformation receives the response of the IO-Link-Master regarding its port modes. The function now processes the response and returns a port, portmode map.
func unmarshalModeInformation(dataRaw []byte) (map[int]int, error) {
	dataUnmarshaled := ModeInformation{}
	if err := json.Unmarshal(dataRaw, &dataUnmarshaled); err != nil {
		return nil, err
	}
	modeMap := make(map[int]int) //key: portNumber, value: portMode
	for key, element := range dataUnmarshaled.ModeData {
		// extract port number from key e.g. "/iolinkmaster/port[1]/mode" --> 1
		portNumber, err := extractIntFromString(key)
		if err != nil {
			continue
		}
		elementMap := element.(map[string]interface{})
		portMode := int(elementMap["data"].(float64))
		modeMap[portNumber] = portMode
	}
	return modeMap, nil
}

// findNumberOfPorts returns the number of ports a given Io-Link-Master has regarding to its Product Code
func findNumberOfPorts(ProductCode string) int {
	eightPortDevices := []string{"AL1342", "AL1352", "AL1353"}
	if contains(eightPortDevices, ProductCode) {
		return 8
	} else {
		return 4
	}
}

// createModeRequestBody creates the POST request body for ifm gateways. The body is made to simultaneously request the ports 1 - numberOfPorts.
func createModeRequestBody(numberOfPorts int) []byte {
	// Payload to send to the gateways
	var payload = []byte(`{
	"code":"request",
	"cid":24,
	"adr":"/getdatamulti",
	"data":{
		"datatosend":[
			"/iolinkmaster/port[1]/mode"`)
	for i := 2; i <= numberOfPorts; i++ {
		currentPort := []byte(strconv.Itoa(i))
		payload = append(payload, []byte(`,
			"/iolinkmaster/port[`)...)
		payload = append(payload, currentPort...)
		payload = append(payload, []byte(`]/mode"`)...)
	}
	payload = append(payload, []byte(`
		]
	}
}`)...)
	return payload
}

// downloadModeStatus sends a POST request to the given url with the given payload. It returns the body and an error in case of problems.
func downloadModeStatus(url string, payload []byte) (body []byte, err error) {
	// Create Request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		zap.S().Warnf("Failed to create post request for url: %s", url)
		return
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		zap.S().Debugf("Client at %s did not respond.", url)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		zap.S().Debugf("Responsstatus not 200 but instead: %d", resp.StatusCode)
		return
	}
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		zap.S().Errorf("ioutil.Readall(resp.Body)  failed: %s", err)
		return
	}
	return
}

// extractIntFromString returns exactly one int from a given string. If no int or more then one int is inside of the string, an error is thrown.
func extractIntFromString(input string) (int, error) {
	re := regexp.MustCompile("[0-9]+")
	outputSlice := re.FindAllString(input, -1)
	if len(outputSlice) != 1 {
		err := errors.New("extractinfFromStringFailed")
		zap.S().Errorf("not exactly one integer found %d, %s", len(outputSlice), err)
		return -1, err
	}
	outputNumber, err := strconv.Atoi(outputSlice[0])
	if err != nil {
		err := errors.New("extractinfFromStringFailed")
		zap.S().Errorf("not exactly one integer found %d, %s", len(outputSlice), err)
		return -1, err
	}
	return outputNumber, nil
}
