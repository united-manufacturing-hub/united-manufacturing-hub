package main

import (
	"fmt"
	"math/big"
	"strconv"
	"time"

	"go.uber.org/zap"
)

func processSensorData(sensorDataMap map[string]interface{},
	currentDeviceInformation DiscoveredDeviceInformation,
	portModeMap map[int]int,
	ioddIoDeviceMap map[IoddFilemapKey]IoDevice,
	updateIoddIoDeviceMapChannel chan IoddFilemapKey) (err error) {
	timestampMs := getUnixTimestampMs()
	for portNumber, portMode := range portModeMap {
		//mqttRawTopic := fmt.Sprintf("ia/raw/%v/%v/X0%v", transmitterId, currentDeviceInformation.SerialNumber, portNumber)
		switch portMode {
		case 1: // digital input
			// get value from sensorDataMap
			portNumberString := strconv.Itoa(portNumber)
			key := "/iolinkmaster/port[" + portNumberString + "]/pin2in"
			dataPin2In := extractByteArrayFromSensorDataMap(key, "data", sensorDataMap)

			// Payload to send
			payload := createDigitalInputPayload(currentDeviceInformation.SerialNumber, portNumberString, timestampMs, timestampMs)
			var payload = fmt.Println(payload)
		case 2: // digital output
			// Todo
			continue
		case 3: // IO-Link
			// check connection status
			portNumberString := strconv.Itoa(portNumber)
			keyPdin := "/iolinkmaster/port[" + portNumberString + "]/iolinkdevice/pdin"
			connectionCode := extractIntFromSensorDataMap(keyPdin, "code", sensorDataMap)
			if connectionCode != 200 {
				zap.S().Errorf("connection code of port %v not 200 but: %v", portNumber, connectionCode)
				continue
			}

			// get Deviceid
			keyDeviceid := "/iolinkmaster/port[" + portNumberString + "]/iolinkdevice/deviceid"
			keyVendorid := "/iolinkmaster/port[" + portNumberString + "]/iolinkdevice/vendorid"
			deviceId := extractIntFromSensorDataMap(keyDeviceid, "data", sensorDataMap)
			vendorId := extractInt64FromSensorDataMap(keyVendorid, "data", sensorDataMap)
			rawSensorOutput := extractByteArrayFromSensorDataMap(keyPdin, "data", sensorDataMap)
			rawSensorOutputLength := len(rawSensorOutput)

			//create IoddFilemapKey
			var ioddFilemapKey IoddFilemapKey
			ioddFilemapKey.DeviceId = deviceId
			ioddFilemapKey.VendorId = vendorId

			//check if entry for IoddFilemapKey exists in ioddIoDeviceMap
			if _, ok := ioddIoDeviceMap[ioddFilemapKey]; !ok {
				updateIoddIoDeviceMapChannel <- ioddFilemapKey // send iodd filemap Key into update channel
				continue                                       // drop data to avoid locking
			}

			//prepare json Payload to send
			var payload = []byte(`{
				"serial_number":`)
			payload = append(payload, []byte(currentDeviceInformation.SerialNumber)...)
			payload = append(payload, []byte(`-X0`)...)
			payload = append(payload, []byte(strconv.Itoa(portNumber))...)
			payload = append(payload, []byte(`,
			"timestamp_ms:`)...)
			payload = append(payload, []byte(strconv.Itoa(timestampMs))...)
			payload = append(payload, []byte(`,
			"type":Io-Link,
			"connected":connected`)...)

			// create padded binary raw sensor output
			outputBitLength := rawSensorOutputLength * 4
			rawSensorOutputString := string(rawSensorOutput[:])
			rawSensorOutputBinary := HexToBin(string(rawSensorOutputString))
			rawSensorOutputBinaryPadded := zeroPadding(rawSensorOutputBinary, outputBitLength)

			// iterate through RecordItems in Iodd file to extract all values from the padded binary sensor output
			for _, element := range ioddIoDeviceMap[ioddFilemapKey].ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem {
				valueBitLength := determineValueBitLength(element) // length of value
				leftIndex := outputBitLength - valueBitLength - element.BitOffset
				rightIndex := outputBitLength - element.BitOffset
				binaryValue := rawSensorOutputBinaryPadded[leftIndex:rightIndex]
				valueString := convertBinaryValueToString(binaryValue, element)
				payload = append(payload, []byte(`,
				"`)...)
				payload = append(payload, []byte(element.Name.TextId)...)
				payload = append(payload, []byte(`":`)...)
				payload = append(payload, []byte(valueString)...)
			}
			payload = append(payload, []byte(`}`)...)
			fmt.Println(payload)
		case 4: // port inactive or problematic (custom port mode: not transmitted from IO-Link-Gateway, but set by sensorconnect)
			continue
		}
	}
	return
}

func getUnixTimestampMs() (timestampMs string) {
	t := time.Now()
	timestampMsInt := int(t.UnixNano() / 1000000)
	timestampMs = strconv.Itoa(timestampMsInt)
	return
}

func extractIntFromSensorDataMap(key string, tag string, sensorDataMap map[string]interface{}) int {
	element := sensorDataMap[key]
	elementMap := element.(map[string]interface{})
	returnValue := int(elementMap[tag].(float64))
	return returnValue
}

func extractInt64FromSensorDataMap(key string, tag string, sensorDataMap map[string]interface{}) int64 {
	element := sensorDataMap[key]
	elementMap := element.(map[string]interface{})
	returnValue := int64(elementMap[tag].(float64))
	return returnValue
}

func extractByteArrayFromSensorDataMap(key string, tag string, sensorDataMap map[string]interface{}) []byte {
	element := sensorDataMap[key]
	elementMap := element.(map[string]interface{})
	returnValue := elementMap[tag].([]byte)
	return returnValue
}

func zeroPadding(input string, length int) (output string) {
	output = fmt.Sprintf("%0*v", length, input)
	return
}

func HexToBin(hex string) (bin string) {
	i := new(big.Int)
	i.SetString(hex, 16)
	bin = fmt.Sprintf("%b", i)
	return
}
func BinToHex(s string) string {
	ui, err := strconv.ParseUint(s, 2, 64)
	if err != nil {
		return "error"
	}

	return fmt.Sprintf("%x", ui)
}

func determineValueBitLength(item RecordItem) (length int) {
	if item.SimpleDatatype.Type == "BooleanT" {
		return 1
	} else if item.SimpleDatatype.Type == "octetStringT" {
		return item.SimpleDatatype.FixedLength * 8
	} else {
		return item.SimpleDatatype.BitLength
	}
}

func convertBinaryValueToString(binaryValue string, element RecordItem) (output string) {
	if element.SimpleDatatype.Type == "OctetStringT" {
		output = BinToHex(binaryValue)
	} else {
		outputString, _ := strconv.ParseUint(binaryValue, 2, 64)
		return fmt.Sprintf("%v", outputString)
	}
	return
}

func createDigitalInputPayload(serialNumber string, portNumberString string, timestampMs string, dataPin2In []byte) (payload []byte) {
	payload = []byte(`{
		"serial_number":`)
	payload = append(payload, []byte(serialNumber)...)
	payload = append(payload, []byte(`-X0`)...)
	payload = append(payload, []byte(portNumberString)...)
	payload = append(payload, []byte(`,
	"timestamp_ms:`)...)
	payload = append(payload, []byte(timestampMs)...)
	payload = append(payload, []byte(`,
	"type":DI,
	"connected":connected
	"value":`)...)
	payload = append(payload, dataPin2In...)
	payload = append(payload, []byte(`}`)...)

	return
}
