package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"

	"go.uber.org/zap"
)

type ModeInformation struct {
	Cid      int                    `json:"cid"`
	ModeData map[string]interface{} `json:"data"`
}

func unmarshalModeInformation(dataRaw []byte) []int {
	dataUnmarshaled := ModeInformation{}
	if err := json.Unmarshal(dataRaw, &dataUnmarshaled); err != nil {
		panic(err)
	}

	var modeSlice []int
	for _, element := range dataUnmarshaled.ModeData {
		//portNumber
		elementMap := element.(map[string]interface{})
		fmt.Println(reflect.TypeOf(int(elementMap["data"])))
		//modeSlice[portNumber] = elementMap["data"].(int)
	}
	return modeSlice
}

func GetModeStatusStruct(currentDeviceInformation DiscoveredDeviceInformation) []int {

	numberOfPorts := findNumberOfPorts(currentDeviceInformation.ProductCode)
	modeRequestBody := createModeRequestBody(numberOfPorts)
	respBody, err := downloadModeStatus(currentDeviceInformation.Url, modeRequestBody)
	fmt.Println(respBody)
	if err != nil {
		zap.S().Errorf("download of response from url %s failed.", currentDeviceInformation.Url)
	}
	unmarshaledModeSlice := unmarshalModeInformation(respBody)
	//fmt.Println(unmarshaledModeSlice)
	return unmarshaledModeSlice
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
		zap.S().Debugf("Responsstatus not 200 but instead: %s", resp.StatusCode)
		return
	}
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		zap.S().Errorf("ioutil.Readall(resp.Body)  failed: %s", err)
		return
	}
	return
}
