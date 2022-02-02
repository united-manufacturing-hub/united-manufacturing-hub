package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// DiscoverResponseFromDevice Structs for parsing response to discover all IO-Link Master Devices
type DiscoverResponseFromDevice struct {
	Cid  int  `json:"cid"`
	Data Data `json:"data"`
}

type Data struct {
	DeviceInfoSerialnumber StringDataPoint `json:"/deviceinfo/serialnumber/"`
	DeviceInfoProductCode  StringDataPoint `json:"/deviceinfo/productcode/"`
}

type StringDataPoint struct {
	Code int    `json:"code"`
	Data string `json:"data"`
}

// DiscoveredDeviceInformation Struct for relevant information of already discovered devices
type DiscoveredDeviceInformation struct {
	ProductCode  string
	SerialNumber string
	Url          string
}

func DiscoverDevices(cidr string) ([]DiscoveredDeviceInformation, error) {
	var discoveredDevices []DiscoveredDeviceInformation
	var err error
	start, finish, err := ConvertCidrToIpRange(cidr)

	// loop through addresses as uint32
	for i := start; i <= finish; i++ {
		body, url, err := CheckGivenIpAddress(i)
		if err != nil {
			continue
		}
		unmarshaledAnswer := DiscoverResponseFromDevice{}

		// Unmarshal file with Unmarshal
		err = json.Unmarshal(body, &unmarshaledAnswer)
		if err != nil {
			zap.S().Errorf("Unmarshal of body from url %s failed.", url)
			continue
		}
		// Check CID
		if unmarshaledAnswer.Cid != 23 {
			zap.S().Errorf("Incorrect or missing cid in response. (Should be 23). UnmarshaledAnswer: %s, CurrentUrl: %s, body: %v", unmarshaledAnswer, url, body)
			continue
		}

		discoveredDeviceInformation := DiscoveredDeviceInformation{}
		// Insert relevant gained data into DiscoveredDeviceInformation and store in slice
		discoveredDeviceInformation.ProductCode = unmarshaledAnswer.Data.DeviceInfoProductCode.Data
		discoveredDeviceInformation.SerialNumber = unmarshaledAnswer.Data.DeviceInfoSerialnumber.Data
		discoveredDeviceInformation.Url = url

		discoveredDevices = append(discoveredDevices, discoveredDeviceInformation)
	}
	return discoveredDevices, err
}

func ConvertCidrToIpRange(cidr string) (start uint32, finish uint32, err error) {
	// CIDR conversion copied from https://stackoverflow.com/questions/60540465/how-to-list-all-ips-in-a-network
	// convert string to IPNet struct
	_, ipv4Net, err := net.ParseCIDR(cidr)
	if err != nil {
		log.Fatal(err)
		return
	}

	// convert IPNet struct mask and address to uint32
	// network is BigEndian
	mask := binary.BigEndian.Uint32(ipv4Net.Mask)
	start = binary.BigEndian.Uint32(ipv4Net.IP)

	// find the final address
	finish = (start & mask) | (mask ^ 0xffffffff)
	return
}

func CheckGivenIpAddress(i uint32) (body []byte, url string, err error) {
	// convert back to net.IP
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, i)

	// converto ip to url
	url = "http://" + ip.String()

	// Payload to send to the gateways
	var payload = []byte(`{
        "code":"request",
        "cid":23,
        "adr":"/getdatamulti",
        "data":{
            "datatosend":[
                "/deviceinfo/serialnumber/","/deviceinfo/productcode/"]
        }
    }`)

	// Create Request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		zap.S().Warnf("Failed to create post request for url: %s", url)
		return
	}
	client := &http.Client{}
	client.CloseIdleConnections()
	client.Timeout = time.Second * 50
	resp, err := client.Do(req)
	if err != nil {
		zap.S().Debugf("Client at %s did not respond. %s", url, err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		zap.S().Debugf("Respons status not 200 but instead: %s", resp.StatusCode)
		return
	}
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		zap.S().Errorf("ioutil.Readall(resp.Body)  failed: %s", err)
		return
	}
	return
}
