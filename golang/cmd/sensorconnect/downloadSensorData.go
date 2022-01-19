package main

func DownloadSensorData(currentDeviceInformation DiscoveredDeviceInformation) {

	//Number of ports
	numberOfPorts := findNumberOfPorts(currentDeviceInformation.ProductCode)
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

func createRequestBodyForPort(numberOfPorts int) []byte {
	// Payload to send to the gateways
	var payload = []byte(`{
		"code":"request",
		"cid":23,
		"adr":"/getdatamulti",
		"data":{
			"datatosend":[
				"/iolinkmaster/port[1]/mode"]`)
	for i := 2; i <= numberOfPorts; i++ {
		payload = append(payload, []byte(`,"/iolinkmaster/port[`)...)
		payload = append(payload, byte(numberOfPorts))
		payload = append(payload, []byte(`]/mode"`)...)
	}
	payload = append(payload, []byte(`]}}`)...)
	return payload
}
