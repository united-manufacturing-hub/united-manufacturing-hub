package main

import (
    "fmt"
    "io/ioutil"
    "log"
    "net/http"
	"encoding/binary"
	"strconv"
)


// Structs for parsing response to discover all IO-Link Master Devices
type DiscoverResponseFromDevice struct{
	cid int `xml:"ProcessData,attr"`
	data Data `xml:"data"`
}

type Data struct {
	_deviceinfo_serialnumber_  StringDataPoint `xml:"/deviceinfo/serialnumber/"`//in xml with / instead of _
	_deviceinfo_productcode_ StringDataPoint `xml:"/deviceinfo/productcode/"`//in xml with / instead of _
}

type StringDataPoint struct {
	code int `xml:"code,attr"`
	data string `xml:"data,attr"`
}


// Struct for relevant information of already discovered devices
type DiscoveredDeviceInformation struct{
	productCode string
	serialNumber string
	url string
}

func discoverDevices(discoveredDevices []DiscoveredDeviceInformation, ipRange string)(discoveredDevices []DiscoveredDeviceInformation){
	// Payload to send to the gateways
    payload := `{
        "code":"request",
        "cid":-1,
        "adr":"/getdatamulti",
        "data":{
            "datatosend":[
                "/deviceinfo/serialnumber/","/deviceinfo/productcode/"]
        }
    }`

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
		resp, err := http.PostForm(url, payload) //Todo: how to set the timeout = 0.1?
		if err != nil {
			fmt.Print(err)
			log.Fatal(err)
		}
		unmarshaledAnswer := DiscoverResponseFromDevice{}

		// Unmarshal file with Unmarshal
		err := xml.Unmarshal(resp, &unmarshaledAnswer)
		if err != nil {
			panic(err) //Todo change to zap stuff
		}

		discoveredDeviceInformation := DiscoveredDeviceInformation{}
		// Insert relevant gained data into DiscoveredDeviceInformation and store in slice
		discoveredDeviceInformation.productCode = unmarshaledAnswer.data._deviceinfo_productcode_.data
		discoveredDeviceInformation.serialNumber = unmarshaledAnswer.data._deviceinfo_serialnumber_.data
		discoveredDeviceInformation.url = url.String()
		
		discoveredDevices = append(discoveredDevices, DiscoveredDeviceInformation)
		return payload, err



	}


    # Iterate over the specified IP space to find connected gateways
    for ip in ipaddress.IPv4Network(ip_range): #IPv4Network('172.16.0.0/24'):
        try:
            url = str(ip)
            r = requests.post("http://"+url, data=payload, timeout=0.1)

            r.raise_for_status()

            response = json.loads(r.text)

            productCode = response["data"]["/deviceinfo/productcode/"]["data"]
            serialNumber = response["data"]["/deviceinfo/serialnumber/"]["data"]
            returnArray.append([url, productCode, serialNumber])
        except Exception as e:
            pass

    return returnArray
}