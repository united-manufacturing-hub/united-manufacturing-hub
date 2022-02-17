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
	"regexp"
	"strconv"
	"strings"
	"time"
)

type ModeInformation struct {
	Cid      int                    `json:"cid"`
	ModeData map[string]interface{} `json:"data"`
}

// GetUsedPortsAndModeCached returns a map of one IO-Link-Master with the port number as key and the port mode as value
func GetUsedPortsAndModeCached(currentDeviceInformation DiscoveredDeviceInformation) (map[int]ConnectedDeviceInfo, error) {
	var modeMap map[int]ConnectedDeviceInfo
	var val interface{}
	var found bool

	cacheKey := fmt.Sprintf("GetUsedPortsAndModeCached%s:%s:%s", currentDeviceInformation.ProductCode, currentDeviceInformation.SerialNumber, currentDeviceInformation.Url)
	val, found = internal.GetMemcached(cacheKey)
	if found {
		modeMap = val.(map[int]ConnectedDeviceInfo)
		return modeMap, nil
	}

	usedPortsAndModes, err := getUsedPortsAndMode(currentDeviceInformation.Url)

	if err == nil {
		internal.SetMemcachedLong(cacheKey, usedPortsAndModes, time.Second*20)
	}

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

		if elementMap == nil || elementMap["data"] == nil {
			return nil, errors.New("elementMap is nil")
		}
		portMode := int(elementMap["data"].(float64))
		modeMap[portNumber] = portMode
	}
	return modeMap, nil
}

func UnmarshalUsedPortsAndMode(data []byte) (RawUsedPortsAndMode, error) {
	var r RawUsedPortsAndMode
	err := json.Unmarshal(data, &r)
	return r, err
}

func (r *RawUsedPortsAndMode) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

type RawUsedPortsAndMode struct {
	Cid  int64                `json:"cid"`
	Data map[string]UPAMDatum `json:"data"`
	Code int64                `json:"code"`
}

type UPAMDatum struct {
	Code int64  `json:"code"`
	Data *int64 `json:"data,omitempty"`
}

type ConnectedDeviceInfo struct {
	Mode      uint
	Connected bool
	DeviceId  uint
	VendorId  uint
}

// getUsedPortsAndMode returns which ports have sensors connected, by querying there mastercycletime & mode
func getUsedPortsAndMode(url string) (portmodeusagemap map[int]ConnectedDeviceInfo, err error) {

	//cid can be any number
	var payload = []byte(`{
    "code":"request",
    "cid":42,
    "adr":"/getdatamulti",
    "data":{
        "datatosend":[
            "/iolinkmaster/port[1]/mastercycletime_actual",
            "/iolinkmaster/port[2]/mastercycletime_actual",
            "/iolinkmaster/port[3]/mastercycletime_actual",
            "/iolinkmaster/port[4]/mastercycletime_actual",
            "/iolinkmaster/port[5]/mastercycletime_actual",
            "/iolinkmaster/port[6]/mastercycletime_actual",
            "/iolinkmaster/port[7]/mastercycletime_actual",
            "/iolinkmaster/port[8]/mastercycletime_actual",
            "/iolinkmaster/port[1]/mode",
            "/iolinkmaster/port[2]/mode",
            "/iolinkmaster/port[3]/mode",
            "/iolinkmaster/port[4]/mode",
            "/iolinkmaster/port[5]/mode",
            "/iolinkmaster/port[6]/mode",
            "/iolinkmaster/port[7]/mode",
            "/iolinkmaster/port[8]/mode",
			"/iolinkmaster/port[1]/iolinkdevice/deviceid",
			"/iolinkmaster/port[2]/iolinkdevice/deviceid",
			"/iolinkmaster/port[3]/iolinkdevice/deviceid",
			"/iolinkmaster/port[4]/iolinkdevice/deviceid",
			"/iolinkmaster/port[5]/iolinkdevice/deviceid",
			"/iolinkmaster/port[6]/iolinkdevice/deviceid",
			"/iolinkmaster/port[7]/iolinkdevice/deviceid",
			"/iolinkmaster/port[8]/iolinkdevice/deviceid",
			"/iolinkmaster/port[1]/iolinkdevice/vendorid",
			"/iolinkmaster/port[2]/iolinkdevice/vendorid",
			"/iolinkmaster/port[3]/iolinkdevice/vendorid",
			"/iolinkmaster/port[4]/iolinkdevice/vendorid",
			"/iolinkmaster/port[5]/iolinkdevice/vendorid",
			"/iolinkmaster/port[6]/iolinkdevice/vendorid",
			"/iolinkmaster/port[7]/iolinkdevice/vendorid",
			"/iolinkmaster/port[8]/iolinkdevice/vendorid"
        ]
    }
}`)

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
		return
	}

	var body []byte
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	var portmodesraw RawUsedPortsAndMode
	portmodesraw, err = UnmarshalUsedPortsAndMode(body)
	if err != nil {
		return
	}

	if portmodesraw.Code != 200 {
		err = errors.New(fmt.Sprintf("getusedPortsAndMode returned non 200 code: %d", portmodesraw.Code))
	}

	portmodeusagemap = make(map[int]ConnectedDeviceInfo)

	for key, value := range portmodesraw.Data {
		var port int
		port, err = extractIntFromString(key)
		if err != nil {
			continue
		}
		var val ConnectedDeviceInfo
		var ok bool
		if val, ok = portmodeusagemap[port]; !ok {
			val = ConnectedDeviceInfo{
				Mode:      0,
				Connected: false,
				DeviceId:  0,
				VendorId:  0,
			}
			portmodeusagemap[port] = val
		}
		// mastercycletime_actual will return 200 for analog sensors, if they are connected
		if strings.Contains(key, "mastercycletime_actual") {
			if value.Code == 200 {
				val.Connected = true
				portmodeusagemap[port] = val
			}
		} else if strings.Contains(key, "mode") {
			if value.Code == 200 && value.Data != nil {
				val.Mode = uint(*value.Data)
				if val.Mode == 0 {
					//Manually set to disconnected
					val.Connected = false
				}
				portmodeusagemap[port] = val
			}
		} else if strings.Contains(key, "deviceid") {
			if value.Code == 200 {
				val.DeviceId = uint(*value.Data)
				portmodeusagemap[port] = val
			}
		} else if strings.Contains(key, "vendorid") {
			if value.Code == 200 {
				val.VendorId = uint(*value.Data)
				portmodeusagemap[port] = val
			}
		} else {
			err = errors.New(fmt.Sprintf("Invalid data returned from IO-Link Master: %v -> %v", key, value))
		}
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
