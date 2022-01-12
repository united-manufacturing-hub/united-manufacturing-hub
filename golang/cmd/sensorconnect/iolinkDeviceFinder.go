package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
)

/*
testRequest := {
	"code":"request",
	"cid":23,
	"adr":"/getdatamulti",
	"data":{
		"datatosend":[
			"/deviceinfo/serialnumber/","/deviceinfo/productcode/"]
	}
}
*/
// Structs for parsing response to discover all IO-Link Master Devices
type DiscoverResponseFromDevice struct {
	cid  int  `xml:"ProcessData,attr"`
	data Data `xml:"data"`
}

type Data struct {
	_deviceinfo_serialnumber_ StringDataPoint `xml:"/deviceinfo/serialnumber/"` //in xml with / instead of _
	_deviceinfo_productcode_  StringDataPoint `xml:"/deviceinfo/productcode/"`  //in xml with / instead of _
}

type StringDataPoint struct {
	code int    `xml:"code,attr"`
	data string `xml:"data,attr"`
}

// Struct for relevant information of already discovered devices
type DiscoveredDeviceInformation struct {
	productCode  string
	serialNumber string
	url          string
}

func discoverDevices(discoveredDevices []DiscoveredDeviceInformation, ipRange string) []DiscoveredDeviceInformation {
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

	// CIDR copied from https://stackoverflow.com/questions/60540465/how-to-list-all-ips-in-a-network
	// convert string to IPNet struct
	_, ipv4Net, err := net.ParseCIDR(ipRange)
	if err != nil {
		log.Fatal(err)
	}

	// convert IPNet struct mask and address to uint32
	// network is BigEndian
	mask := binary.BigEndian.Uint32(ipv4Net.Mask)
	start := binary.BigEndian.Uint32(ipv4Net.IP)

	// find the final address
	finish := (start & mask) | (mask ^ 0xffffffff)

	// loop through addresses as uint32
	for i := start; i <= finish; i++ {
		// convert back to net.IP
		ip := make(net.IP, 4)
		binary.BigEndian.PutUint32(ip, i)
		fmt.Println(ip)

		// converto ip to url
		url := "http://" + ip.String()
		//check url address with post request
		/*
			resp, err := http.PostForm(urlString, url.Values{
				"code":"request",
				"cid":24,
				"adr":"/getdatamulti",
				"data":{
					"datatosend":[
						"/deviceinfo/serialnumber/","/deviceinfo/productcode/"]
					}
				}) //Todo: how to set the timeout = 0.1?
		*/

		fmt.Println("URL:>", url)

		req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
		// necessary?: req.Header.Set("X-Custom-Header", "myvalue")
		// necessary?: req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			panic(err)
		}
		defer resp.Body.Close()

		fmt.Println("response Status:", resp.Status)
		fmt.Println("response Headers:", resp.Header)

		if err != nil {
			fmt.Print(err)
			log.Fatal(err)
		}
		if resp.StatusCode != 200 {
			fmt.Println("Error: Statuscode not euqal 200!")
			continue
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			// zap.error
			fmt.Println("ioutil.Readall(resp.Body)  failed")
			continue
		}
		unmarshaledAnswer := DiscoverResponseFromDevice{}

		// Unmarshal file with Unmarshal
		err = json.Unmarshal(body, &unmarshaledAnswer)
		if err != nil {
			panic(err) //Todo change to zap stuff
		}

		if unmarshaledAnswer.cid != 23 {
			fmt.Println("Incorrect or missing cid in response. (Should be 23)")
		}
		discoveredDeviceInformation := DiscoveredDeviceInformation{}
		// Insert relevant gained data into DiscoveredDeviceInformation and store in slice
		discoveredDeviceInformation.productCode = unmarshaledAnswer.data._deviceinfo_productcode_.data
		discoveredDeviceInformation.serialNumber = unmarshaledAnswer.data._deviceinfo_serialnumber_.data
		discoveredDeviceInformation.url = url

		discoveredDevices = append(discoveredDevices, discoveredDeviceInformation)
	}
	return discoveredDevices
}
