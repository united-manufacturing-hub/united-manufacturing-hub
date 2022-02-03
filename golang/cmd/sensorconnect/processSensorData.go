package main

import (
	"errors"
	"fmt"
	"go.uber.org/zap"
	"math/big"
	"reflect"
	"strconv"
	"time"
)

// processSensorData processes the donwnloaded information from one io-link-master and sends kafka messages with that information.
// The method sends one message per sensor (active port).
func processSensorData(currentDeviceInformation DiscoveredDeviceInformation, updateIoddIoDeviceMapChan chan IoddFilemapKey, portModeMap map[int]int, sensorDataMap map[string]interface{}) {
	timestampMs := getUnixTimestampMs()

	for portNumber, portMode := range portModeMap {
		mqttRawTopic := fmt.Sprintf("ia/raw/%v/%v/X0%v", transmitterId, currentDeviceInformation.SerialNumber, portNumber)
		switch portMode {
		case 1: // digital input
			// get value from sensorDataMap
			portNumberString := strconv.Itoa(portNumber)
			key := "/iolinkmaster/port[" + portNumberString + "]/pin2in"
			dataPin2In := extractByteArrayFromSensorDataMap(key, "data", sensorDataMap)

			// Payload to send
			payload := createDigitalInputPayload(currentDeviceInformation.SerialNumber, portNumberString, timestampMs, dataPin2In)
			go SendKafkaMessage(MqttTopicToKafka(mqttRawTopic), payload)
		case 2: // digital output
			// Todo
			continue
		case 3: // IO-Link
			// check connection status
			portNumberString := strconv.Itoa(portNumber)
			keyPdin := "/iolinkmaster/port[" + portNumberString + "]/iolinkdevice/pdin"
			connectionCode := extractIntFromSensorDataMap(keyPdin, "code", sensorDataMap)
			if connectionCode != 200 {
				//zap.S().Debugf("connection code of port %v not 200 but: %v", portNumber, connectionCode)
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

			var idm interface{}
			var ok bool
			//check if entry for IoddFilemapKey exists in ioddIoDeviceMap
			if idm, ok = ioDeviceMap.Load(ioddFilemapKey); !ok {
				//zap.S().Debugf("IoddFilemapKey %v not in IodddeviceMap", ioddFilemapKey)
				updateIoddIoDeviceMapChan <- ioddFilemapKey // send iodd filemap Key into update channel (updates can take a while, especially with bad internet -> do it concurrently)
				continue                                    // drop data to avoid locking
			}

			//prepare json Payload to send
			payload := createIoLinkBeginPayload(currentDeviceInformation.SerialNumber, portNumberString, timestampMs)

			// create padded binary raw sensor output
			outputBitLength := rawSensorOutputLength * 4
			rawSensorOutputString := string(rawSensorOutput[:])
			rawSensorOutputBinary := HexToBin(rawSensorOutputString)
			rawSensorOutputBinaryPadded := zeroPadding(rawSensorOutputBinary, outputBitLength)

			cidm := idm.(IoDevice)
			// iterate through RecordItems in Iodd file to extract all values from the padded binary sensor output
			for _, element := range cidm.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray {
				datatype, valueBitLength, err := determineDatatypeAndValueBitLengthOfRecordItem(element, cidm.ProfileBody.DeviceFunction.DatatypeCollection.DatatypeArray)
				if err != nil {
					//zap.S().Warnf("%s", err.Error())
					continue
				}
				leftIndex := outputBitLength - int(valueBitLength) - element.BitOffset
				rightIndex := outputBitLength - element.BitOffset
				binaryValue := rawSensorOutputBinaryPadded[leftIndex:rightIndex]
				valueString := convertBinaryValueToString(binaryValue, datatype)
				//name, err := checkSingleValuesAndValueRanges(element, valueString, datatype, ioddIoDeviceMap[ioddFilemapKey].ProfileBody.DeviceFunction.ProfileBody.DeviceFunction.DatatypeCollection.DatatypeArray)
				valueName := getNameFromExternalTextCollection(element.Name.TextId, cidm.ExternalTextCollection.PrimaryLanguage.Text)
				payload = attachValueString(payload, valueName, valueString)

			}
			payload = append(payload, []byte(`}`)...)
			go SendKafkaMessage(MqttTopicToKafka(mqttRawTopic), payload)
		case 4: // port inactive or problematic (custom port mode: not transmitted from IO-Link-Gateway, but set by sensorconnect)
			continue
		}
	}
	return
}

// getUnixTimestampMs returns the current unix timestamp as string in milliseconds
func getUnixTimestampMs() (timestampMs string) {
	t := time.Now()
	timestampMsInt := int(t.UnixNano() / 1000000)
	timestampMs = strconv.Itoa(timestampMsInt)
	return
}

// extractIntFromSensorDataMap uses the combination of key and tag to retreive an integer
func extractIntFromSensorDataMap(key string, tag string, sensorDataMap map[string]interface{}) int {
	element := sensorDataMap[key]
	elementMap := element.(map[string]interface{})

	val, ok := elementMap[tag].(float64)
	if !ok {
		zap.S().Errorf("Failed to cast elementMap[%s] for key %s to float64. %#v", tag, key, elementMap)
	}
	returnValue := int(val)
	return returnValue
}

// extractIntFromSensorDataMap uses the combination of key and tag to retreive an integer 64
func extractInt64FromSensorDataMap(key string, tag string, sensorDataMap map[string]interface{}) int64 {
	element := sensorDataMap[key]
	elementMap := element.(map[string]interface{})
	val, ok := elementMap[tag].(float64)
	if !ok {
		zap.S().Errorf("Failed to cast elementMap[%s] for key %s to float64. %#v", tag, key, elementMap)
	}
	returnValue := int64(val)
	return returnValue
}

// extractIntFromSensorDataMap uses the combination of key and tag to retreive a byte slice
func extractByteArrayFromSensorDataMap(key string, tag string, sensorDataMap map[string]interface{}) []byte {
	element := sensorDataMap[key]
	elementMap := element.(map[string]interface{})
	returnString := fmt.Sprintf("%v", elementMap[tag])
	returnValue := []byte(returnString)
	return returnValue
}

// zeroPadding adds zeros on the left side of a string until the lengt of the string equals the requested length
func zeroPadding(input string, length int) (output string) {
	output = fmt.Sprintf("%0*v", length, input)
	return
}

// HexToBin converts a hex string into a binary string
func HexToBin(hex string) (bin string) {
	i := new(big.Int)
	i.SetString(hex, 16)
	bin = fmt.Sprintf("%b", i)
	return
}

// BinToHex converts a binary string to a hex string
func BinToHex(bin string) (hex string) {
	i := new(big.Int)
	i.SetString(bin, 2)
	hex = fmt.Sprintf("%x", i)
	return
}

// determineValueBitLength returns the bitlength of a value
func determineValueBitLength(datatype string, bitLength uint, fixedLength uint) (length uint) {
	if datatype == "BooleanT" {
		return 1
	} else if datatype == "octetStringT" {
		return fixedLength * 8
	} else {
		return bitLength
	}
}

// determineDatatypeAndValueBitLengthOfRecordItem finds out datatype and bit length of a given iodd RecordItem
func determineDatatypeAndValueBitLengthOfRecordItem(item RecordItem, datatypeArray []Datatype) (datatype string, bitLength uint, err error) {
	if !reflect.DeepEqual(item.SimpleDatatype.Type, "") { //  true if record item includes a simple datatype
		datatype = item.SimpleDatatype.Type
		bitLength = determineValueBitLength(datatype, item.SimpleDatatype.BitLength, item.SimpleDatatype.FixedLength)
		return
	} else if !reflect.DeepEqual(item.DatatypeRef.DatatypeId, "") { // true if record item includes a datatypeRef -> look for type into DatatypeCollection with id
		for _, datatypeElement := range datatypeArray {
			if reflect.DeepEqual(datatypeElement.Id, item.DatatypeRef.DatatypeId) {
				datatype = datatypeElement.Type // IntegerT or UIntegerT or Float32T
				bitLength = determineValueBitLength(datatype, datatypeElement.BitLength, datatypeElement.FixedLength)
				return
			}
			//zap.S().Warnf("datatypeElement.Id vs item.DatatypeRef.DatatypeId: %s vs %s", datatypeElement.Id, item.DatatypeRef.DatatypeId)
		}
		//zap.S().Warnf("datatypeArray: %v", datatypeArray)
		err = errors.New("DatatypeRef.DatatypeId is not in DatatypeCollection of Iodd file -> Datatype could not be determined.")
		return
	} else {
		err = errors.New("Neither SimpleDatatype nor DatatypeRef included in Recorditem")
		return
	}
}

/*
// checkSingleValuesAndValueRanges checks if value of record item is in a given valuerange or on a singlevalue. It returns the name of the singlevalue and an error if a ValueRange or SingleValue
//are given but not met
func checkSingleValuesAndValueRanges(item RecordItem, valueString string, datatype string, datatypeArray []Datatype) (name string, err error){
	if !reflect.DeepEqual(item.SimpleDatatype.Type, ""){ // enters if simple datatype
		if  (reflect.DeepEqual(item.SimpleDatatype.ValueRange, "") && reflect.DeepEqual(item.SimpleDatatype.SingleValue, "")){ // simple datatype doesn't contain SingleValue or ValueRange
			return nil, nil
		}else if !reflect.DeepEqual(item.SimpleDatatype.ValueRange, ""){// simple datatype contains ValueRange
			if reflect.DeepEqual(datatype, "IntegerT"){
				intUpperBound, err := strconv.Atoi(item.SimpleDatatype.ValueRange.UpperValue)
				if err != nil {
					return
				}
				intLowerBound, err := strconv.Atoi(item.SimpleDatatype.ValueRange.LowerValue)
				if err != nil {
					return
				}
			} else if reflect.DeepEqual(datatype, "UIntegerT"){
				intUpperBound, err := strconv.Atoi(item.SimpleDatatype.ValueRange.UpperValue)
				if err != nil {
					return
				}
				intLowerBound, err := strconv.Atoi(item.SimpleDatatype.ValueRange.LowerValue)
				if err != nil {
					return
				}
			} else if reflect.DeepEqual(datatype, "Float32T"){
				floatUpperBound, err := strconv.ParseFloat(item.SimpleDatatype.ValueRange.UpperValue, 32)
				if err != nil {
					return
				}
				floatLowerBound, err := strconv.ParseFloat(item.SimpleDatatype.ValueRange.UpperValue, 32)
				if err != nil {
					return
				}
			return nil, checkIfValueInValueRange()
		}
	}

	}
	} else if !reflect.DeepEqual(item.DatatypeRef.DatatypeId, "") { // true if record item includes a datatypeRef -> look for type into DatatypeCollection with id
		for _, datatypeElement := range datatypeArray {
			if reflect.DeepEqual(datatypeElement.Id, item.DatatypeRef.DatatypeId) {

				return
			}
		}
		err = errors.New("DatatypeRef.DatatypeId is not in DatatypeCollection of Iodd file -> Datatype could not be determined.")
		return
	} else {
		err = errors.New("Neither SimpleDatatype nor DatatypeRef included in Recorditem")
		return
}


// valueRangeOrSingleValueExists returns true if a RecordItem holds single values or valueRanges
func valueRangeOrSingleValueExistsInSimpleDatatype(item RecordItem){
	if (reflect.DeepEqual(item.SimpleDatatype.ValueRange, "") && reflect.DeepEqual(item.SimpleDatatype.SingleValue, ""){
		return false
	} else{
		return true
	}
}
*/
// convertBinaryValueToString converts a binary string value to a readable string according to its datatype
func convertBinaryValueToString(binaryValue string, datatype string) (output string) {
	switch datatype {
	case "OctetStringT":
		output = BinToHex(binaryValue)
	default:
		outputString, _ := strconv.ParseUint(binaryValue, 2, 64)
		return fmt.Sprintf("%v", outputString)
	}
	return
}

// createDigitalInputPayload creates a json output body from a DigitalInput to send via mqtt or kafka to the server
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

// createDigitalInputPayload creates the upper json output body from an IoLink response to send via mqtt or kafka to the server
func createIoLinkBeginPayload(serialNumber string, portNumberString string, timestampMs string) (payload []byte) {
	payload = []byte(`{
		"serial_number":`)
	payload = append(payload, []byte(serialNumber)...)
	payload = append(payload, []byte(`-X0`)...)
	payload = append(payload, []byte(portNumberString)...)
	payload = append(payload, []byte(`,
	"timestamp_ms":`)...)
	payload = append(payload, []byte(timestampMs)...)
	payload = append(payload, []byte(`,
	"type":Io-Link,
	"connected":connected`)...)

	return
}

// attachValueString can be used to attach further json information to an existing output body
func attachValueString(payload []byte, valueName string, valueString string) []byte {
	payload = append(payload, []byte(`,
	"`)...)
	payload = append(payload, []byte(valueName)...)
	payload = append(payload, []byte(`":`)...)
	payload = append(payload, []byte(valueString)...)
	return payload
}

// getNameFromExternalTextCollection retreives the name correesponding to a textId from the iodd TextCollection
func getNameFromExternalTextCollection(textId string, text []Text) string {
	for _, element := range text {
		if textId == element.Id {
			return element.Value
		}
	}
	return "error: translation not found"
}
