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
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
)

// DiscoverResponseFromDevice Structs for parsing response to discover all IO-Link Master Devices
type DiscoverResponseFromDevice struct {
	Data Data `json:"data"`
	Cid  int  `json:"cid"`
}

type Data struct {
	DeviceInfoSerialnumber StringDataPoint `json:"/deviceinfo/serialnumber/"`
	DeviceInfoProductCode  StringDataPoint `json:"/deviceinfo/productcode/"`
}

type StringDataPoint struct {
	Data string `json:"data"`
	Code int    `json:"code"`
}

// DiscoveredDeviceInformation Struct for relevant information of already discovered devices
type DiscoveredDeviceInformation struct {
	ProductCode  string
	SerialNumber string
	Url          string
}

func DiscoverDevices(cidr string) (err error) {
	start, finish, err := ConvertCidrToIpRange(cidr)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	// loop through addresses as uint32
	nDevices := finish - start
	zap.S().Debugf("Scanning %d IP addresses", nDevices)
	for i := start; i <= finish; i++ {
		wg.Add(1)
		go GetDiscoveredDeviceInformation(&wg, i)
		internal.SleepBackedOff(int64(i), 10*time.Nanosecond, 10*time.Millisecond)
	}

	wg.Wait()
	return nil
}

func GetDiscoveredDeviceInformation(wg *sync.WaitGroup, i uint32) {
	defer wg.Done()
	body, url, err := CheckGivenIpAddress(i)
	if err != nil {
		return
	}
	unmarshaledAnswer := DiscoverResponseFromDevice{}

	// Unmarshal file with Unmarshal
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	err = json.Unmarshal(body, &unmarshaledAnswer)
	if err != nil {
		// zap.S().Errorf("Unmarshal of body from url %s failed.", url)
		return
	}

	ddI := DiscoveredDeviceInformation{}
	// Insert relevant gained data into DiscoveredDeviceInformation and store in slice
	ddI.ProductCode = unmarshaledAnswer.Data.DeviceInfoProductCode.Data
	ddI.SerialNumber = unmarshaledAnswer.Data.DeviceInfoSerialnumber.Data
	ddI.Url = url
	zap.S().Infof("Found device (SN: %s, PN: %s) at %s", ddI.SerialNumber, ddI.ProductCode, url)

	// Pre-create topic
	portModeMap, err := GetUsedPortsAndModeCached(ddI)
	if err != nil {
		return
	}
	for portNumber := range portModeMap {
		mqttRawTopic := fmt.Sprintf("ia/raw/%v/%v/X0%v", transmitterId, ddI.SerialNumber, portNumber)
		validTopic, kafkaTopic := internal.MqttTopicToKafka(mqttRawTopic)
		if !validTopic {
			zap.S().Warnf("Invalid topic %s", mqttRawTopic)
			continue
		}
		if useKafka {
			err := internal.CreateTopicIfNotExists(kafkaTopic)
			if err != nil {
				zap.S().Errorf(
					"Failed to create topic %s, this can happen during initial startup, it might take up to 5 minutes for Kafka to startup. If you encounter this error, while Kafka is already running, please investigate further",
					err)
				internal.ShuttingDownKafka = true
				time.Sleep(internal.FiveSeconds)
				os.Exit(1)
			}
		}
	}

	discoveredDeviceChannel <- ddI
}

func ConvertCidrToIpRange(cidr string) (start uint32, finish uint32, err error) {
	// CIDR conversion copied from https://stackoverflow.com/questions/60540465/how-to-list-all-ips-in-a-network
	// convert string to IPNet struct
	_, ipv4Net, err := net.ParseCIDR(cidr)
	if err != nil {
		zap.S().Fatalf("Failed to parse CIDR %s", cidr)
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
	// cid can be any number
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
	req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewBuffer(payload))
	if err != nil {
		// zap.S().Warnf("Failed to create post request for url: %s", url)
		return
	}
	client := GetHTTPClient(url)
	client.CloseIdleConnections()
	client.Timeout = time.Second * time.Duration(deviceFinderTimeoutInS)
	resp, err := client.Do(req)
	if err != nil {
		// zap.S().Debugf("Client at %s did not respond. %s", url, err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		zap.S().Debugf("Response status not 200 but instead: %d (URL: %s)", resp.StatusCode, url)
		return
	}
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		zap.S().Errorf("io.ReadAll(resp.Body)  failed: %s", err)
		return
	}
	return
}
