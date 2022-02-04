package main

import (
	"crypto/sha512"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"time"

	"go.uber.org/zap"
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
			key := "/iolinkmaster/port[" + portNumberString + "]/iolinkdevice/pdin"
			dataPin2In := extractByteArrayFromSensorDataMap(key, "data", sensorDataMap)

			// Payload to send
			payload := createDigitalInputPayload(currentDeviceInformation.SerialNumber, portNumberString, timestampMs, dataPin2In)
			go SendKafkaMessage(MqttTopicToKafka(mqttRawTopic), payload, GenerateKafkaKey(currentDeviceInformation))
			go SendMQTTMessage(mqttRawTopic, payload)
		case 2: // digital output
			// Todo
			continue
		case 3: // IO-Link
			// check connection status
			portNumberString := strconv.Itoa(portNumber)
			keyPdin := "/iolinkmaster/port[" + portNumberString + "]/iolinkdevice/pdin"
			connectionCode := extractIntFromSensorDataMap(keyPdin, "code", sensorDataMap)
			if connectionCode != 200 {
				zap.S().Debugf("connection code of port %v not 200 but: %v", portNumber, connectionCode)
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
				zap.S().Debugf("IoddFilemapKey %v not in IodddeviceMap", ioddFilemapKey)
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

			// Extract important IoddStruct parts for better readability
			processDataIn := cidm.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn
			datatypeReferenceArray := cidm.ProfileBody.DeviceFunction.DatatypeCollection.DatatypeArray
			var emptySimpleDatatype SimpleDatatype
			primLangExternalTextCollection := cidm.ExternalTextCollection.PrimaryLanguage.Text

			var err error
			zap.S().Debugf("Starting to process port number = %v", portNumber)
			// use the acquired info to process the raw data coming from the sensor correctly in to human readable data and attach to payload
			payload, err = processData(processDataIn.Datatype, processDataIn.DatatypeRef, emptySimpleDatatype, 0, payload, outputBitLength, rawSensorOutputBinaryPadded, datatypeReferenceArray, processDataIn.Name.TextId, primLangExternalTextCollection)
			if err != nil {
				payload = attachValueString(payload, "RawSensorOutput", string(rawSensorOutput[:])) // if an error occurs attach the raw sensor data to the payload
				zap.S().Errorf("Processing Sensordata failed: %v", err)
			}

			payload = append(payload, []byte(`}`)...)

			go SendKafkaMessage(MqttTopicToKafka(mqttRawTopic), payload, GenerateKafkaKey(currentDeviceInformation))
			go SendMQTTMessage(mqttRawTopic, payload)
		case 4: // port inactive or problematic (custom port mode: not transmitted from IO-Link-Gateway, but set by sensorconnect)
			continue
		}
	}
}

func GenerateKafkaKey(information DiscoveredDeviceInformation) []byte {
	sha_512 := sha512.New()
	sha_512.Write([]byte(information.Url))
	sha_512.Write([]byte(information.ProductCode))
	sha_512.Write([]byte(information.SerialNumber))
	return sha_512.Sum(nil)
}

// processData turns raw sensor data into human readable data and attaches it to the payload. It can handle the input of datatype, datatypeRef and simpleDatatype structures.
// It determines which one of those was given (not empty) and delegates the processing accordingly.
func processData(datatype Datatype, datatypeRef DatatypeRef, simpleDatatype SimpleDatatype, bitOffset int,
	payload []byte, outputBitLength int, rawSensorOutputBinaryPadded string, datatypeReferenceArray []Datatype,
	nameTextId string, primLangExternalTextCollection []Text) (payloadOut []byte, err error) {
	if !isEmpty(simpleDatatype) {
		payloadOut, err = processSimpleDatatype(simpleDatatype, payload, outputBitLength, rawSensorOutputBinaryPadded, bitOffset, nameTextId, primLangExternalTextCollection)
		if err != nil {
			zap.S().Errorf("Error with processSimpleDatatype: %v", err)
			return
		}
		zap.S().Debugf("Processed simple Datatype, Payload = %v", string(payload))
		return
	} else if !isEmpty(datatype) {
		payloadOut, err = processDatatype(datatype, payload, outputBitLength, rawSensorOutputBinaryPadded, bitOffset, datatypeReferenceArray, nameTextId, primLangExternalTextCollection)
		if err != nil {
			zap.S().Errorf("Error with processDatatype: %v", err)
			return
		}
		zap.S().Debugf("Processed Datatype, Payload = %v", string(payload))
		return
	} else if !isEmpty(datatypeRef) {
		datatype, err = getDatatypeFromDatatypeRef(datatypeRef, datatypeReferenceArray)
		if err != nil {
			zap.S().Errorf("Error with getDatatypeFromDatatypeRef: %v", err)
			return
		}
		zap.S().Debugf("Processed datatypeRef, Payload = %v", string(payload))
		payloadOut, err = processDatatype(datatype, payload, outputBitLength, rawSensorOutputBinaryPadded, bitOffset, datatypeReferenceArray, nameTextId, primLangExternalTextCollection)
		return
	} else {
		zap.S().Errorf("Missing input, neither simpleDatatype or datatype or datatypeRef given.")
		return
	}
}

