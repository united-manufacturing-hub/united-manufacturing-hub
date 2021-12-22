package main

import (
    "fmt"
    "io/ioutil"
    "log"
    "net/http"
	"encoding/binary"
	"strconv"
)


func discoverDevices(deviceMap [key]device, ipRange string)(returnDeviceMap [key]device){
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
		//check url addresse with post request
		resp, err := http.PostForm(url, payload)
		if err != nil {
			fmt.Print(err)
			log.Fatal(err)
		}
		var response map[string]interface{}

		json.NewDecoder(response.Body).Decode(&response)
	
		fmt.Println(response["form"])
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