// getDatatypeFromDatatypeRef uses the given datatypeReference to find the actual datatype description in the datatypeReferenceArray and returns it.
func getDatatypeFromDatatypeRef(datatypeRef DatatypeRef, datatypeReferenceArray []Datatype) (datatypeOut Datatype, err error) {
	for _, datatypeElement := range datatypeReferenceArray {
		if reflect.DeepEqual(datatypeElement.Id, datatypeRef.DatatypeId) {
			datatypeOut = datatypeElement
			zap.S().Debugf("Found matching datatype for datatypeRef, datatype = %v", datatypeOut)
			return
		}
	}
	zap.S().Errorf("DatatypeRef.DatatypeId is not in DatatypeCollection of Iodd file -> Datatype could not be determined.")
	err = fmt.Errorf("did not find Datatype structure for given datatype reference id: %v", datatypeRef.DatatypeId)
	return
}

// processSimpleDatatype uses the given simple datatype information to attach the information to the payload
func processSimpleDatatype(simpleDatatype SimpleDatatype, payload []byte, outputBitLength int, rawSensorOutputBinaryPadded string, bitOffset int,
	nameTextId string, primLangExternalTextCollection []Text) (payloadOut []byte, err error) {

	binaryValue := extractBinaryValueFromRawSensorOutput(rawSensorOutputBinaryPadded, simpleDatatype.Type, simpleDatatype.BitLength, simpleDatatype.FixedLength, outputBitLength, bitOffset)
	valueString := convertBinaryValueToString(binaryValue, simpleDatatype.Type)
	valueName := getNameFromExternalTextCollection(nameTextId, primLangExternalTextCollection)
	payloadOut = attachValueString(payload, valueName, valueString)
	return
}

// extractBinaryValueFromRawSensorOutput handles the cutting and converting of the actual raw sensor data.
func extractBinaryValueFromRawSensorOutput(rawSensorOutputBinaryPadded string, typeString string, bitLength uint, fixedLength uint, outputBitLength int, bitOffset int) string {
	valueBitLength := determineValueBitLength(typeString, bitLength, fixedLength)

	leftIndex := outputBitLength - int(valueBitLength) - bitOffset
	rightIndex := outputBitLength - bitOffset
	binaryValue := rawSensorOutputBinaryPadded[leftIndex:rightIndex]
	return binaryValue
}

// processDatatype can process a Datatype structure. If the bitOffset is not given, enter zero.
func processDatatype(datatype Datatype, payload []byte, outputBitLength int, rawSensorOutputBinaryPadded string, bitOffset int, datatypeReferenceArray []Datatype,
	nameTextId string, primLangExternalTextCollection []Text) (payloadOut []byte, err error) {
	if reflect.DeepEqual(datatype.Type, "RecordT") {
		payloadOut = processRecordType(payload, datatype.RecordItemArray, outputBitLength, rawSensorOutputBinaryPadded, datatypeReferenceArray, primLangExternalTextCollection)
		return
	} else {
		zap.S().Debugf("Starting to process rawSensorOutputBinaryPadded = %v with datatype %v iodd information", rawSensorOutputBinaryPadded, datatype)
		binaryValue := extractBinaryValueFromRawSensorOutput(rawSensorOutputBinaryPadded, datatype.Type, datatype.BitLength, datatype.FixedLength, outputBitLength, bitOffset)
		valueString := convertBinaryValueToString(binaryValue, datatype.Type)
		valueName := getNameFromExternalTextCollection(nameTextId, primLangExternalTextCollection)
		payloadOut = attachValueString(payload, valueName, valueString)
		return
	}
}

// processRecordType iterates through the given recordItemArray and calls the processData function for each RecordItem
func processRecordType(payload []byte, recordItemArray []RecordItem, outputBitLength int, rawSensorOutputBinaryPadded string, datatypeReferenceArray []Datatype, primLangExternalTextCollection []Text) []byte {
	// iterate through RecordItems in Iodd file to extract all values from the padded binary sensor output
	for _, element := range recordItemArray {
		var datatypeEmpty Datatype
		var err error
		payload, err = processData(datatypeEmpty, element.DatatypeRef, element.SimpleDatatype, element.BitOffset, payload, outputBitLength, rawSensorOutputBinaryPadded, datatypeReferenceArray, element.Name.TextId, primLangExternalTextCollection)
		//zap.S().Debugf("Processed RecordItem = %v with datatype %v iodd information", element)
		if err != nil {
			zap.S().Errorf("Procession of RecordItem failed: %v", element)
		}
	}
	return payload
}

// isEmpty determines if an field of a struct is empty of filled
func isEmpty(object interface{}) bool {
	//First check normal definitions of empty
	if object == nil {
		return true
	} else if object == "" {
		return true
	} else if object == false {
		return true
	}

	//Then see if it's a struct
	if reflect.ValueOf(object).Kind() == reflect.Struct {
		// and create an empty copy of the struct object to compare against
		empty := reflect.New(reflect.TypeOf(object)).Elem().Interface()
		if reflect.DeepEqual(object, empty) {
			return true
		}
	}
	return false
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
